package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/ai/internal/config"
	httpHandler "github.com/kurovoxx/tallerintegracion3/back/ai/internal/handler/http"
	"github.com/kurovoxx/tallerintegracion3/back/ai/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/ai/internal/service"
)

func main() {
	cfg := config.Load()
	log.Printf("config: PORT=%s GEMINI_API_KEY set=%v INTERNAL_API_KEY set=%v", cfg.Port, cfg.GeminiAPIKey != "", cfg.InternalAPIKey != "")

	// Servicio de IA: si GEMINI_API_KEY está configurada, usa Gemini;
	// de lo contrario, usa MockAIService (modo mock para desarrollo/sin clave).
	var aiSvc service.AIService
	if cfg.GeminiAPIKey != "" {
		aiSvc = service.NewGeminiAIService(cfg)
		log.Print("IA: modo Gemini activado (GEMINI_API_KEY configurada)")
	} else {
		aiSvc = service.NewMockAIService()
		log.Print("IA: modo mock activado (GEMINI_API_KEY no configurada)")
	}

	// Middleware de auth JWT para endpoints de IA.
	authMw := middleware.NewAuthMiddleware(middleware.NewConfigValidator(cfg))

	// Rate limiter: 5 conversiones por hora por usuario.
	rateLimiter := service.NewRateLimiter(cfg.GeminiRateLimit)

	r := gin.Default()

	// Health checks (público, sin auth).
	r.GET("/health", httpHandler.NewHealthHandler().Health)
	r.GET("/health/db", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "UP",
			"service": "ai-service",
			"note":    "sin dependencia obligatoria de Postgres (scaffold)",
		})
	})

	// Rutas de IA (requieren JWT de usuario + rate limit).
	ai := r.Group("/ai")
	ai.Use(authMw.RequireAuth())
	{
		ai.POST("/generate", httpHandler.NewAIHandler(aiSvc).Generate)
		ai.POST("/pdf-to-markdown", httpHandler.NewPDFHandler(aiSvc, authMw, rateLimiter).ConvertPDFToMarkdown)
	}

	addr := ":" + cfg.Port
	log.Printf("Servidor de IA corriendo en http://localhost%s", addr)
	httpSrv := &http.Server{Addr: addr, Handler: r}

	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error HTTP: %v", err)
		}
	}()

	// Apagado gracioso.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Print("apagando servidor de IA...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Printf("apagado HTTP: %v", err)
	}
	log.Print("apagado OK")
}
