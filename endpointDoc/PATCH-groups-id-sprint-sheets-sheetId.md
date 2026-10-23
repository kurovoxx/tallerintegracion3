# PATCH /groups/:id/sprint-sheets/:sheetId

Edita nombre y/o rango de una hoja de sprint (microservicio `social`).

## Req
- Método: `PATCH`, ruta: `/groups/:id/sprint-sheets/:sheetId` (protegida con JWT `Bearer`; cualquier miembro; `back/social/cmd/server/main.go:266`).
- Headers: `Content-Type: application/json`, `Authorization: Bearer <access_token>`.
- Path: `id` (uuid del grupo), `sheetId` (uuid de la hoja; debe pertenecer al grupo).
- Body parcial (`service.SheetPatch`): al menos uno de `name`, `period_start`, `period_end`. El rango resultante debe quedar completo (ambas fechas o ninguna) y ordenado.

## Res
- `200`: `{"data":{"id","name","period_start","period_end"}}`.
- `400` `invalid_body` (body vacío, nombre inválido, rango incompleto o invertido, uuid malo). `403` `forbidden`. `404` `not_found` (grupo u hoja de otro grupo). `401` sin body.

## Flujo
- `SprintHandler.UpdateSheet` → `writeSheet(update=true)` (`internal/handler/http/sprint_sheets.go:72`) → `SprintService.UpdateSprintSheet` (`internal/service/sprint_sheets.go:67`: membresía, `resolveSheet` dentro del grupo, aplica el parche sobre el valor actual) → `UpdateSheet` (sqlc, filtra por `id` y `group_id`).

## Tests
- `internal/handler/http/sprint_sheets_test.go`: `TestSprintSheetRoutes`.
- `internal/service/sprint_sheets_test.go`: `TestSprintSheetsLifecycleAndIsolation`, `TestLegacySprintAcceptsDatesWithoutReplacingSheet`.
