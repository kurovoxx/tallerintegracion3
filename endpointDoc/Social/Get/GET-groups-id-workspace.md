# GET /groups/:id/workspace

Vista de trabajo del grupo en una sola llamada: kanban, hojas y tareas de sprint, reuniones próximas y datos del chat (microservicio `social`).

## Req
- Método: `GET`, ruta: `/groups/:id/workspace` (protegida con JWT `Bearer`; solo miembros; `back/social/internal/handler/http/view_handler.go:25`).
- Headers: `Authorization: Bearer <access_token>`.
- Path: `id` (uuid del grupo).

## Res
- `200` (`model.Workspace`):
  ```json
  {
    "group": {"id","name","description","role"},
    "kanban": {"todo": [], "in_progress": [], "done": []},
    "sprint_sheet": {"sheets": [{"id","name","period_start","period_end"}], "tasks": []},
    "meetings": {"upcoming": [{"id","title","description?","scheduled_at"}]},
    "chat": {"provider": "stream", "token_endpoint": "/groups/<id>/stream-token"}
  }
  ```
  Ítems del kanban y tareas usan el mismo JSON que `GET /groups/:id/todo` y `GET /groups/:id/sprint-sheet`. Las listas nunca son `null`.
- `403` `{"error":"Acceso denegado: debes ser miembro del grupo"}`. `404` grupo inexistente o id inválido. `401`: sin token. `500`: falla de cualquier fuente.

## Flujo
- `ViewHandler.Workspace` (`internal/handler/http/view_handler.go:44`) → `ViewService.Workspace` (`internal/service/view_service.go:58`): `GroupService.GetGroup` (exige membresía antes de leer nada) → `TodoService.ListTodos` → `SprintService.ListSheets` + `ListSprintTasks` → `MeetingService.ListUpcomingMeetings`. `buildWorkspace` reparte las tareas por `status`.
- No devuelve el token de Stream; el cliente lo pide aparte en `token_endpoint`.

## Tests
- `internal/handler/http/view_handler_test.go`: `TestWorkspaceAccessRules`, `TestWorkspaceEmptyGroupHasEmptyArrays`, `TestWorkspacePayload`, `TestWorkspaceSourceErrorIs500`.
- `test/social_routes_e2e_test.go`: subtest `todo_sprint_hours_and_workspace`.
