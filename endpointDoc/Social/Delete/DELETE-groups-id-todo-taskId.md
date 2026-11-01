# DELETE /groups/:id/todo/:taskId

Elimina una tarea Kanban (microservicio `social`).

## Req
- Método: `DELETE`, ruta: `/groups/:id/todo/:taskId` (protegida con JWT `Bearer`; exige membresía; `back/social/cmd/server/main.go:200`).
- Path: `id` + `taskId` (uuid). Sin body.

## Res
- `200`: `{"message":"todo deleted successfully","data":{...tarea eliminada...}}`.
- `400`: `invalid_group_id|invalid_todo_id`. `401`: `unauthorized`. `403`: `forbidden`. `404`: `group_not_found|todo_not_found` (doble delete → `404`). `500`: `internal`.

## Flujo
- `TodoHandler.DeleteTodo` (`internal/handler/http/todo_handler.go:253-305`) → `TodoService.DeleteTodo` (`internal/service/todo_service.go:442-491`: `requireGroup` + `requireMembership`, `GetTaskByID` + `requireTaskInGroup`) → `DeleteTask` (`DELETE FROM social.todo_tasks`, `internal/repository/sqlc/todo.sql.go:239`).

## Tests
- `internal/handler/http/martin_unit_test.go`: `TestMartin_TodoPatchDelete_Unit` (200 + doble delete 404).
- `internal/handler/http/martin_integration_test.go`: `TestMartinIntegration_TodoCRUD_PG` (la fila desaparece).
- `internal/handler/http/martin_e2e_test.go`: `TestMartinE2E_TodoJourney` (GET vacío tras DELETE).
- `internal/handler/http/tasks_routes_test.go`: `TestTodoTaskIdRoutes_Contrato` (DELETE extraño → 403).
