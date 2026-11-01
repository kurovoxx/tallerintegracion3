# PATCH /groups/:id/todo/:taskId

Actualización parcial de una tarea Kanban (microservicio `social`).

## Req
- Método: `PATCH`, ruta: `/groups/:id/todo/:taskId` (protegida con JWT `Bearer`; exige membresía; `back/social/cmd/server/main.go:199`).
- Path: `id` (grupo) + `taskId` (tarea), ambos uuid.
- Body parcial (`UpdateTodoRequest`, `internal/handler/http/todo_handler.go:168-173`): al menos un campo de `title, status, assigned_to, due_date` (si no → `400` nothing-to-patch). Ausente = no toca; `""` en `assigned_to`/`due_date` = desasigna/limpia. Ej: `{"status":"done","title":"Nuevo título"}`.

## Res
- `200`: `{"message":"todo updated successfully","data":{...misma forma que POST...}}`.
- `400`: `invalid_group_id|invalid_todo_id|invalid_body` (incluye nothing-to-update). `401`: `unauthorized`. `403`: `forbidden`. `404`: `group_not_found|todo_not_found` (tarea de otro grupo → `404` sin filtrar). `500`: `internal`.

## Flujo
- `TodoHandler.UpdateTodo` (`todo_handler.go:175-251`) → `TodoService.UpdateTodo` (`internal/service/todo_service.go:344-440`: exige ≥1 campo, `requireGroup` + `requireMembership`, `GetTaskByID` + `requireTaskInGroup`, validación por campo) → `UpdateTask` (`UPDATE social.todo_tasks`, `internal/repository/sqlc/todo.sql.go:198`) + re-lectura del board.

## Tests
- `internal/handler/http/martin_unit_test.go`: `TestMartin_TodoPatchDelete_Unit` (parcial, scoping por grupo, nothing-to-patch, 404).
- `internal/handler/http/martin_integration_test.go`: `TestMartinIntegration_TodoCRUD_PG` (persiste `status=done`).
- `internal/handler/http/martin_e2e_test.go`: `TestMartinE2E_TodoJourney` (PATCH + filtro `done`).
- `internal/handler/http/tasks_routes_test.go`: `TestTodoTaskIdRoutes_Contrato` (404 con otro grupo, 403 extraño).
