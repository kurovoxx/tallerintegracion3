package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/config"
	httpHandler "github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/handler/http"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/service"
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

	// CRUD académico (100% personal): todo filtrado por user_id del token.
	// Sin pool no hay storage: las rutas responden 503 (no 404) para no
	// confundir "sin BD" con "recurso ajeno".
	authMw := middleware.NewAuthMiddleware(&middleware.SimpleHS256Validator{
		Secret:   []byte(cfg.JWTSecret),
		Issuer:   middleware.TokenIssuer,
		Audience: middleware.TokenAudience,
	})
	protected := r.Group("")
	protected.Use(authMw.RequireAuth())
	if pool != nil {
		curH := httpHandler.NewCurriculaHandler(service.NewCurriculumService(repository.NewCurriculumRepository(pool)))
		subH := httpHandler.NewSubjectsHandler(service.NewSubjectService(repository.NewSubjectRepository(pool)))
		schH := httpHandler.NewSchedulesHandler(service.NewScheduleService(repository.NewScheduleRepository(pool)))

		protected.POST("/curricula", curH.Create)
		protected.GET("/curricula", curH.List)
		protected.GET("/curricula/:id", curH.Get)
		protected.PATCH("/curricula/:id", curH.Update)
		protected.DELETE("/curricula/:id", curH.Delete)

		protected.POST("/curricula/:id/subjects", subH.Create)
		protected.GET("/curricula/:id/subjects", subH.ListByCurriculum)
		protected.GET("/subjects/:id", subH.Get)
		protected.PATCH("/subjects/:id", subH.Update)
		protected.DELETE("/subjects/:id", subH.Delete)

		protected.POST("/subjects/:id/schedules", schH.Create)
		protected.GET("/subjects/:id/schedules", schH.ListBySubject)
		protected.DELETE("/schedules/:id", schH.Delete)
	} else {
		for _, method := range []string{"GET", "POST", "PATCH", "DELETE"} {
			for _, path := range []string{"/curricula", "/curricula/:id", "/curricula/:id/subjects", "/subjects/:id", "/subjects/:id/schedules", "/schedules/:id"} {
				protected.Handle(method, path, serviceUnavailable)
			}
		}
	}

	addr := ":" + cfg.Port
	log.Printf("Servidor de Academic corriendo en http://localhost%s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Error al iniciar el servidor: %v", err)
	}
}

// serviceUnavailable responde 503 cuando el servicio arrancó sin pool
// (sin DATABASE_URL o BD caída al inicio).
func serviceUnavailable(c *gin.Context) {
	c.JSON(http.StatusServiceUnavailable, gin.H{
		"error": gin.H{
			"code":    "db_unconfigured",
			"message": "Almacenamiento no disponible",
		},
	})
}
