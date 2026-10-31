# GET /groups/:id/sprint-sheet

Lista tareas de sprint con filtros (microservicio `social`).

## Req
- Método: `GET`, ruta: `/groups/:id/sprint-sheet` (protegida con JWT `Bearer`; exige membresía; `back/social/cmd/server/main.go:206`).
- Path: `id` (uuid del grupo). Query opcional: `?status=sin_empezar|en_proceso|listo&priority=alta|media|baja&sheet_id=<uuid>`.

## Res
- `200`: `{"data":[{"id":"uuid","group_id":"uuid","sheet_id":"uuid","sheet_name":"...","title":"...","assigned_to":"uuid","priority":"...","status":"...","estimated_hours":0,"created_at":"RFC3339","updated_at":"RFC3339"}]}`.
- `400`: `invalid_group_id|invalid_status|invalid_priority|invalid_sheet_id`. `401`: `unauthorized`. `403`: `forbidden`. `404`: `group_not_found|sheet_not_found`. `500`: `internal`.

## Flujo
- `SprintHandler.ListSprintTasks` (`internal/handler/http/sprint_handler.go:111-166`) → `SprintService.ListSprintTasks` (`internal/service/sprint_service.go:289-360`: valida filtros, `requireGroup` + `requireMembership`, `resolveSheet` solo si se filtra por `sheet_id`) → `ListTasksByGroup` (`JOIN social.sprint_sheet_tasks + social.sprint_sheets`, `internal/repository/sqlc/sprint.sql.go:153`).

## Tests
- `internal/handler/http/martin_unit_test.go`: `TestMartin_SprintCreateList_Unit` (filtros, 400s, 403).
- `internal/handler/http/martin_integration_test.go`: `TestMartinIntegration_SprintCRUD_PG` (filtro `priority=media` → 1).
- `internal/handler/http/martin_e2e_test.go`: `TestMartinE2E_SprintJourney` (GET tras crear/borrar).
