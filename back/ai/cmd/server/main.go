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
	"github.com/kurovoxx/tallerintegracion3/back/ai/internal/service"
)

func main() {
	cfg := config.Load()
	log.Printf("config: PORT=%s GEMINI_API_KEY set=%v INTERNAL_API_KEY set=%v", cfg.Port, cfg.GeminiAPIKey != "", cfg.InternalAPIKey != "")

	// Servicio de IA: modo mock por defecto (scaffold).
	// La integración real con Gemini se conecta en la tarea 4_5_6.
	aiSvc := service.NewMockAIService()

	r := gin.Default()

	// Health checks.
	r.GET("/health", httpHandler.NewHealthHandler().Health)
	r.GET("/health/db", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "UP",
			"service": "ai-service",
			"note":    "sin dependencia obligatoria de Postgres (scaffold)",
		})
	})

	// Rutas de IA (público por ahora; se añadirá auth interno en el futuro).
	ai := r.Group("/ai")
	{
		ai.POST("/generate", httpHandler.NewAIHandler(aiSvc).Generate)
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
