package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

// E2E Martín: servidor HTTP real (httptest.NewServer) + JWT reales (HS256,
// iss=apuntes-auth aud=apuntes-client) + Postgres real. Requiere
// SOCIAL_TEST_DATABASE_URL; sin ella se omite.
// El notifier de meetings es Noop a propósito: el fan-out externo
// (Discord/Calendar/Stream) ya está cubierto por sus tests unitarios y aquí
// solo se verifica HTTP + persistencia.

const martinE2ESecret = "e2e-test-secret-martin"

type martinE2EEnv struct {
	t      *testing.T
	server *httptest.Server
	client *http.Client
	pool   *pgxpool.Pool
}

func newMartinE2EEnv(t *testing.T) *martinE2EEnv {
	t.Helper()
	dsn := os.Getenv("SOCIAL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SOCIAL_TEST_DATABASE_URL no definida: se omite el test e2e")
	}
	pool, err := repository.NewPool(context.Background(), dsn)
	if err != nil {
		t.Fatalf("conectar a la base de pruebas: %v", err)
	}

	gin.SetMode(gin.ReleaseMode)
	groupSvc := service.NewGroupService(repository.NewGroupRepository(pool), nil)
	groupH := NewGroupHandler(groupSvc)
	todoH := NewTodoHandler(service.NewTodoService(repository.NewTodoRepository(pool)))
	sprintH := NewSprintHandler(service.NewSprintService(repository.NewSprintRepository(pool)))
	meetingH := NewMeetingHandler(service.NewMeetingService(
		repository.NewMeetingRepository(pool), service.NoopMeetingCreatedNotifier{}))

	r := gin.New()
	r.Use(gin.Recovery())
	validator := &middleware.SimpleHS256Validator{
		Secret:   []byte(martinE2ESecret),
		Issuer:   "apuntes-auth",
		Audience: "apuntes-client",
	}
	protected := r.Group("")
	protected.Use(middleware.NewAuthMiddleware(validator).RequireAuth())
	groupH.RegisterRoutes(protected)
	protected.POST("/groups/:id/todo", todoH.CreateTodo)
	protected.GET("/groups/:id/todo", todoH.ListTodos)
	protected.PATCH("/groups/:id/todo/:taskId", todoH.UpdateTodo)
	protected.DELETE("/groups/:id/todo/:taskId", todoH.DeleteTodo)
	protected.POST("/groups/:id/sprint-sheet", sprintH.CreateSprintTask)
	protected.GET("/groups/:id/sprint-sheet", sprintH.ListSprintTasks)
	protected.PATCH("/groups/:id/sprint-sheet/:taskId", sprintH.UpdateSprintTask)
	protected.DELETE("/groups/:id/sprint-sheet/:taskId", sprintH.DeleteSprintTask)
	protected.POST("/groups/:id/meetings", meetingH.CreateMeeting)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	t.Cleanup(pool.Close)
	return &martinE2EEnv{t: t, server: srv, client: srv.Client(), pool: pool}
}

// mintE2EToken firma un JWT real como lo haría auth.
func (e *martinE2EEnv) mintToken(userID string) string {
	e.t.Helper()
	now := time.Now()
	claims := middleware.ClaimsPersonalizadas{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "apuntes-auth",
			Audience:  jwt.ClaimStrings{"apuntes-client"},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(martinE2ESecret))
	if err != nil {
		e.t.Fatalf("firmar JWT: %v", err)
	}
	return signed
}

// call hace una petición HTTP real al servidor e2e. token vacío = sin header.
func (e *martinE2EEnv) call(method, path, token string, body any) (int, map[string]any) {
	e.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			e.t.Fatalf("encode body: %v", err)
		}
	}
	req, err := http.NewRequest(method, e.server.URL+path, &buf)
	if err != nil {
		e.t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		e.t.Fatalf("do %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var m map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &m); err != nil {
			e.t.Fatalf("JSON inválido %q: %v", raw, err)
		}
	}
	return resp.StatusCode, m
}

func (e *martinE2EEnv) mustStatus(method, path, token string, body any, want int) map[string]any {
	e.t.Helper()
	got, m := e.call(method, path, token, body)
	if got != want {
		e.t.Fatalf("%s %s: status = %d, se esperaba %d (body %v)", method, path, got, want, m)
	}
	return m
}

// setupGroup crea grupo + miembro por HTTP real y lo borra al final (CASCADE).
func (e *martinE2EEnv) setupGroup(ownerID, memberID string) (gid, ownerTok, memberTok string) {
	e.t.Helper()
	ownerTok, memberTok = e.mintToken(ownerID), e.mintToken(memberID)
	m := e.mustStatus(http.MethodPost, "/groups", ownerTok, map[string]any{"name": "E2E Martín"}, http.StatusCreated)
	gid, _ = m["group_id"].(string)
	if gid == "" {
		e.t.Fatalf("POST /groups sin group_id: %v", m)
	}
	g := e.mustStatus(http.MethodGet, "/groups/"+gid, ownerTok, nil, http.StatusOK)
	invite, _ := g["invite_token"].(string)
	if invite == "" {
		e.t.Fatalf("GET /groups sin invite_token para admin: %v", g)
	}
	e.mustStatus(http.MethodPost, "/groups/"+gid+"/join", memberTok, map[string]any{"invite_token": invite}, http.StatusCreated)
	e.t.Cleanup(func() {
		_, _ = e.pool.Exec(context.Background(), `DELETE FROM social.groups WHERE id = $1`, gid)
	})
	return gid, ownerTok, memberTok
}

func e2eDataID(t *testing.T, m map[string]any) string {
	t.Helper()
	d, _ := m["data"].(map[string]any)
	id, _ := d["id"].(string)
	if id == "" {
		t.Fatalf("sin data.id: %v", m)
	}
	return id
}

func e2eDataLen(t *testing.T, m map[string]any) int {
	t.Helper()
	lst, _ := m["data"].([]any)
	return len(lst)
}

// Viaje Kanban completo por HTTP real: crear → listar → filtrar → parchear → borrar.
func TestMartinE2E_TodoJourney(t *testing.T) {
	e := newMartinE2EEnv(t)
	owner, member, stranger := uuid.NewString(), uuid.NewString(), uuid.NewString()
	gid, ownerTok, memberTok := e.setupGroup(owner, member)
	strangerTok := e.mintToken(stranger)

	m := e.mustStatus(http.MethodPost, "/groups/"+gid+"/todo", memberTok, map[string]any{"title": "E2E-T1"}, http.StatusCreated)
	tid := e2eDataID(t, m)

	m = e.mustStatus(http.MethodGet, "/groups/"+gid+"/todo", memberTok, nil, http.StatusOK)
	if n := e2eDataLen(t, m); n != 1 {
		t.Fatalf("GET debe traer 1, got %d", n)
	}

	e.mustStatus(http.MethodPatch, "/groups/"+gid+"/todo/"+tid, ownerTok, map[string]any{"status": "done"}, http.StatusOK)
	m = e.mustStatus(http.MethodGet, "/groups/"+gid+"/todo?status=done", memberTok, nil, http.StatusOK)
	if n := e2eDataLen(t, m); n != 1 {
		t.Fatalf("filtro done debe traer 1, got %d", n)
	}

	e.mustStatus(http.MethodDelete, "/groups/"+gid+"/todo/"+tid, ownerTok, nil, http.StatusOK)
	m = e.mustStatus(http.MethodGet, "/groups/"+gid+"/todo", memberTok, nil, http.StatusOK)
	if n := e2eDataLen(t, m); n != 0 {
		t.Fatalf("tras DELETE debe traer 0, got %d", n)
	}

	// Seguridad real del middleware: sin token, token malo y no-miembro.
	e.mustStatus(http.MethodGet, "/groups/"+gid+"/todo", "", nil, http.StatusUnauthorized)
	e.mustStatus(http.MethodGet, "/groups/"+gid+"/todo", "token-invalido", nil, http.StatusUnauthorized)
	e.mustStatus(http.MethodGet, "/groups/"+gid+"/todo", strangerTok, nil, http.StatusForbidden)
	e.mustStatus(http.MethodPost, "/groups/"+gid+"/todo", strangerTok, map[string]any{"title": "X"}, http.StatusForbidden)
}

// Viaje sprint-sheet completo por HTTP real.
func TestMartinE2E_SprintJourney(t *testing.T) {
	e := newMartinE2EEnv(t)
	owner, member := uuid.NewString(), uuid.NewString()
	gid, ownerTok, memberTok := e.setupGroup(owner, member)

	m := e.mustStatus(http.MethodPost, "/groups/"+gid+"/sprint-sheet", ownerTok,
		map[string]any{"title": "E2E-S1", "assigned_to": member, "priority": "alta"}, http.StatusCreated)
	tid := e2eDataID(t, m)

	m = e.mustStatus(http.MethodGet, "/groups/"+gid+"/sprint-sheet", memberTok, nil, http.StatusOK)
	if n := e2eDataLen(t, m); n != 1 {
		t.Fatalf("GET debe traer 1, got %d", n)
	}
	m = e.mustStatus(http.MethodGet, "/groups/"+gid+"/sprint-sheet?priority=alta", memberTok, nil, http.StatusOK)
	if n := e2eDataLen(t, m); n != 1 {
		t.Fatalf("filtro alta debe traer 1, got %d", n)
	}

	e.mustStatus(http.MethodPatch, "/groups/"+gid+"/sprint-sheet/"+tid, ownerTok,
		map[string]any{"status": "en_proceso"}, http.StatusOK)
	e.mustStatus(http.MethodDelete, "/groups/"+gid+"/sprint-sheet/"+tid, ownerTok, nil, http.StatusOK)
	m = e.mustStatus(http.MethodGet, "/groups/"+gid+"/sprint-sheet", memberTok, nil, http.StatusOK)
	if n := e2eDataLen(t, m); n != 0 {
		t.Fatalf("tras DELETE debe traer 0, got %d", n)
	}
}

// Agendar reunión por HTTP real + verificación en DB.
func TestMartinE2E_MeetingJourney(t *testing.T) {
	e := newMartinE2EEnv(t)
	owner, member := uuid.NewString(), uuid.NewString()
	gid, ownerTok, _ := e.setupGroup(owner, member)

	m := e.mustStatus(http.MethodPost, "/groups/"+gid+"/meetings", ownerTok, map[string]any{
		"title": "E2E-Daily", "scheduled_at": "2026-11-20T15:00:00Z", "attendees": []string{"e2e@mail.com"},
	}, http.StatusCreated)
	mid, _ := m["meeting_id"].(string)
	if mid == "" {
		t.Fatalf("sin meeting_id: %v", m)
	}

	var title, email string
	var notifs int
	ctx := context.Background()
	if err := e.pool.QueryRow(ctx, `SELECT title FROM social.meetings WHERE id = $1`, mid).Scan(&title); err != nil || title != "E2E-Daily" {
		t.Fatalf("reunión en DB: %q err=%v", title, err)
	}
	if err := e.pool.QueryRow(ctx, `SELECT email FROM social.meeting_attendees WHERE meeting_id = $1`, mid).Scan(&email); err != nil || email != "e2e@mail.com" {
		t.Fatalf("invitado en DB: %q err=%v", email, err)
	}
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM social.meeting_notifications WHERE meeting_id = $1`, mid).Scan(&notifs); err != nil || notifs != 1 {
		t.Fatalf("notificaciones: %d err=%v (se esperaba 1 al miembro)", notifs, err)
	}
}
