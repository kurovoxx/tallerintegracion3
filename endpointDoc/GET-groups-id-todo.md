# GET /groups/:id/todo

Lista las tareas Kanban del grupo (microservicio `social`).

## Req
- Método: `GET`, ruta: `/groups/:id/todo` (protegida con JWT `Bearer`; exige membresía; `back/social/cmd/server/main.go:198`).
- Path: `id` (uuid del grupo). Query opcional: `?status=todo|in_progress|done&board_id=<uuid>`.

## Res
- `200`: `{"data":[{"id":"uuid","group_id":"uuid","board_id":"uuid","board_name":"...","title":"...","status":"...","assigned_to":null,"due_date":null,"created_at":"RFC3339","updated_at":"RFC3339"}]}` (vacío = `[]`).
- `400`: `invalid_group_id|invalid_status|invalid_board_id`. `401`: `unauthorized`. `403`: `forbidden`. `404`: `group_not_found|board_not_found`. `500`: `internal`.

## Flujo
- `TodoHandler.ListTodos` (`internal/handler/http/todo_handler.go:114-166`) → `TodoService.ListTodos` (`internal/service/todo_service.go:279-342`: valida filtros, `requireGroup` + `requireMembership`) → `ListTasksByGroup` (`JOIN social.todo_tasks + social.todo_boards`, `internal/repository/sqlc/todo.sql.go:141`).

## Tests
- `internal/handler/http/martin_unit_test.go`: `TestMartin_TodoCreateList_Unit` (filtros, lista vacía, 401/403/400/404).
- `internal/handler/http/martin_integration_test.go`: `TestMartinIntegration_TodoCRUD_PG` (lee 1 fila real).
- `internal/handler/http/martin_e2e_test.go`: `TestMartinE2E_TodoJourney` (GET tras crear/borrar + 401/403).
