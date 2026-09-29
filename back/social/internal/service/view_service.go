package service

import (
	"context"
	"time"

	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
)

// Fuentes de datos del workspace. Las implementan GroupService, TodoService,
// SprintService y MeetingService; son interfaces para poder probar el payload
// sin base de datos.
type workspaceGroups interface {
	GetGroup(ctx context.Context, groupID, userID string) (*model.GroupView, error)
}

type workspaceTodos interface {
	ListTodos(ctx context.Context, groupID, userID, status, boardID string) ([]TodoTaskView, error)
}

type workspaceSprint interface {
	ListSheets(ctx context.Context, groupID string) ([]sqlc.SocialSprintSheet, error)
	ListSprintTasks(ctx context.Context, groupID, userID, status, priority, sheetID string) ([]SprintTaskView, error)
}

type workspaceMeetings interface {
	ListUpcomingMeetings(ctx context.Context, groupID, userID string, now time.Time) ([]sqlc.SocialMeeting, error)
}

// WorkspaceData reúne lo necesario para armar el payload de GET /groups/{id}/workspace.
// El handler lo convierte a JSON con los mismos mapeos de los endpoints individuales.
type WorkspaceData struct {
	Group    *model.GroupView
	Todos    []TodoTaskView
	Sheets   []sqlc.SocialSprintSheet
	Tasks    []SprintTaskView
	Meetings []sqlc.SocialMeeting
}

// ViewService compone datos de varios servicios para las vistas del frontend
// (tarea 2_3_15). Solo lee; no aplica reglas de negocio propias salvo exigir
// membresía, que los servicios de tareas y hojas de sprint no comprueban.
type ViewService struct {
	groups   workspaceGroups
	todos    workspaceTodos
	sprint   workspaceSprint
	meetings workspaceMeetings
	now      func() time.Time
}

func NewViewService(groups workspaceGroups, todos workspaceTodos, sprint workspaceSprint, meetings workspaceMeetings) *ViewService {
	return &ViewService{groups: groups, todos: todos, sprint: sprint, meetings: meetings, now: time.Now}
}

// Workspace devuelve los datos de la vista de trabajo del grupo. GetGroup exige
// que el usuario sea miembro (403) y que el grupo exista (404) antes de leer nada.
func (s *ViewService) Workspace(ctx context.Context, groupID, userID string) (*WorkspaceData, error) {
	group, err := s.groups.GetGroup(ctx, groupID, userID)
	if err != nil {
		return nil, err
	}
	todos, err := s.todos.ListTodos(ctx, groupID, userID, "", "")
	if err != nil {
		return nil, err
	}
	sheets, err := s.sprint.ListSheets(ctx, groupID)
	if err != nil {
		return nil, err
	}
	tasks, err := s.sprint.ListSprintTasks(ctx, groupID, userID, "", "", "")
	if err != nil {
		return nil, err
	}
	meetings, err := s.meetings.ListUpcomingMeetings(ctx, groupID, userID, s.now())
	if err != nil {
		return nil, err
	}
	return &WorkspaceData{Group: group, Todos: todos, Sheets: sheets, Tasks: tasks, Meetings: meetings}, nil
}
