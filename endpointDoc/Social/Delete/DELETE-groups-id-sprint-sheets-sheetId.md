# DELETE /groups/:id/sprint-sheets/:sheetId

Elimina una hoja de sprint con sus tareas y horas (microservicio `social`). El grupo nunca queda sin hojas.

## Req
- Método: `DELETE`, ruta: `/groups/:id/sprint-sheets/:sheetId` (protegida con JWT `Bearer`; cualquier miembro, misma regla que crear/editar; `back/social/cmd/server/main.go:267`).
- Headers: `Authorization: Bearer <access_token>`.
- Path: `id` (uuid del grupo), `sheetId` (uuid de la hoja).

## Res
- `200`: `{"data":{"id","name","period_start","period_end"}}` (la hoja borrada).
- `400` `invalid_body` con mensaje `"No puedes eliminar el único sprint del grupo."` si es la última hoja; también para uuid inválido. `403` `forbidden`. `404` `not_found`. `401` sin body.

## Flujo
- `SprintHandler.DeleteSheet` (`internal/handler/http/sprint_sheets.go:59`) → `SprintService.DeleteSprintSheet` (`internal/service/sprint_sheets.go:96`: membresía, hoja del grupo, cuenta hojas y rechaza la última) → `DeleteSheet` (sqlc). Tareas y `sprint_sheet_daily_hours` caen por `ON DELETE CASCADE`.

## Tests
- `internal/service/sprint_sheets_test.go`: `TestDeleteSprintSheet`.
- `internal/handler/http/sprint_sheets_test.go`: `TestSprintSheetRoutes`.
- `test/social_routes_e2e_test.go`: subtest `todo_sprint_hours_and_workspace` (hoja eliminada deja de listarse).
