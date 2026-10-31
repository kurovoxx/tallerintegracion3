# POST /groups/:id/sprint-sheets

Crea una hoja (sprint) en el grupo (microservicio `social`).

## Req
- Método: `POST`, ruta: `/groups/:id/sprint-sheets` (protegida con JWT `Bearer`; cualquier miembro; `back/social/cmd/server/main.go:265`).
- Headers: `Content-Type: application/json`, `Authorization: Bearer <access_token>`.
- Path: `id` (uuid del grupo).
- Body (`service.SheetPatch`): `{"name":"Sprint 1","period_start":"2026-10-01","period_end":"2026-10-14"}`. Los tres obligatorios; `name` 1-200 caracteres (se recorta); `period_end` >= `period_start`.

## Res
- `201`: `{"data":{"id","name","period_start","period_end"}}`.
- `400` `invalid_body` (faltan campos, fechas inválidas o invertidas). `403` `forbidden`. `404` `not_found`. `401` sin body.

## Flujo
- `SprintHandler.CreateSheet` → `writeSheet(update=false)` (`internal/handler/http/sprint_sheets.go:72`) → `SprintService.CreateSprintSheet` (`internal/service/sprint_sheets.go:52`: membresía + `applySheetPatch`) → `CreateSheet` (sqlc).

## Tests
- `internal/handler/http/sprint_sheets_test.go`: `TestSprintSheetRoutes`.
- `internal/service/sprint_sheets_test.go`: `TestSprintSheetsLifecycleAndIsolation`.
- `test/social_routes_e2e_test.go`: subtest `todo_sprint_hours_and_workspace`.
