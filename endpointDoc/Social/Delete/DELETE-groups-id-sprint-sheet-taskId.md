# DELETE /groups/:id/sprint-sheet/:taskId

Elimina una tarea de sprint (microservicio `social`).

## Req
- Método: `DELETE`, ruta: `/groups/:id/sprint-sheet/:taskId` (protegida con JWT `Bearer`; exige membresía; `back/social/cmd/server/main.go:208`).
- Path: `id` + `taskId` (uuid). Sin body.

## Res
- `200`: `{"message":"sprint task deleted successfully","data":{...tarea eliminada...}}`.
- `400`: `invalid_group_id|invalid_sprint_task_id`. `401`: `unauthorized`. `403`: `forbidden`. `404`: `group_not_found|sprint_task_not_found` (doble delete → `404`). `500`: `internal`.

## Flujo
- `SprintHandler.DeleteSprintTask` (`internal/handler/http/sprint_handler.go:257-309`) → `SprintService.DeleteSprintTask` (`internal/service/sprint_service.go:471-520`: `requireGroup` + `requireMembership`, `GetTaskByID` + `requireTaskInGroup`) → `DeleteTask` (`DELETE FROM social.sprint_sheet_tasks`, `internal/repository/sqlc/sprint.sql.go:259`).

## Tests
- `internal/handler/http/martin_unit_test.go`: `TestMartin_SprintPatchDelete_Unit` (200 + doble delete 404).
- `internal/handler/http/martin_integration_test.go`: `TestMartinIntegration_SprintCRUD_PG` (la fila desaparece).
- `internal/handler/http/martin_e2e_test.go`: `TestMartinE2E_SprintJourney` (GET vacío tras DELETE).
- `internal/handler/http/tasks_routes_test.go`: `TestSprintTaskIdRoutes_Contrato` (DELETE ok).
