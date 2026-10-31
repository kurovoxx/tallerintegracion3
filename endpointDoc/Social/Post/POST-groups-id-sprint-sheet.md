# POST /groups/:id/sprint-sheet

Crea una tarea de sprint (microservicio `social`). Diferencia clave vs todo: `assigned_to` es **obligatorio**.

## Req
- Método: `POST`, ruta: `/groups/:id/sprint-sheet` (protegida con JWT `Bearer`; exige membresía; `back/social/cmd/server/main.go:205`).
- Headers: `Content-Type: application/json`, `Authorization: Bearer <access_token>`.
- Path: `id` (uuid del grupo).
- Body (`CreateSprintTaskRequest`, `internal/handler/http/sprint_handler.go:25-32`): `{"title":"API login","assigned_to":"<uuid-miembro>","sheet_id":"uuid-opcional","priority":"media","status":"sin_empezar","estimated_hours":5}`.
- `title` requerido max 300. `assigned_to` requerido (uuid, debe ser miembro). `priority`: `alta|media|baja` (vacío = `media`). `status`: `sin_empezar|en_proceso|listo` (vacío = `sin_empezar`). `estimated_hours`: número ≥ 0 (vacío = 0).

## Res
- `201`: `{"message":"sprint task created successfully","data":{"id":"uuid","group_id":"uuid","sheet_id":"uuid","sheet_name":"Sprint 1","title":"...","assigned_to":"uuid","priority":"media","status":"sin_empezar","estimated_hours":5,"created_at":"RFC3339","updated_at":"RFC3339"}}` (`sprintTaskToJSON`, `sprint_handler.go:311-325`).
- `400`: `invalid_group_id|invalid_sheet_id|invalid_user_id|invalid_body` (título, prioridad, estado, `assigned_to is required`, horas < 0). `401`: `unauthorized`. `403`: `forbidden`. `404`: `group_not_found|sheet_not_found`. `500`: `internal`.

## Flujo
- `SprintHandler.CreateSprintTask` (`sprint_handler.go:34-109`) → `SprintService.CreateSprintTask` (`internal/service/sprint_service.go:215-275`: `requireGroup` + `requireMembership` + `requireAssigneeMember`, `resolveSheet` usa la hoja dada o la primera o crea `"Sprint 1"`) → `CreateTask` (`INSERT INTO social.sprint_sheet_tasks`, `internal/repository/sqlc/sprint.sql.go:111`).

## Tests
- `internal/handler/http/martin_unit_test.go`: `TestMartin_SprintCreateList_Unit` (sin `assigned_to` → 400, defaults, horas negativas).
- `internal/handler/http/martin_integration_test.go`: `TestMartinIntegration_SprintCRUD_PG` (defaults en DB).
- `internal/handler/http/martin_e2e_test.go`: `TestMartinE2E_SprintJourney` (viaje HTTP real).
- `internal/handler/http/tasks_routes_test.go`: `TestSprintTaskIdRoutes_Contrato` (asignado extraño → 400).
