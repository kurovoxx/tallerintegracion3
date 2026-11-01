# PATCH /groups/:id/sprint-sheet/:taskId

Actualización parcial de una tarea de sprint (microservicio `social`).

## Req
- Método: `PATCH`, ruta: `/groups/:id/sprint-sheet/:taskId` (protegida con JWT `Bearer`; exige membresía; `back/social/cmd/server/main.go:207`).
- Path: `id` + `taskId` (uuid).
- Body parcial (`UpdateSprintTaskRequest`, `internal/handler/http/sprint_handler.go:168-174`): ≥1 de `title, assigned_to, priority, status, estimated_hours` (si no → `400`). Ej: `{"priority":"alta","status":"en_proceso"}`.
- Ojo: `assigned_to` no admite `""` para desasignar (vacío → `400 assigned_to is required`).

## Res
- `200`: `{"message":"sprint task updated successfully","data":{...}}`.
- `400`: `invalid_group_id|invalid_sprint_task_id|invalid_body|invalid_user_id`. `401`: `unauthorized`. `403`: `forbidden`. `404`: `group_not_found|sprint_task_not_found` (tarea de otro grupo → `404` sin filtrar). `500`: `internal`.

## Flujo
- `SprintHandler.UpdateSprintTask` (`sprint_handler.go:176-255`) → `SprintService.UpdateSprintTask` (`internal/service/sprint_service.go:362-469`: `ErrNothingToPatch` si vacío, `requireGroup` + `requireMembership`, `GetTaskByID` + `requireTaskInGroup`, validación por campo) → `UpdateTask` (`UPDATE social.sprint_sheet_tasks`, `internal/repository/sqlc/sprint.sql.go:214`) + re-lectura de la hoja.

## Tests
- `internal/handler/http/martin_unit_test.go`: `TestMartin_SprintPatchDelete_Unit` (parcial, scoping, nothing-to-patch, 404).
- `internal/handler/http/martin_integration_test.go`: `TestMartinIntegration_SprintCRUD_PG` (persiste `alta`/`en_proceso`).
- `internal/handler/http/martin_e2e_test.go`: `TestMartinE2E_SprintJourney` (PATCH por HTTP real).
- `internal/handler/http/tasks_routes_test.go`: `TestSprintTaskIdRoutes_Contrato` (404 con otro grupo, 403 extraño).
