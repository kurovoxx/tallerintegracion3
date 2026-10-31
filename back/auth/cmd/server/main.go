package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/config"
	identityv1 "github.com/kurovoxx/tallerintegracion3/back/auth/internal/gen/identityv1"
	grpchandler "github.com/kurovoxx/tallerintegracion3/back/auth/internal/handler/grpc"
	httpHandler "github.com/kurovoxx/tallerintegracion3/back/auth/internal/handler/http"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/interceptor"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
	"google.golang.org/grpc"
)

func main() {
	cfg := config.Load()
	log.Printf("config: DATABASE_URL set=%v JWT_SECRET set=%v", cfg.DatabaseURL != "", cfg.JWTSecret != "")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := repository.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("No se pudo conectar a Postgres (DATABASE_URL): %v", err)
	}
	defer pool.Close()
	log.Println("PostgreSQL conectado OK (pgxpool Ping)")

	// Verificación rápida de schemas críticos Sprint 1
	var cnt int
	if err := pool.QueryRow(ctx, "select count(*) from identity.users").Scan(&cnt); err != nil {
		log.Fatalf("query identity.users: %v", err)
	}
	log.Printf("DB check: identity.users count=%d", cnt)
	if err := repository.VerifyAccountDeletionSchema(ctx, pool); err != nil {
		log.Fatalf("%v", err)
	}

	// Wire Handler → Service → Repository (masterprompt 2) + JWT standalone integrado (3.1)
	userRepo := repository.NewUserRepository(pool)
	refreshRepo := repository.NewRefreshTokenRepository(pool)
	jwtSvc, err := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte(cfg.JWTSecret),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     time.Duration(cfg.AccessExpiresIn) * time.Second,
	})
	if err != nil {
		log.Fatalf("config JWT: %v", err)
	}
	authSvc := service.NewAuthService(userRepo, refreshRepo, jwtSvc, cfg.AccessExpiresIn, cfg.RefreshExpiresIn)
	authH := httpHandler.NewAuthHandler(authSvc)
	resetMailer := &service.ResetMailer{Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword}
	if !resetMailer.Ready() {
		log.Print("password recovery disabled: configure SMTP_PASSWORD")
	}
	resetH := httpHandler.NewPasswordResetHandler(service.NewPasswordResetService(userRepo, repository.NewPasswordResetRepository(pool), resetMailer))
	deletionRepo := repository.NewAccountDeletionRepository(pool)
	defer deletionRepo.Close()
	authSvc.SetAccountAccessGate(deletionRepo)
	cleanupRPC, err := service.NewGRPCAccountCleanup(cfg.SocialGRPCAddr, cfg.NotesGRPCAddr, cfg.InternalAPIKey)
	if err != nil {
		log.Fatalf("config account deletion gRPC: %v", err)
	}
	defer cleanupRPC.Close()
	deletionCoordinator := service.NewAccountDeletionCoordinator(cleanupRPC, deletionRepo)
	go deletionCoordinator.RunWorker(context.Background(), 15*time.Second)
	deletionH := httpHandler.NewAccountDeletionHandler(deletionCoordinator)
	authMw := middleware.NewAuthMiddleware(jwtSvc, deletionRepo)

	// Profile: Handler → Service → Repository (Sprint 1)
	// Con email de identity.users (fuente de verdad, best-effort).
	profileRepo := repository.NewProfileRepository(pool)
	profileSvc := service.NewProfileServiceWithUsers(profileRepo, userRepo)
	profileH := httpHandler.NewProfileHandler(profileSvc)

	// Drive OAuth: Handler → Service → Repository (google-drive/connect)
	oauthRepo := repository.NewOAuthRepository(pool)
	driveProvider := &service.ConfigDriveOAuthProvider{
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleClientSecret,
		RedirectURI:  cfg.GoogleRedirectURI,
	}
	driveSvc := service.NewDriveOAuthService(oauthRepo, driveProvider)
	driveH := httpHandler.NewDriveHandlerWithGoogleConfig(driveSvc, cfg.GoogleClientID, cfg.GoogleRedirectURI)

	// Calendar OAuth: Handler → Service → Repository (google-calendar/connect + interno para Social)
	calendarProvider := &service.ConfigCalendarOAuthProvider{
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleClientSecret,
		RedirectURI:  cfg.GoogleRedirectURI,
	}
	calendarSvc := service.NewCalendarOAuthService(oauthRepo, calendarProvider)
	calendarH := httpHandler.NewCalendarHandler(calendarSvc)
	internalOAuthH := httpHandler.NewInternalOAuthHandler(calendarSvc)
	usersSvc := service.NewUsersService(userRepo)
	internalUsersH := httpHandler.NewInternalUsersHandler(usersSvc)
	internalAuthH := httpHandler.NewInternalAuthHandler(jwtSvc)

	// Estado del gRPC para /health/db: si Serve muere, HTTP no debe verse
	// saludable sin señal (ver serveGRPC).
	grpcStatus := &grpcHealth{}
	// gRPC Identity en goroutine; HTTP bloquea abajo (igual que social).
	startGRPC(usersSvc, jwtSvc, driveSvc, calendarSvc, oauthRepo, cfg, grpcStatus)

	r := gin.Default()

	// Público (sin auth) según agentApiContract.md
	r.POST("/auth/register", authH.Register)
	r.POST("/auth/login", authH.Login)
	r.POST("/auth/refresh", authH.Refresh)
	r.POST("/auth/logout", authH.Logout)
	r.POST("/auth/forgot-password", resetH.Forgot)
	r.POST("/auth/reset-password", resetH.Reset)
	r.DELETE("/auth/account", authMw.RequireIdentity(), deletionH.Delete)
	r.GET("/auth/account-deletion", authMw.RequireIdentity(), deletionH.Status)
	r.POST("/auth/google-drive/connect", authMw.RequireAuth(), driveH.Connect)
	r.GET("/auth/google-drive/status", authMw.RequireAuth(), driveH.Status)
	r.DELETE("/auth/google-drive/connection", authMw.RequireAuth(), driveH.Disconnect)
	r.POST("/auth/google-calendar/connect", authMw.RequireAuth(), calendarH.Connect)
	r.GET("/auth/google-calendar/status", authMw.RequireAuth(), calendarH.Status)

	// Interno servicio-a-servicio (Social → Auth): secreto compartido, sin JWT de usuario
	internal := r.Group("/internal")
	internal.Use(middleware.RequireInternalKey(cfg.InternalAPIKey))
	{
		internal.GET("/oauth/calendar-token", internalOAuthH.GetCalendarToken)
		internal.POST("/oauth/calendar-revoked", internalOAuthH.ReportCalendarRevoked)
		internal.GET("/users/lookup", internalUsersH.Lookup)
		internal.GET("/users/:id", internalUsersH.GetByID)
		internal.POST("/auth/verify", internalAuthH.Verify)
	}

	// Protegido: middleware valida firma+expiración, inyecta solo user_id (sin role global)
	// 401 token inválido/expirado
	protected := r.Group("")
	protected.Use(authMw.RequireAuth())
	{
		protected.GET("/auth/me", func(c *gin.Context) {
			uid, _ := middleware.GetUserID(c)
			c.JSON(http.StatusOK, gin.H{"user_id": uid})
		})
		protected.GET("/profile/me", profileH.GetProfile)
		protected.PATCH("/profile/me", profileH.PatchProfile)
	}

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "UP",
			"service": "auth-service",
		})
	})

	// Config pública de Google OAuth para el front: el client ID es público
	// por diseño (viaja en URLs visibles); el secret jamás sale del backend.
	// Así ningún cliente necesita --dart-define ni IDs quemados.
	r.GET("/auth/google-config", driveH.GetGoogleConfig)

	r.GET("/health/db", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "DOWN",
				"error": gin.H{
					"code":    "db_unreachable",
					"message": err.Error(),
				},
			})
			return
		}
		var now time.Time
		var db, user string
		_ = pool.QueryRow(ctx, "select now(), current_database(), current_user").Scan(&now, &db, &user)
		grpcServing, grpcErr := grpcStatus.snapshot()
		c.JSON(http.StatusOK, gin.H{
			"status":   "UP",
			"database": db,
			"user":     user,
			"now":      now.UTC().Format(time.RFC3339),
			"checks": gin.H{
				"identity_users": cnt,
				"grpc":           gin.H{"serving": grpcServing, "error": grpcErr},
			},
		})
	})

	addr := ":" + cfg.Port
	log.Printf("Servidor de Auth corriendo en http://localhost%s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Error al iniciar el servidor: %v", err)
	}
}

// grpcHealth conserva si el gRPC sigue sirviendo para exponerlo en
// /health/db (campo aditivo "grpc", sin cambiar los status HTTP). Así un
// fallo del Serve no deja a HTTP informando salud sin señal.
type grpcHealth struct {
	mu      sync.Mutex
	serving bool
	errMsg  string
}

func (h *grpcHealth) setServing() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.serving = true
	h.errMsg = ""
}

func (h *grpcHealth) setFailed(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.serving = false
	h.errMsg = err.Error()
}

func (h *grpcHealth) setStopped() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.serving = false
}

func (h *grpcHealth) snapshot() (bool, string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.serving, h.errMsg
}

// startGRPC sirve IdentityService (usuarios, verify y OAuth) para Social y
// Notes. Logging va afuera para auditar también los rechazos de clave.
func startGRPC(usersSvc *service.UsersService, jwtSvc *service.JWTService, driveSvc *service.DriveOAuthService, calendarSvc *service.CalendarOAuthService, oauthRepo *repository.OAuthRepository, cfg *config.Config, health *grpcHealth) {
	if cfg.InternalAPIKey == "" {
		log.Printf("gRPC identity: INTERNAL_API_KEY vacía, todas las llamadas fallarán (fail-closed)")
	}
	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Fatalf("Error gRPC (puerto %s ocupado?): %v", cfg.GRPCPort, err)
	}
	s := grpc.NewServer(grpc.ChainUnaryInterceptor(
		interceptor.NewLogging(log.Default()),
		interceptor.NewInternalAuth(cfg.InternalAPIKey),
	))
	identityv1.RegisterIdentityServiceServer(s, grpchandler.NewIdentityServer(usersSvc, jwtSvc, driveSvc, calendarSvc, oauthRepo))
	go func() {
		log.Printf("Servidor gRPC de Identity en :%s", cfg.GRPCPort)
		serveGRPC(s, lis, cfg.GRPCPort, health)
	}()
}

// serveGRPC bloquea sirviendo y deja constancia del resultado en health para
// /health/db. Un Serve que termina (p. ej. Stop) queda registrado con su
// error en vez de morir en silencio; HTTP no se detiene por esto.
func serveGRPC(s *grpc.Server, lis net.Listener, port string, health *grpcHealth) {
	health.setServing()
	if err := s.Serve(lis); err != nil {
		health.setFailed(err)
		log.Printf("gRPC detenido en :%s: %v", port, err)
		return
	}
	// No debería pasar (Serve solo vuelve con error), pero nunca mentir.
	health.setStopped()
	log.Printf("gRPC detenido en :%s", port)
}
