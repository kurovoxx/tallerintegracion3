package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

// Fuentes simuladas del workspace: registran si se las llamó, para comprobar
// que un no-miembro nunca provoca lecturas.
type stubSources struct {
	todos    []service.TodoTaskView
	sheets   []sqlc.SocialSprintSheet
	tasks    []service.SprintTaskView
	meetings []sqlc.SocialMeeting
	err      error
	calls    int
}

func (s *stubSources) ListTodos(context.Context, string, string, string, string) ([]service.TodoTaskView, error) {
	s.calls++
	return s.todos, s.err
}
func (s *stubSources) ListSheets(context.Context, string) ([]sqlc.SocialSprintSheet, error) {
	s.calls++
	return s.sheets, s.err
}
func (s *stubSources) ListSprintTasks(context.Context, string, string, string, string, string) ([]service.SprintTaskView, error) {
	s.calls++
	return s.tasks, s.err
}
func (s *stubSources) ListUpcomingMeetings(context.Context, string, string, time.Time) ([]sqlc.SocialMeeting, error) {
	s.calls++
	return s.meetings, s.err
}

type viewEnv struct {
	*env
	src *stubSources
}

func newViewEnv() *viewEnv {
	gin.SetMode(gin.TestMode)
	store := service.NewMemoryGroupStore()
	groupSvc := service.NewGroupService(store, nil)
	src := &stubSources{}
	viewSvc := service.NewViewService(groupSvc, src, src, src)

	r := gin.New()
	protected := r.Group("")
	protected.Use(middleware.NewAuthMiddleware(stubValidator{}).RequireAuth())
	NewGroupHandler(groupSvc).RegisterRoutes(protected)
	NewViewHandler(groupSvc, viewSvc).RegisterRoutes(protected)
	return &viewEnv{env: &env{router: r, store: store}, src: src}
}

func pgUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		t.Fatalf("uuid %q: %v", s, err)
	}
	return u
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func TestOverviewEmptyUser(t *testing.T) {
	e := newViewEnv()
	w := e.do(http.MethodGet, "/me/overview", hStrang, nil)
	expect(t, w, http.StatusOK)

	got := decode(t, w)
	if got["user_id"] != hStrang {
		t.Fatalf("user_id = %v", got["user_id"])
	}
	// slices vacíos, nunca null
	raw := w.Body.String()
	for _, want := range []string{`"groups":[]`, `"sidebar":{"groups":[]}`, `"groups_count":0`, `"admin_groups_count":0`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("falta %s en %s", want, raw)
		}
	}
}

func TestOverviewPayload(t *testing.T) {
	e := newViewEnv()
	mine := e.createGroup(t, hMember)  // admin
	joined := e.createGroup(t, hAdmin) // member
	_ = e.createGroup(t, hOther)       // ajeno
	e.join(t, joined, hMember)
	e.join(t, joined, hOther)

	w := e.do(http.MethodGet, "/me/overview", hMember, nil)
	expect(t, w, http.StatusOK)

	var ov struct {
		UserID  string `json:"user_id"`
		Sidebar struct {
			Groups []map[string]any `json:"groups"`
		} `json:"sidebar"`
		Groups []map[string]any `json:"groups"`
		Stats  map[string]int   `json:"stats"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ov); err != nil {
		t.Fatal(err)
	}
	if len(ov.Groups) != 2 || len(ov.Sidebar.Groups) != 2 {
		t.Fatalf("debe listar solo mis 2 grupos: %s", w.Body)
	}
	// el más reciente primero
	if ov.Groups[0]["group_id"] != joined || ov.Groups[1]["group_id"] != mine {
		t.Fatalf("orden inesperado: %s", w.Body)
	}
	if ov.Groups[0]["role"] != "member" || ov.Groups[1]["role"] != "admin" {
		t.Fatalf("roles inesperados: %s", w.Body)
	}
	if ov.Groups[0]["member_count"].(float64) != 3 || ov.Groups[1]["member_count"].(float64) != 1 {
		t.Fatalf("member_count inesperado: %s", w.Body)
	}
	for _, k := range []string{"group_id", "name", "role", "member_count", "joined_at"} {
		if ov.Groups[0][k] == nil {
			t.Fatalf("falta %s en la tarjeta: %s", k, w.Body)
		}
	}
	for _, k := range []string{"group_id", "name", "role"} {
		if ov.Sidebar.Groups[0][k] == nil {
			t.Fatalf("falta %s en la barra lateral: %s", k, w.Body)
		}
	}
	if ov.Stats["groups_count"] != 2 || ov.Stats["admin_groups_count"] != 1 {
		t.Fatalf("stats inesperadas: %v", ov.Stats)
	}
}

func TestWorkspaceAccessRules(t *testing.T) {
	e := newViewEnv()
	gid := e.createGroup(t, hAdmin)

	expect(t, e.do(http.MethodGet, "/groups/"+gid+"/workspace", "", nil), http.StatusUnauthorized)
	expect(t, e.do(http.MethodGet, "/groups/"+gid+"/workspace", hStrang, nil), http.StatusForbidden)
	expect(t, e.do(http.MethodGet, "/groups/dddddddd-0000-4000-8000-000000000001/workspace", hAdmin, nil), http.StatusNotFound)
	expect(t, e.do(http.MethodGet, "/groups/basura/workspace", hAdmin, nil), http.StatusNotFound)
	if e.src.calls != 0 {
		t.Fatalf("un no-miembro no debe provocar lecturas de tareas/reuniones: %d llamadas", e.src.calls)
	}

	// un baneado pierde el acceso
	e.join(t, gid, hMember)
	expect(t, e.do(http.MethodGet, "/groups/"+gid+"/workspace", hMember, nil), http.StatusOK)
	expect(t, e.do(http.MethodPost, "/groups/"+gid+"/members/"+hMember+"/ban", hAdmin, nil), http.StatusNoContent)
	expect(t, e.do(http.MethodGet, "/groups/"+gid+"/workspace", hMember, nil), http.StatusForbidden)
}

func TestWorkspaceEmptyGroupHasEmptyArrays(t *testing.T) {
	e := newViewEnv()
	gid := e.createGroup(t, hAdmin)
	w := e.do(http.MethodGet, "/groups/"+gid+"/workspace", hAdmin, nil)
	expect(t, w, http.StatusOK)

	raw := w.Body.String()
	for _, want := range []string{`"todo":[]`, `"in_progress":[]`, `"done":[]`, `"sheets":[]`, `"tasks":[]`, `"upcoming":[]`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("falta %s en %s", want, raw)
		}
	}
	if strings.Contains(raw, "null") {
		t.Fatalf("no debe haber null en un grupo vacío: %s", raw)
	}
	got := decode(t, w)
	group := got["group"].(map[string]any)
	if group["id"] != gid || group["role"] != "admin" {
		t.Fatalf("group inesperado: %v", group)
	}
	chat := got["chat"].(map[string]any)
	if chat["provider"] != "stream" || chat["token_endpoint"] != "/groups/"+gid+"/stream-token" {
		t.Fatalf("chat inesperado: %v", chat)
	}
	if strings.Contains(raw, "invite_token") {
		t.Fatalf("el workspace no debe exponer el invite_token: %s", raw)
	}
}

func TestWorkspacePayload(t *testing.T) {
	e := newViewEnv()
	gid := e.createGroup(t, hAdmin)
	e.join(t, gid, hMember)

	gID := pgUUID(t, gid)
	board := sqlc.SocialTodoBoard{ID: pgUUID(t, "11111111-0000-4000-8000-000000000001"), GroupID: gID, Name: "General"}
	mkTodo := func(id, title, status string) service.TodoTaskView {
		return service.TodoTaskView{
			Task:  sqlc.SocialTodoTask{ID: pgUUID(t, id), BoardID: board.ID, Title: title, Status: status},
			Board: board,
		}
	}
	e.src.todos = []service.TodoTaskView{
		mkTodo("22222222-0000-4000-8000-000000000001", "por hacer", "todo"),
		mkTodo("22222222-0000-4000-8000-000000000002", "en curso", "in_progress"),
		mkTodo("22222222-0000-4000-8000-000000000003", "lista", "done"),
		mkTodo("22222222-0000-4000-8000-000000000004", "otra por hacer", "todo"),
	}

	sheetID := pgUUID(t, "33333333-0000-4000-8000-000000000001")
	e.src.sheets = []sqlc.SocialSprintSheet{{ID: sheetID, GroupID: gID, Name: "Sprint 1"}}
	e.src.tasks = []service.SprintTaskView{{
		Task: sqlc.SocialSprintSheetTask{
			ID: pgUUID(t, "44444444-0000-4000-8000-000000000001"), SheetID: sheetID,
			AssigneeUserID: pgUUID(t, hMember), Title: "tarea sprint", Priority: "alta", Status: "en_proceso",
		},
		Sheet: e.src.sheets[0],
	}}

	when := time.Date(2026, 9, 30, 15, 0, 0, 0, time.UTC)
	e.src.meetings = []sqlc.SocialMeeting{{
		ID: pgUUID(t, "55555555-0000-4000-8000-000000000001"), GroupID: gID, Title: "Daily",
		Description: pgtype.Text{String: "revisión", Valid: true}, ScheduledAt: ts(when),
	}}

	w := e.do(http.MethodGet, "/groups/"+gid+"/workspace", hMember, nil)
	expect(t, w, http.StatusOK)

	var ws struct {
		Group  map[string]any `json:"group"`
		Kanban map[string][]struct {
			Title  string `json:"title"`
			Status string `json:"status"`
		} `json:"kanban"`
		SprintSheet struct {
			Sheets []map[string]any `json:"sheets"`
			Tasks  []map[string]any `json:"tasks"`
		} `json:"sprint_sheet"`
		Meetings struct {
			Upcoming []map[string]any `json:"upcoming"`
		} `json:"meetings"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ws); err != nil {
		t.Fatal(err)
	}

	if ws.Group["role"] != "member" {
		t.Fatalf("role = %v, debe ser el rol de quien consulta", ws.Group["role"])
	}
	if len(ws.Kanban["todo"]) != 2 || len(ws.Kanban["in_progress"]) != 1 || len(ws.Kanban["done"]) != 1 {
		t.Fatalf("columnas Kanban inesperadas: %s", w.Body)
	}
	if ws.Kanban["in_progress"][0].Title != "en curso" || ws.Kanban["done"][0].Status != "done" {
		t.Fatalf("tarjetas mal ubicadas: %s", w.Body)
	}
	if len(ws.SprintSheet.Sheets) != 1 || ws.SprintSheet.Sheets[0]["name"] != "Sprint 1" || ws.SprintSheet.Sheets[0]["id"] != "33333333-0000-4000-8000-000000000001" {
		t.Fatalf("hojas inesperadas: %s", w.Body)
	}
	if len(ws.SprintSheet.Tasks) != 1 || ws.SprintSheet.Tasks[0]["sheet_id"] != "33333333-0000-4000-8000-000000000001" ||
		ws.SprintSheet.Tasks[0]["priority"] != "alta" || ws.SprintSheet.Tasks[0]["assigned_to"] != hMember {
		t.Fatalf("tareas de sprint inesperadas: %s", w.Body)
	}
	if len(ws.Meetings.Upcoming) != 1 || ws.Meetings.Upcoming[0]["title"] != "Daily" ||
		ws.Meetings.Upcoming[0]["description"] != "revisión" || ws.Meetings.Upcoming[0]["scheduled_at"] != "2026-09-30T15:00:00Z" {
		t.Fatalf("reuniones inesperadas: %s", w.Body)
	}
}

// Un error de una fuente se responde 500 sin filtrar el detalle interno.
func TestWorkspaceSourceErrorIs500(t *testing.T) {
	e := newViewEnv()
	gid := e.createGroup(t, hAdmin)
	e.src.err = errors.New("db caída: password=secreto")

	w := e.do(http.MethodGet, "/groups/"+gid+"/workspace", hAdmin, nil)
	expect(t, w, http.StatusInternalServerError)
	if strings.Contains(w.Body.String(), "secreto") {
		t.Fatalf("no debe filtrar el error interno: %s", w.Body)
	}
}

func TestViewRoutesRequireAuth(t *testing.T) {
	e := newViewEnv()
	expect(t, e.do(http.MethodGet, "/me/overview", "", nil), http.StatusUnauthorized)
	expect(t, e.do(http.MethodGet, "/me/overview", "malo", nil), http.StatusUnauthorized)
}
