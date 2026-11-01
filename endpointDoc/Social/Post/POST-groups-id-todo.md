# POST /groups/:id/todo

Crea una tarea del tablero Kanban (microservicio `social`).

## Req
- Método: `POST`, ruta: `/groups/:id/todo` (protegida con JWT `Bearer`; exige membresía del grupo; `back/social/cmd/server/main.go:197`).
- Headers: `Content-Type: application/json`, `Authorization: Bearer <access_token>`.
- Path: `id` (uuid del grupo, requerido).
- Body (`CreateTodoRequest`, `internal/handler/http/todo_handler.go:26-32`): `{"title":"Diseñar login","board_id":"uuid-opcional","status":"todo","assigned_to":"uuid-opcional","due_date":"2026-10-20"}`.
- `title` requerido, max 300. `status` opcional: `todo|in_progress|done` (vacío = `todo`). `board_id`/`assigned_to` opcionales con formato uuid. `due_date` opcional `YYYY-MM-DD`.

## Res
- `201`: `{"message":"todo created successfully","data":{"id":"uuid","group_id":"uuid","board_id":"uuid","board_name":"General","title":"...","status":"todo","assigned_to":null,"due_date":"2026-10-20","created_at":"RFC3339","updated_at":"RFC3339"}}` (forma en `todoToJSON`, `todo_handler.go:307-320`).
- `400`: `invalid_group_id|invalid_board_id|invalid_user_id|invalid_body` (título vacío/largo, status inválido, fecha mala).
- `401`: `unauthorized` (sin token o token inválido, desde el middleware).
- `403`: `forbidden` (no-miembro; el asignado también debe ser miembro, regla 4.4).
- `404`: `group_not_found|board_not_found`. `500`: `internal`.

## Flujo
- `TodoHandler.CreateTodo` (`todo_handler.go:34-112`) valida `id` uuid + JSON + formatos → `TodoService.CreateTodo` (`internal/service/todo_service.go:222-277`: `requireGroup` + `requireMembership` + `requireAssigneeMember`, `resolveBoard` usa el tablero dado o el primero o crea `"General"`) → `TodoRepository.CreateTask` (`INSERT INTO social.todo_tasks`, `internal/repository/sqlc/todo.sql.go:102`).

## Tests
- `internal/handler/http/martin_unit_test.go`: `TestMartin_TodoCreateList_Unit` (201 + 401/403/400/404).
- `internal/handler/http/martin_integration_test.go`: `TestMartinIntegration_TodoCRUD_PG` (fila + board auto-creado en Postgres).
- `internal/handler/http/martin_e2e_test.go`: `TestMartinE2E_TodoJourney` (viaje HTTP real con JWT reales).
- `internal/handler/http/tasks_routes_test.go`: `TestTodoTaskIdRoutes_Contrato` (403 de extraño).
