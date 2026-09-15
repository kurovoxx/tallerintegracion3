package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/config"
	httpHandler "github.com/kurovoxx/tallerintegracion3/back/social/internal/handler/http"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

func main() {
	cfg := config.Load()
	log.Printf("config social: DATABASE_URL set=%v PORT=%s", cfg.DatabaseURL != "", cfg.Port)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := repository.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("No se pudo conectar a Postgres: %v", err)
	}
	defer pool.Close()
	log.Println("Postgres conectado OK (social)")

	var cnt int
	if err := pool.QueryRow(ctx, "select count(*) from social.groups").Scan(&cnt); err != nil {
		log.Printf("check social.groups: %v (schema puede no existir aún — corre agentSql.md)", err)
	} else {
		log.Printf("DB check: social.groups count=%d", cnt)
	}

	runServer(pool, cfg)
}

func runServer(pool *pgxpool.Pool, cfg *config.Config) {
	groupRepo := repository.NewGroupRepository(pool)
	// TODO(martín, 2_5_14): reemplazar por un notifier real que cree el canal
	// de Stream al crear el grupo. Mientras tanto no-op — ver comentario en
	// service.GroupCreatedNotifier.
	groupSvc := service.NewGroupService(groupRepo, nil)

	startGin(groupSvc, cfg)
}

func startGin(groupSvc *service.GroupService, cfg *config.Config) {
	validator := &middleware.SimpleHS256Validator{
		Secret:   []byte(cfg.JWTSecret),
		Issuer:   "apuntes-auth",
		Audience: "apuntes-client",
	}
	authMw := middleware.NewAuthMiddleware(validator)
	groupHandler := httpHandler.NewGroupHandler(groupSvc)

	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP", "service": "social-service"})
	})

	protected := r.Group("")
	protected.Use(authMw.RequireAuth())
	{
		protected.POST("/groups", groupHandler.Create)
		// Próximas tareas del backlog de Benjamín (2_3_2 en adelante) se
		// registran acá a medida que se implementan:
		protected.GET("/groups/me", groupHandler.ListMy)
		protected.GET("/groups/:id", groupHandler.Get)
		protected.POST("/groups/:id/join", groupHandler.Join)
		protected.POST("/groups/:id/invite/regenerate", groupHandler.RegenerateInvite)
		protected.GET("/groups/:id/members", groupHandler.ListMembers)

		protected.POST("/groups/:id/members/:user_id/kick", groupHandler.KickMember)
		protected.POST("/groups/:id/members/:user_id/ban", groupHandler.BanMember)
		// protected.PATCH("/groups/:id/members/:userId/role", groupHandler.SetRole)
		// protected.POST("/groups/:id/transfer-admin", groupHandler.TransferAdmin)
		// protected.POST("/groups/:id/leave", groupHandler.Leave)
	}

	addr := ":" + cfg.Port
	log.Printf("Social service corriendo en http://localhost%s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Error iniciando social service: %v", err)
	}
}
