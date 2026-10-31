package http

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

// Integración 3_5_14: notificaciones + próximas reuniones contra Postgres real.
// Requiere SOCIAL_TEST_DATABASE_URL; sin ella se omite.
type notifPGEnv struct {
	router *gin.Engine
	pool   *pgxpool.Pool
	groups *repository.GroupRepository
}

func newNotifPGEnv(t *testing.T) *notifPGEnv {
	t.Helper()
	dsn := os.Getenv("SOCIAL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SOCIAL_TEST_DATABASE_URL no definida: se omite el test de integración")
	}
	pool, err := repository.NewPool(context.Background(), dsn)
	if err != nil {
		t.Fatalf("conectar a la base de pruebas: %v", err)
	}
	t.Cleanup(pool.Close)

	gin.SetMode(gin.TestMode)
	groups := repository.NewGroupRepository(pool)
	meetRepo := repository.NewMeetingRepository(pool)
	meetingH := NewMeetingHandler(service.NewMeetingService(meetRepo, service.NoopMeetingCreatedNotifier{}))
	notifH := NewNotificationHandler(service.NewNotificationService(meetRepo))

	r := gin.New()
	protected := r.Group("")
	protected.Use(middleware.NewAuthMiddleware(stubValidator{}).RequireAuth())
	protected.POST("/groups/:id/meetings", meetingH.CreateMeeting)
	protected.GET("/groups/:id/meetings", meetingH.ListMeetings)
	protected.GET("/notifications/me", notifH.ListMine)
	protected.POST("/notifications/:id/read", notifH.MarkRead)
	return &notifPGEnv{router: r, pool: pool, groups: groups}
}

func (e *notifPGEnv) groupWithMeeting(t *testing.T, admin, member string) (gid, mid string) {
	t.Helper()
	g, err := e.groups.CreateWithOwner(context.Background(), "it-nt-"+uuid.NewString()[:8], nil, admin)
	if err != nil {
		t.Fatalf("crear grupo: %v", err)
	}
	t.Cleanup(func() { _ = e.groups.DeleteGroup(context.Background(), g.ID) })
	full, err := e.groups.GetByID(context.Background(), g.ID)
	if err != nil {
		t.Fatalf("leer grupo: %v", err)
	}
	if err := e.groups.JoinWithInviteToken(context.Background(), g.ID, full.InviteToken, member); err != nil {
		t.Fatalf("join: %v", err)
	}
	w := doMartinReq(e.router, http.MethodPost, "/groups/"+g.ID+"/meetings", admin, map[string]any{
		"title": "PG-Reunión", "scheduled_at": "2026-11-20T15:00:00Z",
	})
	expect(t, w, http.StatusCreated)
	mid, _ = decode(t, w)["meeting_id"].(string)
	return g.ID, mid
}

func TestNotifPG_FlowLectura(t *testing.T) {
	e := newNotifPGEnv(t)
	admin, member := uuid.NewString(), uuid.NewString()
	gid, _ := e.groupWithMeeting(t, admin, member)

	// El miembro tiene 1 no leída con datos de la reunión.
	w := doMartinReq(e.router, http.MethodGet, "/notifications/me", member, nil)
	expect(t, w, http.StatusOK)
	list, _ := decode(t, w)["data"].([]any)
	if len(list) != 1 {
		t.Fatalf("se esperaba 1 notificación, got %s", w.Body.String())
	}
	first := list[0].(map[string]any)
	if first["read"] != false || first["title"] != "PG-Reunión" {
		t.Fatalf("no leída con título esperado, got %v", first)
	}
	nid, _ := first["id"].(string)

	// El creador no tiene (fan-out excluye al creador).
	w = doMartinReq(e.router, http.MethodGet, "/notifications/me", admin, nil)
	expect(t, w, http.StatusOK)
	if len(decode(t, w)["data"].([]any)) != 0 {
		t.Fatalf("el creador no debe tener notificaciones: %s", w.Body.String())
	}

	// Marcar leída persiste read_at; segunda vez → 404.
	w = doMartinReq(e.router, http.MethodPost, "/notifications/"+nid+"/read", member, nil)
	expect(t, w, http.StatusOK)
	var readAt string
	if err := e.pool.QueryRow(context.Background(),
		`SELECT read_at::text FROM social.meeting_notifications WHERE id = $1`, nid).Scan(&readAt); err != nil || readAt == "" {
		t.Fatalf("read_at debe quedar seteado: %q err=%v", readAt, err)
	}
	expect(t, doMartinReq(e.router, http.MethodPost, "/notifications/"+nid+"/read", member, nil), http.StatusNotFound)
	// La ajena (admin marca la del miembro) → 404.
	expect(t, doMartinReq(e.router, http.MethodPost, "/notifications/"+nid+"/read", admin, nil), http.StatusNotFound)

	// Ventana próximas: el miembro ve la futura.
	w = doMartinReq(e.router, http.MethodGet, "/groups/"+gid+"/meetings", member, nil)
	expect(t, w, http.StatusOK)
	mlist, _ := decode(t, w)["data"].([]any)
	if len(mlist) != 1 || mlist[0].(map[string]any)["title"] != "PG-Reunión" {
		t.Fatalf("se esperaba 1 próxima, got %s", w.Body.String())
	}
}
