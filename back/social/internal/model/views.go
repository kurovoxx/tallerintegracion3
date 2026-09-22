package model

import "time"

// Payloads de las vistas del frontend (tareas 2_3_14 y 2_3_15).
// Social solo expone datos de grupos: el perfil vive en Identity (/profile/me)
// y los apuntes en Notes (/notes/me, /groups/{id}/notes), que el cliente
// consulta directamente.

// GroupCard es una tarjeta de grupo para la vista de grupos.
type GroupCard struct {
	GroupID     string    `json:"group_id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	Role        string    `json:"role"`
	MemberCount int       `json:"member_count"`
	JoinedAt    time.Time `json:"joined_at"`
}

// SidebarGroup es la entrada mínima de un grupo en la barra lateral global.
type SidebarGroup struct {
	GroupID string `json:"group_id"`
	Name    string `json:"name"`
	Role    string `json:"role"`
}

type OverviewSidebar struct {
	Groups []SidebarGroup `json:"groups"`
}

type OverviewStats struct {
	GroupsCount      int `json:"groups_count"`
	AdminGroupsCount int `json:"admin_groups_count"`
}

// MyOverview es el payload de GET /me/overview (vista principal global).
type MyOverview struct {
	UserID  string          `json:"user_id"`
	Sidebar OverviewSidebar `json:"sidebar"`
	Groups  []GroupCard     `json:"groups"`
	Stats   OverviewStats   `json:"stats"`
}

// WorkspaceGroup identifica el grupo y el rol de quien consulta.
type WorkspaceGroup struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Role        string  `json:"role"`
}

// WorkspaceKanban agrupa las tareas del tablero por estado.
// Los elementos son los mismos objetos que devuelve GET /groups/{id}/todo.
type WorkspaceKanban struct {
	Todo       []map[string]any `json:"todo"`
	InProgress []map[string]any `json:"in_progress"`
	Done       []map[string]any `json:"done"`
}

// WorkspaceSprintSheet expone las hojas de sprint del grupo y todas sus tareas
// (cada tarea trae su sheet_id). Las horas diarias no se incluyen (N+1): se
// piden por tarea bajo demanda en /sprint-sheet/{taskId}/hours.
type WorkspaceSprintSheet struct {
	Sheets []SprintSheetInfo `json:"sheets"`
	Tasks  []map[string]any  `json:"tasks"`
}

type SprintSheetInfo struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	PeriodStart *string `json:"period_start"`
	PeriodEnd   *string `json:"period_end"`
}

type MeetingItem struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description *string   `json:"description,omitempty"`
	ScheduledAt time.Time `json:"scheduled_at"`
}

type WorkspaceMeetings struct {
	Upcoming []MeetingItem `json:"upcoming"`
}

// WorkspaceChat apunta al endpoint del contrato que emite el token de Stream.
type WorkspaceChat struct {
	Provider      string `json:"provider"`
	TokenEndpoint string `json:"token_endpoint"`
}

// Workspace es el payload de GET /groups/{id}/workspace (hoja de sprint,
// tablero Kanban, reuniones y chat grupal).
type Workspace struct {
	Group       WorkspaceGroup       `json:"group"`
	Kanban      WorkspaceKanban      `json:"kanban"`
	SprintSheet WorkspaceSprintSheet `json:"sprint_sheet"`
	Meetings    WorkspaceMeetings    `json:"meetings"`
	Chat        WorkspaceChat        `json:"chat"`
}
