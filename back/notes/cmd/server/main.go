package main

import (
	"context"
	"log"
	"net/http"
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

	driveClient := drive.NewMockClient()

	if cfg.DatabaseURL == "" {
		log.Println("notes: DATABASE_URL vacío — levantando con stores en memoria (modo mock)")
		runServerMemory(driveClient, cfg)
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
	runServer(p, driveClient, cfg)
}

func runServer(pool *pgxpool.Pool, driveClient drive.Client, cfg *config.Config) {
	noteStore := service.NewPGNoteStore(pool)
	attStore := service.NewPGAttachmentStore(pool)
	savedStore := service.NewPGSavedStore(pool)
	likeStore := service.NewPGLikeStore(pool)
	sharedStore := service.NewPGSharedStore(pool)
	social := service.NewMemorySocialResolver() // TODO: reemplazar por gRPC a Social
	svc := service.NewNoteService(noteStore, attStore, savedStore, likeStore, sharedStore, driveClient, social)
	startGin(svc, cfg)
}

func runServerMemory(driveClient drive.Client, cfg *config.Config) {
	noteStore := service.NewMemoryNoteStore()
	attStore := service.NewMemoryAttachmentStore()
	savedStore := service.NewMemorySavedStore()
	likeStore := service.NewMemoryLikeStore()
	sharedStore := service.NewMemorySharedStore()
	social := service.NewMemorySocialResolver()

	svc := service.NewNoteService(noteStore, attStore, savedStore, likeStore, sharedStore, driveClient, social)
	startGin(svc, cfg)
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
		protected.GET("/notes/:id/access", h.GetAccess)
		protected.POST("/notes/:id/attachments", h.UploadAttachment)
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
	if err := r.Run(addr); err != nil {
		log.Fatalf("Error iniciando notes service: %v", err)
	}
}
