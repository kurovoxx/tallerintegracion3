package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

// ViewHandler expone los payloads agregados de las vistas del frontend
// (tareas 2_3_14 y 2_3_15). Documentados en agentApiContract.md sección 6.
type ViewHandler struct {
	groups *service.GroupService
	view   *service.ViewService
}

func NewViewHandler(groups *service.GroupService, view *service.ViewService) *ViewHandler {
	return &ViewHandler{groups: groups, view: view}
}

// RegisterRoutes registra las rutas en un router ya protegido por autenticación.
func (h *ViewHandler) RegisterRoutes(r gin.IRoutes) {
	r.GET("/me/overview", h.MyOverview)
	r.GET("/groups/:id/workspace", h.Workspace)
}

// MyOverview maneja GET /me/overview: grupos del usuario y barra lateral.
func (h *ViewHandler) MyOverview(c *gin.Context) {
	userID, ok := requireUser(c)
	if !ok {
		return
	}
	overview, err := h.groups.MyOverview(c.Request.Context(), userID)
	if err != nil {
		respondGroupError(c, err, "")
		return
	}
	c.JSON(http.StatusOK, overview)
}

// Workspace maneja GET /groups/{id}/workspace: hoja de sprint, tablero Kanban,
// reuniones próximas y datos del chat. Solo para miembros del grupo.
func (h *ViewHandler) Workspace(c *gin.Context) {
	userID, ok := requireUser(c)
	if !ok {
		return
	}
	data, err := h.view.Workspace(c.Request.Context(), c.Param("id"), userID)
	if err != nil {
		respondGroupError(c, err, "Acceso denegado: debes ser miembro del grupo")
		return
	}
	c.JSON(http.StatusOK, buildWorkspace(data))
}

// buildWorkspace convierte los datos del servicio al payload, reutilizando los
// mapeos JSON de los endpoints individuales (todoToJSON, sprintTaskToJSON).
// Todas las listas salen no nil para serializar [] en vez de null.
func buildWorkspace(d *service.WorkspaceData) model.Workspace {
	ws := model.Workspace{
		Group: model.WorkspaceGroup{
			ID:          d.Group.ID,
			Name:        d.Group.Name,
			Description: d.Group.Description,
			Role:        d.Group.Role,
		},
		Kanban: model.WorkspaceKanban{
			Todo:       []map[string]any{},
			InProgress: []map[string]any{},
			Done:       []map[string]any{},
		},
		SprintSheet: model.WorkspaceSprintSheet{
			Sheets: []model.SprintSheetInfo{},
			Tasks:  []map[string]any{},
		},
		Meetings: model.WorkspaceMeetings{Upcoming: []model.MeetingItem{}},
		Chat: model.WorkspaceChat{
			Provider:      "stream",
			TokenEndpoint: "/groups/" + d.Group.ID + "/stream-token",
		},
	}

	for _, t := range d.Todos {
		item := todoToJSON(t)
		switch t.Task.Status {
		case "in_progress":
			ws.Kanban.InProgress = append(ws.Kanban.InProgress, item)
		case "done":
			ws.Kanban.Done = append(ws.Kanban.Done, item)
		default:
			ws.Kanban.Todo = append(ws.Kanban.Todo, item)
		}
	}

	for _, s := range d.Sheets {
		ws.SprintSheet.Sheets = append(ws.SprintSheet.Sheets, model.SprintSheetInfo{
			ID:          uuidToString(s.ID),
			Name:        s.Name,
			PeriodStart: dateToNil(s.PeriodStart),
			PeriodEnd:   dateToNil(s.PeriodEnd),
		})
	}
	for _, t := range d.Tasks {
		ws.SprintSheet.Tasks = append(ws.SprintSheet.Tasks, sprintTaskToJSON(t))
	}

	for _, m := range d.Meetings {
		item := model.MeetingItem{
			ID:          uuidToString(m.ID),
			Title:       m.Title,
			ScheduledAt: m.ScheduledAt.Time.UTC(),
		}
		if m.Description.Valid {
			desc := m.Description.String
			item.Description = &desc
		}
		ws.Meetings.Upcoming = append(ws.Meetings.Upcoming, item)
	}
	return ws
}
