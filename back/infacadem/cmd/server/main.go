package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/config"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/repository"
)

func main() {
	cfg := config.Load()
	log.Printf("config: DATABASE_URL set=%v PORT=%s", cfg.DatabaseURL != "", cfg.Port)

	// Pool tolerante: en scaffold (y hasta que existan tablas) la BD puede
	// no estar; se loguea advertencia y se sigue sin pool (no Fatal).
	var pool *pgxpool.Pool
	if cfg.DatabaseURL == "" {
		log.Print("DB no configurada (DATABASE_URL vacío): /health/db responderá 503")
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		var err error
		pool, err = repository.NewPool(ctx, cfg.DatabaseURL)
		cancel()
		if err != nil {
			log.Printf("advertencia: no se pudo conectar a Postgres: %v", err)
			pool = nil
		} else {
			log.Println("PostgreSQL conectado OK (pgxpool Ping)")
			// Verificación best-effort del schema academic (tolerante:
			// las tablas llegan con migraciones posteriores).
			ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
			var cnt int
			if err := pool.QueryRow(ctx2, "select count(*) from academic.curricula").Scan(&cnt); err != nil {
				log.Printf("advertencia: check academic.curricula: %v", err)
			} else {
				log.Printf("DB check: academic.curricula count=%d", cnt)
			}
			cancel2()
		}
	}
	if pool != nil {
		defer pool.Close()
	}

	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "UP",
			"service": "academic-service",
		})
	})

	r.GET("/health/db", func(c *gin.Context) {
		if pool == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "DOWN",
				"error": gin.H{
					"code":    "db_unconfigured",
					"message": "Postgres no configurado o no disponible al arrancar",
				},
			})
			return
		}
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
		})
	})

	addr := ":" + cfg.Port
	log.Printf("Servidor de Academic corriendo en http://localhost%s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Error al iniciar el servidor: %v", err)
	}
}
