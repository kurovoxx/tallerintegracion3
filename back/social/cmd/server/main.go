package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/calendar"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/config"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/discord"
	httpHandler "github.com/kurovoxx/tallerintegracion3/back/social/internal/handler/http"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/stream"
)

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
	streamRepo := repository.NewStreamRepository(pool)
	groupSvc := service.NewGroupService(groupRepo, streamNotifierFor(cfg, streamRepo))

	todoRepo := repository.NewTodoRepository(pool)
	todoSvc := service.NewTodoService(todoRepo)
	todoH := httpHandler.NewTodoHandler(todoSvc)

	sprintRepo := repository.NewSprintRepository(pool)
	sprintSvc := service.NewSprintService(sprintRepo)
	sprintH := httpHandler.NewSprintHandler(sprintSvc)

	hoursRepo := repository.NewHoursRepository(pool)
	hoursSvc := service.NewHoursService(hoursRepo)
	hoursH := httpHandler.NewHoursHandler(hoursSvc)

	meetingRepo := repository.NewMeetingRepository(pool)
	discordRepo := repository.NewDiscordRepository(pool)
	discordSvc := service.NewDiscordService(discordRepo)
	discordH := httpHandler.NewDiscordHandler(discordSvc)
	meetingSvc := service.NewMeetingService(meetingRepo,
		service.NewMultiMeetingNotifier(
			calendarNotifierFor(cfg, meetingRepo),
			service.NewDiscordMeetingNotifier(discordRepo, discord.NewWebhookClient()),
		))
	meetingH := httpHandler.NewMeetingHandler(meetingSvc)

	viewSvc := service.NewViewService(groupSvc, todoSvc, sprintSvc, meetingSvc)
	viewH := httpHandler.NewViewHandler(groupSvc, viewSvc)

	startGin(groupSvc, todoH, sprintH, hoursH, meetingH, discordH, viewH, pool, cfg)
}

func calendarNotifierFor(cfg *config.Config, meetingRepo *repository.MeetingRepository) service.MeetingCreatedNotifier {
	// CALENDAR_MODE=mock (default): sync simulado, sin llamar a Google ni a Auth.
	// Persiste un google_calendar_event_id falso para verificar el cableado E2E.
	if strings.ToLower(strings.TrimSpace(cfg.CalendarMode)) != "real" {
		log.Printf("calendar sync: modo mock (CALENDAR_MODE=%s)", cfg.CalendarMode)
		return service.NewCalendarMeetingNotifier(
			&service.StubCalendarGateway{},
			calendar.NewMockClient(),
			meetingRepo,
		)
	}
	// CALENDAR_MODE=real: token vía Auth interno + API REST de Google.
	log.Printf("calendar sync: modo real contra Auth %s", cfg.AuthBaseURL)
	return service.NewCalendarMeetingNotifier(
		&service.AuthCalendarGateway{BaseURL: cfg.AuthBaseURL, InternalKey: cfg.InternalKey},
		calendar.NewRESTClient(),
		meetingRepo,
	)
}

func streamNotifierFor(cfg *config.Config, streamRepo *repository.StreamRepository) service.GroupCreatedNotifier {
	// STREAM_MODE=mock (default): canal simulado, sin llamar a Stream.
	// Persiste un channel_id falso para verificar el cableado E2E.
	if strings.ToLower(strings.TrimSpace(cfg.StreamMode)) != "real" {
		log.Printf("stream sync: modo mock (STREAM_MODE=%s)", cfg.StreamMode)
		return service.NewStreamChannelNotifier(streamRepo, stream.NewMockClient())
	}
	// STREAM_MODE=real: creación vía API de Stream Chat con API key/secret.
	log.Printf("stream sync: modo real")
	return service.NewStreamChannelNotifier(
		streamRepo,
		stream.NewRESTClient(cfg.StreamAPIKey, cfg.StreamSecret),
	)
}

func startGin(groupSvc *service.GroupService, todoH *httpHandler.TodoHandler, sprintH *httpHandler.SprintHandler, hoursH *httpHandler.HoursHandler, meetingH *httpHandler.MeetingHandler, discordH *httpHandler.DiscordHandler, viewH *httpHandler.ViewHandler, pool *pgxpool.Pool, cfg *config.Config) {
	validator := &middleware.SimpleHS256Validator{
		Secret:   []byte(cfg.JWTSecret),
		Issuer:   "apuntes-auth",
		Audience: "apuntes-client",
	}
	authMw := middleware.NewAuthMiddleware(validator)
	groupHandler := httpHandler.NewGroupHandler(groupSvc)

	r := gin.Default()
	r.Use(corsMiddleware())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP", "service": "social-service"})
	})

	r.GET("/health/db", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "DOWN",
				"error":  gin.H{"code": "db_unreachable", "message": err.Error()},
			})
			return
		}
		var now time.Time
		var db, user string
		_ = pool.QueryRow(ctx, "select now(), current_database(), current_user").Scan(&now, &db, &user)
		var cnt int
		_ = pool.QueryRow(ctx, "select count(*) from social.groups").Scan(&cnt)
		c.JSON(http.StatusOK, gin.H{
			"status":   "UP",
			"database": db,
			"user":     user,
			"now":      now.UTC().Format(time.RFC3339),
			"checks":   gin.H{"social_groups": cnt},
		})
	})

	protected := r.Group("")
	protected.Use(authMw.RequireAuth())
	{
		groupHandler.RegisterRoutes(protected)
		viewH.RegisterRoutes(protected) // payloads de vistas (2_3_14 y 2_3_15)

		protected.POST("/groups/:id/todo", todoH.CreateTodo)
		protected.GET("/groups/:id/todo", todoH.ListTodos)
		protected.PATCH("/groups/:id/todo", todoH.UpdateTodo)
		protected.DELETE("/groups/:id/todo", todoH.DeleteTodo)
		protected.POST("/groups/:id/sprint-sheet", sprintH.CreateSprintTask)
		protected.GET("/groups/:id/sprint-sheet", sprintH.ListSprintTasks)
		protected.PATCH("/groups/:id/sprint-sheet", sprintH.UpdateSprintTask)
		protected.DELETE("/groups/:id/sprint-sheet", sprintH.DeleteSprintTask)
		protected.POST("/sprint-sheet/:taskId/hours", hoursH.LogHours)
		protected.GET("/sprint-sheet/:taskId/hours", hoursH.ListHours)
		protected.POST("/groups/:id/meetings", meetingH.CreateMeeting)
		protected.PUT("/groups/:id/discord-config", discordH.PutConfig)
	}

	addr := ":" + cfg.Port
	log.Printf("Social service corriendo en http://localhost%s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Error iniciando social service: %v", err)
	}
}
