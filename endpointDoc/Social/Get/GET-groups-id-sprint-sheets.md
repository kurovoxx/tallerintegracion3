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

## Front
- Cliente: `SocialService.listSprintSheets` (`front/lib/core/services/social_service.dart:443`); lee `data[]` → `SprintSheetInfo` (`id, name, period_start, period_end`).
- Pantalla: `_SprintSheetScreenState._loadReal` (`front/lib/features/workspace/sprint_sheet_screen.dart:177`), chips de selección de sprint (`_SprintChip`) y calendario de días a partir de `period_start`/`period_end`.

## Tests
- `internal/handler/http/sprint_sheets_test.go`: `TestSprintSheetRoutes`.
- `internal/service/sprint_sheets_test.go`: `TestSprintSheetsLifecycleAndIsolation`.
