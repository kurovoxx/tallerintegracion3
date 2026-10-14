package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/calendar"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/config"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/discord"
	socialv1 "github.com/kurovoxx/tallerintegracion3/back/social/internal/gen/socialv1"
	grpcHandler "github.com/kurovoxx/tallerintegracion3/back/social/internal/handler/grpc"
	httpHandler "github.com/kurovoxx/tallerintegracion3/back/social/internal/handler/http"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/interceptor"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/stream"
	"google.golang.org/grpc"
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
	// Directorio Identity para nombres en GET /groups/{id}/members
	// (best-effort: sin Auth responde solo IDs, sin romper).
	groupSvc.SetUserDirectory(service.NewHTTPUserDirectory(cfg.AuthBaseURL, cfg.InternalKey))

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
			service.NewStreamMessageNotifier(streamRepo, streamClientFor(cfg)),
		))
	meetingH := httpHandler.NewMeetingHandler(meetingSvc)
	notifH := httpHandler.NewNotificationHandler(service.NewNotificationService(meetingRepo))

	streamTokenSvc := service.NewStreamTokenServiceWithKey(streamRepo, cfg.StreamSecret, cfg.StreamAPIKey)
	// Sincronización server-side de membresía del canal al emitir tokens:
	// MockClient en modo mock (sin red), RESTClient en modo real.
	streamTokenSvc.SetClient(streamClientFor(cfg))
	streamH := httpHandler.NewStreamHandler(streamTokenSvc)

	startGRPC(groupSvc, cfg)

	viewSvc := service.NewViewService(groupSvc, todoSvc, sprintSvc, meetingSvc)
	viewH := httpHandler.NewViewHandler(groupSvc, viewSvc)

	startGin(groupSvc, todoH, sprintH, hoursH, meetingH, notifH, discordH, viewH, streamH, pool, cfg)
}

// startGRPC sirve SocialService (membresía/admin/limpieza) para Notes e
// Identity. Logging va afuera para auditar también los rechazos de clave.
func startGRPC(groupSvc *service.GroupService, cfg *config.Config) {
	if cfg.InternalKey == "" {
		log.Printf("gRPC social: INTERNAL_API_KEY vacía, todas las llamadas fallarán (fail-closed)")
	}
	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Fatalf("Error gRPC (puerto %s ocupado?): %v", cfg.GRPCPort, err)
	}
	s := grpc.NewServer(grpc.ChainUnaryInterceptor(
		interceptor.NewLogging(log.Default()),
		interceptor.NewInternalAuth(cfg.InternalKey),
	))
	socialv1.RegisterSocialServiceServer(s, grpcHandler.NewSocialServer(groupSvc))
	go func() {
		log.Printf("Servidor gRPC de Social en :%s", cfg.GRPCPort)
		if err := s.Serve(lis); err != nil {
			log.Printf("gRPC detenido: %v", err)
		}
	}()
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

func streamClientFor(cfg *config.Config) stream.Client {
	// STREAM_MODE=mock (default): sin llamar a Stream.
	if strings.ToLower(strings.TrimSpace(cfg.StreamMode)) != "real" {
		return stream.NewMockClient()
	}
	return stream.NewRESTClient(cfg.StreamAPIKey, cfg.StreamSecret)
}

func streamNotifierFor(cfg *config.Config, streamRepo *repository.StreamRepository) service.GroupCreatedNotifier {
	// STREAM_MODE=mock (default): canal simulado, sin llamar a Stream.
	// Persiste un channel_id falso para verificar el cableado E2E.
	if strings.ToLower(strings.TrimSpace(cfg.StreamMode)) != "real" {
		log.Printf("stream sync: modo mock (STREAM_MODE=%s)", cfg.StreamMode)
		return service.NewStreamChannelNotifier(streamRepo, streamClientFor(cfg))
	}
	// STREAM_MODE=real: creación vía API de Stream Chat con API key/secret.
	log.Printf("stream sync: modo real")
	return service.NewStreamChannelNotifier(
		streamRepo,
		streamClientFor(cfg),
	)
}

func startGin(groupSvc *service.GroupService, todoH *httpHandler.TodoHandler, sprintH *httpHandler.SprintHandler, hoursH *httpHandler.HoursHandler, meetingH *httpHandler.MeetingHandler, notifH *httpHandler.NotificationHandler, discordH *httpHandler.DiscordHandler, viewH *httpHandler.ViewHandler, streamH *httpHandler.StreamHandler, pool *pgxpool.Pool, cfg *config.Config) {
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
		protected.PATCH("/groups/:id/todo/:taskId", todoH.UpdateTodo)
		protected.DELETE("/groups/:id/todo/:taskId", todoH.DeleteTodo)
		protected.GET("/groups/:id/sprint-sheets", sprintH.ListSheets)
		protected.POST("/groups/:id/sprint-sheets", sprintH.CreateSheet)
		protected.PATCH("/groups/:id/sprint-sheets/:sheetId", sprintH.UpdateSheet)
		protected.DELETE("/groups/:id/sprint-sheets/:sheetId", sprintH.DeleteSheet)
		protected.POST("/groups/:id/sprint-sheet", sprintH.CreateSprintTask)
		protected.GET("/groups/:id/sprint-sheet", sprintH.ListSprintTasks)
		protected.PATCH("/groups/:id/sprint-sheet/:taskId", sprintH.UpdateSprintTask)
		protected.DELETE("/groups/:id/sprint-sheet/:taskId", sprintH.DeleteSprintTask)
		protected.POST("/sprint-sheet/:taskId/hours", hoursH.LogHours)
		protected.GET("/sprint-sheet/:taskId/hours", hoursH.ListHours)
		protected.POST("/groups/:id/meetings", meetingH.CreateMeeting)
		protected.GET("/groups/:id/meetings", meetingH.ListMeetings)
		protected.GET("/notifications/me", notifH.ListMine)
		protected.POST("/notifications/:id/read", notifH.MarkRead)
		protected.GET("/groups/:id/stream-token", streamH.Token)
		discordH.RegisterRoutes(protected)
	}

	// Interno servicio-a-servicio (Notes → Social): secreto compartido, sin JWT.
	// Tarea 3_5_4: checks is_admin/member + member-emails para share.
	internalH := httpHandler.NewInternalHandler(groupSvc)
	internal := r.Group("/internal")
	internal.Use(middleware.RequireInternalKey(cfg.InternalKey))
	{
		internal.GET("/groups/:id/members", internalH.Members)
		internal.GET("/groups/:id/member-emails", internalH.MemberEmails)
	}

	addr := ":" + cfg.Port
	log.Printf("Social service corriendo en http://localhost%s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Error iniciando social service: %v", err)
	}
}
