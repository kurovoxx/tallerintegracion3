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

// Env con servicios reales sobre fakes: ejercita rutas con :taskId,
// scoping por grupo y 403 de no-miembro de punta a punta (sin DB).
type tasksEnv struct {
	router *gin.Engine
	todos  *service.TodoService
	sprint *service.SprintService
	hours  *service.HoursService
	tStore *service.MemoryTodoStore
	sStore *service.MemorySprintStore
}

func newTasksEnv() *tasksEnv {
	gin.SetMode(gin.TestMode)
	tStore := service.NewMemoryTodoStore()
	sStore := service.NewMemorySprintStore()
	todoH := NewTodoHandler(service.NewTodoService(tStore))
	sprintH := NewSprintHandler(service.NewSprintService(sStore))
	hoursH := NewHoursHandler(service.NewHoursService(sStore))

	r := gin.New()
	protected := r.Group("")
	protected.Use(middleware.NewAuthMiddleware(stubValidator{}).RequireAuth())
	protected.POST("/groups/:id/todo", todoH.CreateTodo)
	protected.PATCH("/groups/:id/todo/:taskId", todoH.UpdateTodo)
	protected.DELETE("/groups/:id/todo/:taskId", todoH.DeleteTodo)
	protected.POST("/groups/:id/sprint-sheet", sprintH.CreateSprintTask)
	protected.PATCH("/groups/:id/sprint-sheet/:taskId", sprintH.UpdateSprintTask)
	protected.DELETE("/groups/:id/sprint-sheet/:taskId", sprintH.DeleteSprintTask)
	protected.POST("/sprint-sheet/:taskId/hours", hoursH.LogHours)
	return &tasksEnv{
		router: r,
		todos:  service.NewTodoService(tStore),
		sprint: service.NewSprintService(sStore),
		hours:  service.NewHoursService(sStore),
		tStore: tStore,
		sStore: sStore,
	}
}

// do ejecuta una petición autenticada como user (token vacío = sin header).
func (e *tasksEnv) do(method, path, user string, body any) *httptest.ResponseRecorder {
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

func TestTodoTaskIdRoutes_Contrato(t *testing.T) {
	e := newTasksEnv()
	admin, member, stranger := uuid.NewString(), uuid.NewString(), uuid.NewString()
	gid := uuid.NewString()
	e.tStore.AddGroup(gid, admin, member)
	other := uuid.NewString()
	e.tStore.AddGroup(other, admin)
	body := map[string]any{"title": "T1"}

	// Crear como miembro.
	w := e.do(http.MethodPost, "/groups/"+gid+"/todo", member, body)
	expect(t, w, http.StatusCreated)
	tid := decode(t, w)["data"].(map[string]any)["id"].(string)

	// Extraño no puede ni crear.
	w = e.do(http.MethodPost, "/groups/"+gid+"/todo", stranger, body)
	expect(t, w, http.StatusForbidden)

	// PATCH con otro grupo => 404 (no filtra tareas ajenas).
	w = e.do(http.MethodPatch, "/groups/"+other+"/todo/"+tid, admin, map[string]any{"title": "X"})
	expect(t, w, http.StatusNotFound)

	// PATCH extraño => 403.
	w = e.do(http.MethodPatch, "/groups/"+gid+"/todo/"+tid, stranger, map[string]any{"title": "X"})
	expect(t, w, http.StatusForbidden)

	// PATCH ok + DELETE ok.
	w = e.do(http.MethodPatch, "/groups/"+gid+"/todo/"+tid, admin, map[string]any{"status": "done"})
	expect(t, w, http.StatusOK)
	w = e.do(http.MethodDelete, "/groups/"+gid+"/todo/"+tid, admin, nil)
	expect(t, w, http.StatusOK)

	// DELETE extraño sobre tarea existente => 403 (no 404: primero membresía).
	w = e.do(http.MethodPost, "/groups/"+gid+"/todo", member, body)
	expect(t, w, http.StatusCreated)
	tid2 := decode(t, w)["data"].(map[string]any)["id"].(string)
	w = e.do(http.MethodDelete, "/groups/"+gid+"/todo/"+tid2, stranger, nil)
	expect(t, w, http.StatusForbidden)
}

func TestSprintTaskIdRoutes_Contrato(t *testing.T) {
	e := newTasksEnv()
	admin, member, stranger := uuid.NewString(), uuid.NewString(), uuid.NewString()
	gid := uuid.NewString()
	e.sStore.AddGroup(gid, admin, member)
	other := uuid.NewString()
	e.sStore.AddGroup(other, admin)
	body := map[string]any{"title": "S1", "assigned_to": member}

	// Crear + horas como miembro.
	w := e.do(http.MethodPost, "/groups/"+gid+"/sprint-sheet", admin, body)
	expect(t, w, http.StatusCreated)
	tid := decode(t, w)["data"].(map[string]any)["id"].(string)
	w = e.do(http.MethodPost, "/sprint-sheet/"+tid+"/hours", member, map[string]any{"log_date": "2026-09-25", "hours": 2.5})
	expect(t, w, http.StatusCreated)

	// Horas de extraño => 403.
	w = e.do(http.MethodPost, "/sprint-sheet/"+tid+"/hours", stranger, map[string]any{"log_date": "2026-09-25", "hours": 1.0})
	expect(t, w, http.StatusForbidden)

	// Asignado extraño al crear => 400.
	w = e.do(http.MethodPost, "/groups/"+gid+"/sprint-sheet", admin, map[string]any{"title": "S2", "assigned_to": stranger})
	expect(t, w, http.StatusBadRequest)

	// PATCH con otro grupo => 404.
	w = e.do(http.MethodPatch, "/groups/"+other+"/sprint-sheet/"+tid, admin, map[string]any{"title": "X"})
	expect(t, w, http.StatusNotFound)

	// PATCH/DELETE extraño => 403; DELETE ok.
	w = e.do(http.MethodPatch, "/groups/"+gid+"/sprint-sheet/"+tid, stranger, map[string]any{"title": "X"})
	expect(t, w, http.StatusForbidden)
	w = e.do(http.MethodDelete, "/groups/"+gid+"/sprint-sheet/"+tid, admin, nil)
	expect(t, w, http.StatusOK)
}
