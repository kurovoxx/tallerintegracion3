# GET /groups/:id/sprint-sheets

Lista las hojas (sprints) del grupo (microservicio `social`). Las tareas viven en `/groups/:id/sprint-sheet` (singular).

## Req
- Método: `GET`, ruta: `/groups/:id/sprint-sheets` (protegida con JWT `Bearer`; solo miembros; `back/social/cmd/server/main.go:264`).
- Headers: `Authorization: Bearer <access_token>`.
- Path: `id` (uuid del grupo).

## Res
- `200`: `{"data":[{"id","name","period_start":"YYYY-MM-DD"|null,"period_end":"YYYY-MM-DD"|null}]}`. Hojas antiguas pueden no tener rango.
- Errores con forma `{"error":{"code","message"}}`: `403` `forbidden`, `404` `not_found` (grupo), `400` `invalid_body` (uuid inválido). `401` sin body.

## Flujo
- `SprintHandler.ListSheets` (`internal/handler/http/sprint_sheets.go:40`) → `SprintService.ListSprintSheets` (`internal/service/sprint_sheets.go:44`: `sheetAccess` = grupo existe + membresía) → `ListSheetsByGroup` (sqlc).

## Tests
- `internal/handler/http/sprint_sheets_test.go`: `TestSprintSheetRoutes`.
- `internal/service/sprint_sheets_test.go`: `TestSprintSheetsLifecycleAndIsolation`.
