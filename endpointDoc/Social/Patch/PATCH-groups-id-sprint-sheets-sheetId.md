# `PATCH /groups/:id/sprint-sheets/:sheetId`


## Req

- `PATCH /groups/:id/sprint-sheets/:sheetId`, JWT y membresía requeridos; registro `back/social/cmd/server/main.go:283`.
- JSON parcial con uno o más campos: name, period_start, period_end. Fecha YYYY-MM-DD; los campos no enviados conservan su valor.
- El cliente identifica grupo y hoja por UUID.

## Res

- `200`: {"data":{"id":"<uuid>","name":"Sprint actualizado","period_start":"YYYY-MM-DD","period_end":"YYYY-MM-DD"}}.
- 400 si no hay campos editables, fecha/nombre inválido; 401 sin JWT; 403 no miembro; 404 grupo/hoja ausente; 500 interno. sheetError usa {"error":{"code":"...","message":"..."}}.
- Solo actualiza la hoja perteneciente a ese grupo.

## Invocación (Front / Otros Servicios)

- `front/lib/core/services/social_service.dart:459`, `saveSprintSheet` envía PATCH cuando sheetId tiene valor.
- `_SprintSheetScreenState._manageSprint` abre `SprintEditorDialog`, llama a `saveSprintSheet` y recarga la hoja actualizada (`front/lib/features/workspace/sprint_sheet_screen.dart:230-253`).
- No se encontró consumidor backend.

## Flujo

1. `main()` registra la ruta PATCH (`back/social/cmd/server/main.go:283`).
2. `SprintHandler.UpdateSheet` delega en `writeSheet(update=true)`, que toma el actor del JWT, valida el JSON y llama al servicio (`back/social/internal/handler/http/sprint_sheets.go:58,72-96`).
3. `SprintService.UpdateSprintSheet` valida membresía, IDs y campos; `SprintRepository.UpdateSheet` delega a sqlc (`back/social/internal/service/sprint_sheets.go:67-89`; `back/social/internal/repository/sprint_repository.go:66-68`).
4. `Queries.UpdateSheet` actualiza la fila por hoja y grupo (`back/social/internal/repository/sqlc/sprint.sql.go:295-300`). El handler responde `200` con `sheetJSON`; `_manageSprint` procesa el `data` y recarga la hoja (`back/social/internal/handler/http/sprint_sheets.go:12-22,83-96`; `front/lib/features/workspace/sprint_sheet_screen.dart:230-253`). No hay llamada externa.

## Front
- Cliente: `SocialService.saveSprintSheet` (`front/lib/core/services/social_service.dart:459`) con `sheetId`; mismo body y response que la creación (`data` → `SprintSheetInfo`).
- Pantalla: `_SprintSheetScreenState` (`front/lib/features/workspace/sprint_sheet_screen.dart:242`), edición del sprint desde `_NewSprintDialog`.

## Tests

- `back/social/internal/handler/http/sprint_sheets_test.go:13`, `TestSprintSheetRoutes`.
- `back/social/internal/service/sprint_sheets_test.go:10`, `TestSprintSheetsLifecycleAndIsolation`; :78 conserva hojas legacy sin rango.
