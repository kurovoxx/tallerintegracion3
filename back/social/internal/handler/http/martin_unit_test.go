package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

// Env unitario Martín: los 9 endpoints con stores en memoria (sin DB ni red).
type martinEnv struct {
	router *gin.Engine
	tStore *service.MemoryTodoStore
	sStore *service.MemorySprintStore
	mStore *service.MemoryMeetingStore
}

func newMartinEnv() *martinEnv {
	gin.SetMode(gin.TestMode)
	tStore := service.NewMemoryTodoStore()
	sStore := service.NewMemorySprintStore()
	mStore := service.NewMemoryMeetingStore()
	todoH := NewTodoHandler(service.NewTodoService(tStore))
	sprintH := NewSprintHandler(service.NewSprintService(sStore))
	meetingH := NewMeetingHandler(service.NewMeetingService(mStore, service.NoopMeetingCreatedNotifier{}))

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
	return &martinEnv{router: r, tStore: tStore, sStore: sStore, mStore: mStore}
}

func (e *martinEnv) doMartin(method, path, user string, body any) *httptest.ResponseRecorder {
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
	e.router.ServeHTTP(w, req)
	return w
}

func martinIDs() (gid, admin, member, stranger string) {
	return uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
}

func martinDataID(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	m := decode(t, w)
	d, ok := m["data"].(map[string]any)
	if !ok {
		t.Fatalf("sin data en %s", w.Body.String())
	}
	id, _ := d["id"].(string)
	if id == "" {
		t.Fatalf("sin id en %s", w.Body.String())
	}
	return id
}

func martinDataListLen(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	m := decode(t, w)
	lst, ok := m["data"].([]any)
	if !ok {
		t.Fatalf("data no es lista en %s", w.Body.String())
	}
	return len(lst)
}

// 1-2. POST + GET /todo (Kanban): crea, filtra por status y exige membresía.
func TestMartin_TodoCreateList_Unit(t *testing.T) {
	e := newMartinEnv()
	gid, _, member, stranger := martinIDs()
	e.tStore.AddGroup(gid, uuid.NewString(), member)

	w := e.doMartin(http.MethodPost, "/groups/"+gid+"/todo", member, map[string]any{"title": "T1", "status": "in_progress"})
	expect(t, w, http.StatusCreated)
	martinDataID(t, w)

	w = e.doMartin(http.MethodGet, "/groups/"+gid+"/todo", member, nil)
	expect(t, w, http.StatusOK)
	if n := martinDataListLen(t, w); n != 1 {
		t.Fatalf("se esperaba 1 tarea, got %d: %s", n, w.Body.String())
	}

	// Filtro por status que no coincide => lista vacía, no 404.
	w = e.doMartin(http.MethodGet, "/groups/"+gid+"/todo?status=done", member, nil)
	expect(t, w, http.StatusOK)
	if n := martinDataListLen(t, w); n != 0 {
		t.Fatalf("filtro done debía dar []: %s", w.Body.String())
	}

	// 401 sin token, 403 extraño, 400 status inválido y título vacío.
	expect(t, e.doMartin(http.MethodGet, "/groups/"+gid+"/todo", "", nil), http.StatusUnauthorized)
	expect(t, e.doMartin(http.MethodGet, "/groups/"+gid+"/todo", stranger, nil), http.StatusForbidden)
	expect(t, e.doMartin(http.MethodGet, "/groups/"+gid+"/todo?status=mal", member, nil), http.StatusBadRequest)
	expect(t, e.doMartin(http.MethodPost, "/groups/"+gid+"/todo", member, map[string]any{"title": "  "}), http.StatusBadRequest)
	expect(t, e.doMartin(http.MethodPost, "/groups/"+gid+"/todo", member, map[string]any{"title": "T", "status": "mal"}), http.StatusBadRequest)
	expect(t, e.doMartin(http.MethodGet, "/groups/"+uuid.NewString()+"/todo", member, nil), http.StatusNotFound)
}

// 3-4. PATCH + DELETE /todo/:taskId: parcial, scoping por grupo y nothing-to-patch.
func TestMartin_TodoPatchDelete_Unit(t *testing.T) {
	e := newMartinEnv()
	gid, admin, member, _ := martinIDs()
	e.tStore.AddGroup(gid, admin, member)
	other := uuid.NewString()
	e.tStore.AddGroup(other, admin)

	w := e.doMartin(http.MethodPost, "/groups/"+gid+"/todo", member, map[string]any{"title": "T1"})
	tid := martinDataID(t, w)

	w = e.doMartin(http.MethodPatch, "/groups/"+gid+"/todo/"+tid, member, map[string]any{"status": "done"})
	expect(t, w, http.StatusOK)

	// Sin campos => 400 nothing-to-patch; otro grupo => 404 (sin filtrar).
	expect(t, e.doMartin(http.MethodPatch, "/groups/"+gid+"/todo/"+tid, member, map[string]any{}), http.StatusBadRequest)
	expect(t, e.doMartin(http.MethodPatch, "/groups/"+other+"/todo/"+tid, admin, map[string]any{"title": "X"}), http.StatusNotFound)
	// taskId inexistente => 404.
	expect(t, e.doMartin(http.MethodPatch, "/groups/"+gid+"/todo/"+uuid.NewString(), member, map[string]any{"title": "X"}), http.StatusNotFound)

	w = e.doMartin(http.MethodDelete, "/groups/"+gid+"/todo/"+tid, admin, nil)
	expect(t, w, http.StatusOK)
	// Doble delete => 404.
	expect(t, e.doMartin(http.MethodDelete, "/groups/"+gid+"/todo/"+tid, admin, nil), http.StatusNotFound)
}

// 5-6. POST + GET /sprint-sheet: assigned_to obligatorio, defaults y filtros.
func TestMartin_SprintCreateList_Unit(t *testing.T) {
	e := newMartinEnv()
	gid, admin, member, stranger := martinIDs()
	e.sStore.AddGroup(gid, admin, member)

	// Sin assigned_to => 400 (diferencia clave vs todo).
	expect(t, e.doMartin(http.MethodPost, "/groups/"+gid+"/sprint-sheet", admin, map[string]any{"title": "S"}), http.StatusBadRequest)

	w := e.doMartin(http.MethodPost, "/groups/"+gid+"/sprint-sheet", admin, map[string]any{"title": "S1", "assigned_to": member})
	expect(t, w, http.StatusCreated)
	m := decode(t, w)["data"].(map[string]any)
	if m["priority"] != "media" || m["status"] != "sin_empezar" {
		t.Fatalf("defaults prioridad/estado: %s", w.Body.String())
	}

	w = e.doMartin(http.MethodGet, "/groups/"+gid+"/sprint-sheet", member, nil)
	expect(t, w, http.StatusOK)
	if n := martinDataListLen(t, w); n != 1 {
		t.Fatalf("se esperaba 1 sprint task, got %d: %s", n, w.Body.String())
	}

	// Filtros: prioridad y estado.
	w = e.doMartin(http.MethodGet, "/groups/"+gid+"/sprint-sheet?priority=alta", member, nil)
	expect(t, w, http.StatusOK)
	if n := martinDataListLen(t, w); n != 0 {
		t.Fatalf("filtro alta debía dar []: %s", w.Body.String())
	}
	expect(t, e.doMartin(http.MethodGet, "/groups/"+gid+"/sprint-sheet?status=mal", member, nil), http.StatusBadRequest)
	expect(t, e.doMartin(http.MethodGet, "/groups/"+gid+"/sprint-sheet?priority=mal", member, nil), http.StatusBadRequest)
	expect(t, e.doMartin(http.MethodGet, "/groups/"+gid+"/sprint-sheet", stranger, nil), http.StatusForbidden)
	expect(t, e.doMartin(http.MethodPost, "/groups/"+gid+"/sprint-sheet", admin, map[string]any{"title": "S2", "assigned_to": stranger}), http.StatusBadRequest)
	expect(t, e.doMartin(http.MethodPost, "/groups/"+gid+"/sprint-sheet", admin, map[string]any{"title": "S3", "assigned_to": member, "priority": "mal"}), http.StatusBadRequest)
	expect(t, e.doMartin(http.MethodPost, "/groups/"+gid+"/sprint-sheet", admin, map[string]any{"title": "S4", "assigned_to": member, "estimated_hours": -1}), http.StatusBadRequest)
}

// 7-8. PATCH + DELETE /sprint-sheet/:taskId.
func TestMartin_SprintPatchDelete_Unit(t *testing.T) {
	e := newMartinEnv()
	gid, admin, member, _ := martinIDs()
	e.sStore.AddGroup(gid, admin, member)
	other := uuid.NewString()
	e.sStore.AddGroup(other, admin)

	w := e.doMartin(http.MethodPost, "/groups/"+gid+"/sprint-sheet", admin, map[string]any{"title": "S1", "assigned_to": member})
	tid := martinDataID(t, w)

	w = e.doMartin(http.MethodPatch, "/groups/"+gid+"/sprint-sheet/"+tid, admin, map[string]any{"priority": "alta", "status": "en_proceso"})
	expect(t, w, http.StatusOK)

	expect(t, e.doMartin(http.MethodPatch, "/groups/"+gid+"/sprint-sheet/"+tid, admin, map[string]any{}), http.StatusBadRequest)
	expect(t, e.doMartin(http.MethodPatch, "/groups/"+other+"/sprint-sheet/"+tid, admin, map[string]any{"title": "X"}), http.StatusNotFound)
	expect(t, e.doMartin(http.MethodPatch, "/groups/"+gid+"/sprint-sheet/"+uuid.NewString(), admin, map[string]any{"title": "X"}), http.StatusNotFound)

	w = e.doMartin(http.MethodDelete, "/groups/"+gid+"/sprint-sheet/"+tid, admin, nil)
	expect(t, w, http.StatusOK)
	expect(t, e.doMartin(http.MethodDelete, "/groups/"+gid+"/sprint-sheet/"+tid, admin, nil), http.StatusNotFound)
}

// 9. POST /meetings: crea, valida fecha/invitados y exige membresía.
func TestMartin_MeetingCreate_Unit(t *testing.T) {
	e := newMartinEnv()
	gid, admin, member, stranger := martinIDs()
	e.mStore.AddGroup(gid, admin, member)

	w := e.doMartin(http.MethodPost, "/groups/"+gid+"/meetings", admin, map[string]any{
		"title": "Daily", "scheduled_at": "2026-10-20T15:00:00Z", "attendees": []string{"a@mail.com"},
	})
	expect(t, w, http.StatusCreated)
	if mid, _ := decode(t, w)["meeting_id"].(string); mid == "" {
		t.Fatalf("sin meeting_id: %s", w.Body.String())
	}
	_ = member

	expect(t, e.doMartin(http.MethodPost, "/groups/"+gid+"/meetings", stranger, map[string]any{"title": "X", "scheduled_at": "2026-10-20T15:00:00Z"}), http.StatusForbidden)
	expect(t, e.doMartin(http.MethodPost, "/groups/"+gid+"/meetings", admin, map[string]any{"title": "X", "scheduled_at": "ayer"}), http.StatusBadRequest)
	expect(t, e.doMartin(http.MethodPost, "/groups/"+gid+"/meetings", admin, map[string]any{"title": "  ", "scheduled_at": "2026-10-20T15:00:00Z"}), http.StatusBadRequest)
	expect(t, e.doMartin(http.MethodPost, "/groups/"+gid+"/meetings", admin, map[string]any{"title": "X"}), http.StatusBadRequest)
	expect(t, e.doMartin(http.MethodPost, "/groups/"+gid+"/meetings", admin, map[string]any{"title": "X", "scheduled_at": "2026-10-20T15:00:00Z", "attendees": []string{"no-email"}}), http.StatusBadRequest)
	expect(t, e.doMartin(http.MethodPost, "/groups/"+gid+"/meetings", "", map[string]any{"title": "X", "scheduled_at": "2026-10-20T15:00:00Z"}), http.StatusUnauthorized)
	expect(t, e.doMartin(http.MethodPost, "/groups/"+uuid.NewString()+"/meetings", admin, map[string]any{"title": "X", "scheduled_at": "2026-10-20T15:00:00Z"}), http.StatusNotFound)
}
