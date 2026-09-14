package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/config"
	httpHandler "github.com/kurovoxx/tallerintegracion3/back/auth/internal/handler/http"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
)

func main() {
	cfg := config.Load()
	log.Printf("config: DATABASE_URL set=%v DIRECT_URL set=%v SUPABASE_URL=%s JWT kid/set=%v DISCOVERY=%s",
		cfg.DatabaseURL != "", cfg.DirectURL != "", cfg.SupabaseURL, cfg.JWT != "", cfg.DiscoveryURL)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := repository.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("No se pudo conectar a Postgres (DATABASE_URL): %v", err)
	}
	defer pool.Close()
	log.Println("Postgres conectado OK (pgxpool Ping) — pooler us-east-2")

	// Verificación rápida de schemas críticos Sprint 1
	var cnt int
	if err := pool.QueryRow(ctx, "select count(*) from identity.users").Scan(&cnt); err != nil {
		log.Fatalf("query identity.users: %v", err)
	}
	log.Printf("DB check: identity.users count=%d", cnt)

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
	authMw := middleware.NewAuthMiddleware(jwtSvc)

	r := gin.Default()

	// Público (sin auth) según agentApiContract.md
	r.POST("/auth/register", authH.Register)
	r.POST("/auth/login", authH.Login)

	// Protegido: middleware valida firma+expiración, inyecta solo user_id (sin role global)
	// 401 token inválido/expirado
	protected := r.Group("")
	protected.Use(authMw.RequireAuth())
	{
		protected.GET("/auth/me", func(c *gin.Context) {
			uid, _ := middleware.GetUserID(c)
			c.JSON(http.StatusOK, gin.H{"user_id": uid})
		})
		// Ejemplo perfil protegido (placeholder Sprint 1) — sin role
		protected.GET("/profile/me", func(c *gin.Context) {
			uid, _ := middleware.GetUserID(c)
			c.JSON(http.StatusOK, gin.H{"user_id": uid, "note": "profile placeholder"})
		})
	}

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "UP",
			"service": "auth-service",
		})
	})

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
		c.JSON(http.StatusOK, gin.H{
			"status":   "UP",
			"database": db,
			"user":     user,
			"now":      now.UTC().Format(time.RFC3339),
			"checks": gin.H{
				"identity_users": cnt,
				"jwt_kid":        cfg.JWT,
				"discovery_url":  cfg.DiscoveryURL,
			},
		})
	})

	// Log de JWKS kid vs ES256 (masterprompt 3.1 usa JWT standalone; discovery valida ES256)
	if cfg.JWT != "" && cfg.DiscoveryURL != "" {
		log.Printf("JWT kid=%s — verificar contra DISCOVERY_URL %s (ES256 P-256)", cfg.JWT, cfg.DiscoveryURL)
	}

	addr := ":" + cfg.Port
	log.Printf("Servidor de Auth corriendo en http://localhost%s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Error al iniciar el servidor: %v", err)
	}
}
