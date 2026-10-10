package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/config"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	httpHandler "github.com/kurovoxx/tallerintegracion3/back/notes/internal/handler/http"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/service"
)

func main() {
	cfg := config.Load()
	log.Printf("config notes: DATABASE_URL set=%v PORT=%s STORAGE=%s", cfg.DatabaseURL != "", cfg.Port, cfg.StorageMode)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if cfg.DatabaseURL == "" {
		log.Println("notes: DATABASE_URL vacío — levantando con stores en memoria (modo mock)")
		if strings.EqualFold(strings.TrimSpace(cfg.StorageMode), "drive") {
			log.Println("notes: ADVERTENCIA STORAGE_MODE=drive sin DATABASE_URL — sin identity.oauth_connections no hay tokens, se usa mock")
		}
		runServerMemory(drive.NewMockClient(), cfg)
		return
	}

	p, err := repository.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("No se pudo conectar a Postgres: %v", err)
	}
	defer p.Close()
	log.Println("Postgres conectado OK (notes)")
	var cnt int
	if err := p.QueryRow(ctx, "select count(*) from notes.notes").Scan(&cnt); err != nil {
		log.Printf("check notes.notes: %v (schema puede no existir aún)", err)
	} else {
		log.Printf("DB check: notes.notes count=%d", cnt)
	}
	runServer(p, selectDriveClient(p, cfg), cfg)
}

// selectDriveClient elige el cliente Drive según STORAGE_MODE.
// Con "drive" usa la API oficial (tokens por usuario desde identity.oauth_connections);
// en cualquier otro caso (o sin BD) usa el mock en memoria.
func selectDriveClient(p *pgxpool.Pool, cfg *config.Config) drive.Client {
	if !strings.EqualFold(strings.TrimSpace(cfg.StorageMode), "drive") {
		log.Printf("notes: Drive mock activado (STORAGE_MODE=%s)", cfg.StorageMode)
		return drive.NewMockClient()
	}
	if cfg.GoogleClientID == "" || cfg.GoogleClientSecret == "" {
		log.Println("notes: STORAGE_MODE=drive con GOOGLE_CLIENT_ID/SECRET vacíos — el refresh de tokens queda deshabilitado")
	}
	log.Println("notes: Drive real activado (STORAGE_MODE=drive, drive/v3, tokens desde identity.oauth_connections)")
	return drive.NewRealDriveClient(drive.NewPGOAuthTokenStore(p, cfg.GoogleClientID, cfg.GoogleClientSecret))
}

func runServer(pool *pgxpool.Pool, driveClient drive.Client, cfg *config.Config) {
	noteStore := service.NewPGNoteStore(pool)
	attStore := service.NewPGAttachmentStore(pool)
	savedStore := service.NewPGSavedStore(pool)
	likeStore := service.NewPGLikeStore(pool)
	sharedStore := service.NewPGSharedStore(pool)
	social, members := newSocialAdapters(cfg)
	svc := service.NewNoteService(noteStore, attStore, savedStore, likeStore, sharedStore, driveClient, social)
	// Directorio de correos para share restricted: adaptador HTTP real a Social
	// (o memoria solo con DEV_SEED_SOCIAL explícito).
	svc.SetMemberDirectory(members)
	startGin(svc, cfg)
}

func runServerMemory(driveClient drive.Client, cfg *config.Config) {
	noteStore := service.NewMemoryNoteStore()
	attStore := service.NewMemoryAttachmentStore()
	savedStore := service.NewMemorySavedStore()
	likeStore := service.NewMemoryLikeStore()
	likeStore.SetNoteStore(noteStore)
	sharedStore := service.NewMemorySharedStore()
	social, members := newSocialAdapters(cfg)

	svc := service.NewNoteService(noteStore, attStore, savedStore, likeStore, sharedStore, driveClient, social)
	svc.SetMemberDirectory(members)
	startGin(svc, cfg)
}

// newSocialAdapters inyecta los adaptadores de Social: clientes HTTP reales
// contra SOCIAL_SERVICE_URL con timeout y degradación controlada. No hay
// adaptadores mock fijos en producción; los resolvers en memoria solo viven en
// los tests (service/store_memory.go).
func newSocialAdapters(cfg *config.Config) (service.SocialResolver, service.GroupMemberDirectory) {
	adapter := service.NewSocialHTTPAdapter(cfg.SocialServiceURL, cfg.SocialTimeout, cfg.InternalAPIKey)
	log.Printf("notes: adaptadores Social HTTP activados (base=%s timeout=%s)", cfg.SocialServiceURL, cfg.SocialTimeout)
	return adapter, adapter
}

// startGin inicia Gin con middleware y rutas según agentApiContract.md
func startGin(svc *service.NoteService, cfg *config.Config) {
	// JWT validator HS256
	validator := &middleware.SimpleHS256Validator{
		Secret:   []byte(cfg.JWTSecret),
		Issuer:   "apuntes-auth",
		Audience: "apuntes-client",
	}
	authMw := middleware.NewAuthMiddleware(validator)
	h := httpHandler.NewNoteHandler(svc)

	r := gin.Default()

	// health
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP", "service": "notes-service"})
	})
	r.GET("/health/drive", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"storage_mode": cfg.StorageMode})
	})

	// rutas protegidas - estáticas primero para evitar conflicto con :id
	protected := r.Group("")
	protected.Use(authMw.RequireAuth())
	{
		protected.POST("/notes", h.Create)
		protected.GET("/notes/me", h.ListMy)
		protected.POST("/notes/unshare-all", h.UnshareAll)
		protected.POST("/notes/reconcile", h.Reconcile)
		protected.POST("/notes/upload", h.UploadFile)
		protected.GET("/notes/:id/access", h.GetAccess)
		protected.POST("/notes/:id/attachments", h.UploadAttachment)
		protected.GET("/notes/:id/attachments/:attachmentId/content", h.AttachmentContent)
		protected.DELETE("/notes/:id/attachments/:attachmentId", h.DeleteAttachment)
		protected.POST("/notes/:id/save", h.Save)
		protected.POST("/notes/:id/copy", h.Copy)
		protected.POST("/notes/:id/like", h.Like)
		protected.DELETE("/notes/:id/like", h.Unlike)
		protected.POST("/notes/:id/share", h.Share)
		protected.PATCH("/notes/:id", h.Patch)
		protected.DELETE("/notes/:id", h.Delete)
		protected.GET("/notes/:id", h.Get)
		protected.DELETE("/notes/shared/:sharedNoteId", h.Unshare)
		protected.GET("/groups/:id/notes", h.ListGroupNotes)
	}

	addr := ":" + cfg.Port
	log.Printf("Notes service corriendo en http://localhost%s", addr)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	interval := cfg.ReconcileInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	reconcileDone := make(chan struct{})
	go func() {
		defer close(reconcileDone)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := svc.ReconcilePendingNotes(ctx); err != nil && ctx.Err() == nil {
					log.Printf("notes: reconciliation failed: %v", err)
				}
			}
		}
	}()
	server := &http.Server{Addr: addr, Handler: r, ReadHeaderTimeout: 10 * time.Second}
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.ListenAndServe() }()
	select {
	case <-ctx.Done():
	case err := <-serverDone:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("Error iniciando notes service: %v", err)
		}
	}
	stop()
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("notes: graceful shutdown: %v", err)
		_ = server.Close()
	}
	<-reconcileDone
}
