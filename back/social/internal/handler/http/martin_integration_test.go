package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

type testRecorder = httptest.ResponseRecorder

func doMartinReq(r *gin.Engine, method, path, user string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if user != "" {
		req.Header.Set("Authorization", "Bearer "+user)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func expectStatus(t *testing.T, w *httptest.ResponseRecorder, code int) {
	t.Helper()
	expect(t, w, code)
}

func martinRespID(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	return martinDataID(t, w)
}

func martinRespListLen(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	return martinDataListLen(t, w)
}

func martinMeetingID(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	mid, _ := decode(t, w)["meeting_id"].(string)
	if mid == "" {
		t.Fatalf("sin meeting_id: %s", w.Body.String())
	}
	return mid
}

// Integración Martín: HTTP + Service + Postgres real.
// Requiere SOCIAL_TEST_DATABASE_URL con el schema de docker/postgres/init.sql.
// Sin la variable, los tests se omiten (no fallan).
type martinPGEnv struct {
	router *gin.Engine
	pool   *pgxpool.Pool
	groups *repository.GroupRepository
}

func newMartinPGEnv(t *testing.T) *martinPGEnv {
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
	todoH := NewTodoHandler(service.NewTodoService(repository.NewTodoRepository(pool)))
	sprintH := NewSprintHandler(service.NewSprintService(repository.NewSprintRepository(pool)))
	meetingH := NewMeetingHandler(service.NewMeetingService(
		repository.NewMeetingRepository(pool), service.NoopMeetingCreatedNotifier{}))

	r := gin.New()
	protected := r.Group("")
	protected.Use(middleware.NewAuthMiddleware(stubValidator{}).RequireAuth())
	protected.POST("/groups/:id/todo", todoH.CreateTodo)
	protected.GET("/groups/:id/todo", todoH.ListTodos)
	protected.PATCH("/groups/:id/todo/:taskId", todoH.UpdateTodo)
	protected.DELETE("/groups/:id/todo/:taskId", todoH.DeleteTodo)
	protected.POST("/groups/:id/sprint-sheet", sprintH.CreateSprintTask)
	protected.GET("/groups/:id/sprint-sheet", sprintH.ListSprintTasks)
	protected.PATCH("/groups/:id/sprint-sheet/:taskId", sprintH.UpdateSprintTask)
	protected.DELETE("/groups/:id/sprint-sheet/:taskId", sprintH.DeleteSprintTask)
	protected.POST("/groups/:id/meetings", meetingH.CreateMeeting)
	return &martinPGEnv{router: r, pool: pool, groups: groups}
}

// newMartinPGGroup crea grupo owner+member y lo borra al final (CASCADE limpia tareas).
func (e *martinPGEnv) newGroup(t *testing.T, owner, member string) string {
	t.Helper()
	g, err := e.groups.CreateWithOwner(context.Background(), "it-martin-"+uuid.NewString()[:8], nil, owner)
	if err != nil {
		t.Fatalf("crear grupo: %v", err)
	}
	t.Cleanup(func() { _ = e.groups.DeleteGroup(context.Background(), g.ID) })
	if err := e.groups.JoinWithInviteToken(context.Background(), g.ID, g.InviteToken, member); err != nil {
		t.Fatalf("join member: %v", err)
	}
	return g.ID
}

func (e *martinPGEnv) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}

// Todo CRUD contra Postgres: crea (auto-crea board), lista, parchea, borra.
func TestMartinIntegration_TodoCRUD_PG(t *testing.T) {
	e := newMartinPGEnv(t)
	admin, member := uuid.NewString(), uuid.NewString()
	gid := e.newGroup(t, admin, member)
	do := func(method, path, user string, body any) *testRecorder {
		return doMartinReq(e.router, method, path, user, body)
	}

	w := do(http.MethodPost, "/groups/"+gid+"/todo", member, map[string]any{"title": "PG-T1"})
	expectStatus(t, w, http.StatusCreated)
	tid := martinRespID(t, w)

	if n := e.count(t, `SELECT count(*) FROM social.todo_tasks WHERE id = $1 AND title = 'PG-T1'`, tid); n != 1 {
		t.Fatalf("la tarea debe estar en Postgres")
	}
	if n := e.count(t, `SELECT count(*) FROM social.todo_boards WHERE group_id = $1`, gid); n < 1 {
		t.Fatalf("debe auto-crearse el board del grupo")
	}

	w = do(http.MethodGet, "/groups/"+gid+"/todo", member, nil)
	expectStatus(t, w, http.StatusOK)
	if n := martinRespListLen(t, w); n != 1 {
		t.Fatalf("GET debe leer 1 fila de la DB, got %d", n)
	}

	w = do(http.MethodPatch, "/groups/"+gid+"/todo/"+tid, member, map[string]any{"status": "done"})
	expectStatus(t, w, http.StatusOK)
	if n := e.count(t, `SELECT count(*) FROM social.todo_tasks WHERE id = $1 AND status = 'done'`, tid); n != 1 {
		t.Fatalf("el PATCH debe persistir status=done")
	}

	w = do(http.MethodDelete, "/groups/"+gid+"/todo/"+tid, admin, nil)
	expectStatus(t, w, http.StatusOK)
	if n := e.count(t, `SELECT count(*) FROM social.todo_tasks WHERE id = $1`, tid); n != 0 {
		t.Fatalf("el DELETE debe borrar la fila")
	}
}

// Sprint CRUD contra Postgres: defaults, filtros y persistencia de priority/status.
func TestMartinIntegration_SprintCRUD_PG(t *testing.T) {
	e := newMartinPGEnv(t)
	admin, member := uuid.NewString(), uuid.NewString()
	gid := e.newGroup(t, admin, member)
	do := func(method, path, user string, body any) *testRecorder {
		return doMartinReq(e.router, method, path, user, body)
	}

	w := do(http.MethodPost, "/groups/"+gid+"/sprint-sheet", admin, map[string]any{"title": "PG-S1", "assigned_to": member})
	expectStatus(t, w, http.StatusCreated)
	tid := martinRespID(t, w)

	var prio, st string
	if err := e.pool.QueryRow(context.Background(),
		`SELECT priority, status FROM social.sprint_sheet_tasks WHERE id = $1`, tid).Scan(&prio, &st); err != nil {
		t.Fatalf("leer tarea sprint: %v", err)
	}
	if prio != "media" || st != "sin_empezar" {
		t.Fatalf("defaults en DB: prio=%s status=%s", prio, st)
	}

	w = do(http.MethodGet, "/groups/"+gid+"/sprint-sheet?priority=media", member, nil)
	expectStatus(t, w, http.StatusOK)
	if n := martinRespListLen(t, w); n != 1 {
		t.Fatalf("filtro priority=media debe dar 1, got %d", n)
	}

	w = do(http.MethodPatch, "/groups/"+gid+"/sprint-sheet/"+tid, admin, map[string]any{"priority": "alta", "status": "en_proceso"})
	expectStatus(t, w, http.StatusOK)
	if n := e.count(t, `SELECT count(*) FROM social.sprint_sheet_tasks WHERE id = $1 AND priority = 'alta' AND status = 'en_proceso'`, tid); n != 1 {
		t.Fatalf("el PATCH debe persistir prioridad/estado")
	}

	w = do(http.MethodDelete, "/groups/"+gid+"/sprint-sheet/"+tid, admin, nil)
	expectStatus(t, w, http.StatusOK)
	if n := e.count(t, `SELECT count(*) FROM social.sprint_sheet_tasks WHERE id = $1`, tid); n != 0 {
		t.Fatalf("el DELETE debe borrar la fila")
	}
}

// Meeting contra Postgres: reunión + attendees + fan-out de notificaciones.
func TestMartinIntegration_MeetingCreate_PG(t *testing.T) {
	e := newMartinPGEnv(t)
	admin, member := uuid.NewString(), uuid.NewString()
	gid := e.newGroup(t, admin, member)

	w := doMartinReq(e.router, http.MethodPost, "/groups/"+gid+"/meetings", admin, map[string]any{
		"title": "PG-Daily", "scheduled_at": "2026-10-20T15:00:00Z", "attendees": []string{"pg@mail.com"},
	})
	expectStatus(t, w, http.StatusCreated)
	mid := martinMeetingID(t, w)

	if n := e.count(t, `SELECT count(*) FROM social.meetings WHERE id = $1 AND title = 'PG-Daily'`, mid); n != 1 {
		t.Fatalf("la reunión debe estar en Postgres")
	}
	if n := e.count(t, `SELECT count(*) FROM social.meeting_attendees WHERE meeting_id = $1 AND email = 'pg@mail.com'`, mid); n != 1 {
		t.Fatalf("el invitado debe estar en meeting_attendees")
	}
	// Fan-out in-app: solo al otro miembro (no al creador).
	if n := e.count(t, `SELECT count(*) FROM social.meeting_notifications WHERE meeting_id = $1`, mid); n != 1 {
		t.Fatalf("se esperaba 1 notificación (solo member), got %d", n)
	}
	var notified string
	if err := e.pool.QueryRow(context.Background(),
		`SELECT user_id FROM social.meeting_notifications WHERE meeting_id = $1`, mid).Scan(&notified); err != nil || notified != member {
		t.Fatalf("notificado = %q err=%v, se esperaba member", notified, err)
	}
}
