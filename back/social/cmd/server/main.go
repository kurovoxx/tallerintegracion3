package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	httpHandler "github.com/kurovoxx/tallerintegracion3/back/social/internal/handler/http"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

func AuthMiddleware() gin.HandlerFunc {
	allowAnon := strings.EqualFold(strings.TrimSpace(os.Getenv("ALLOW_ANON_MOCK")), "true")
	return func(c *gin.Context) {
		userID := strings.TrimSpace(c.GetHeader("X-User-Id"))
		if userID == "" {
			if auth := strings.TrimSpace(c.GetHeader("Authorization")); strings.HasPrefix(auth, "Bearer ") {
				userID = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
			}
		}
		if userID == "" {
			if allowAnon {
				userID = "00000000-0000-0000-0000-000000000000"
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized: missing X-User-Id or Authorization header", "code": "unauthorized"})
				c.Abort()
				return
			}
		}
		if _, err := uuid.Parse(userID); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user ID format in header", "code": "unauthorized"})
			c.Abort()
			return
		}
		c.Set("user_id", userID)
		c.Next()
	}
}

func corsMiddleware() gin.HandlerFunc {
	origin := strings.TrimSpace(os.Getenv("FRONT_ORIGIN"))
	if origin == "" {
		origin = "*"
	}
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-User-Id")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func loadEnv() {
	candidates := []string{".env", "../.env", "../../.env", "../../../.env"}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			_ = godotenv.Load(p)
			break
		}
	}
	_ = godotenv.Load()
}

func main() {
	loadEnv()

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		log.Println("WARN: DATABASE_URL no seteado — usando Postgres local (para Supabase setea DATABASE_URL con el pooler :6543 y sslmode=require)")
		databaseURL = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	poolCfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		log.Fatalf("DATABASE_URL inválida: %v", err)
	}
	poolCfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	poolCfg.MaxConns = 5
	poolCfg.MinConns = 1

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		log.Fatalf("No se pudo crear el pool de Postgres: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("No se pudo conectar a Postgres (DATABASE_URL): %v", err)
	}
	log.Println("Postgres conectado (social service)")

	var groupsCount int
	if err := pool.QueryRow(ctx, "select count(*) from social.groups").Scan(&groupsCount); err != nil {
		log.Fatalf("DB check social.groups falló — ¿corriste db/schema.sql en Supabase? %v", err)
	}
	log.Printf("DB check: social.groups count=%d", groupsCount)

	todoRepo := repository.NewTodoRepository(pool)
	todoSvc := service.NewTodoService(todoRepo)
	todoH := httpHandler.NewTodoHandler(todoSvc)

	sprintRepo := repository.NewSprintRepository(pool)
	sprintSvc := service.NewSprintService(sprintRepo)
	sprintH := httpHandler.NewSprintHandler(sprintSvc)

	r := gin.Default()
	r.Use(corsMiddleware())

	protected := r.Group("")
	protected.Use(AuthMiddleware())
	{
		protected.POST("/groups/:id/todo", todoH.CreateTodo)
		protected.GET("/groups/:id/todo", todoH.ListTodos)
		protected.PATCH("/groups/:id/todo", todoH.UpdateTodo)
		protected.DELETE("/groups/:id/todo", todoH.DeleteTodo)
		protected.POST("/groups/:id/sprint-sheet", sprintH.CreateSprintTask)
		protected.GET("/groups/:id/sprint-sheet", sprintH.ListSprintTasks)
		protected.PATCH("/groups/:id/sprint-sheet", sprintH.UpdateSprintTask)
		protected.DELETE("/groups/:id/sprint-sheet", sprintH.DeleteSprintTask)
	}

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "UP",
			"service": "social-service",
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
				"social_groups": groupsCount,
			},
		})
	})

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8081"
	}

	log.Printf("Servidor de Social corriendo en http://localhost:%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Error al iniciar el servidor: %v", err)
	}
}
