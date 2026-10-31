# `GET /groups/:id/sprint-sheets`


## Req

- `GET /groups/:id/sprint-sheets`, JWT requerido; registro `back/social/cmd/server/main.go:281-284`.
- :id UUID de grupo. Sin body ni query. El usuario debe pertenecer al grupo.
- Ruta del servicio Social; web gateway usa base `/api-social.`

## Res

- `200`: {"data":[{"id":"<uuid>","name":"Sprint 1","period_start":"2026-09-01","period_end":"2026-09-14"}]}. Fechas pueden ser null en hojas históricas.
- 401 no autenticado; 403 no miembro; 404 grupo/UUID no encontrado; 500 interno. sheetError usa {"error":{"code":"...","message":"..."}}.
- Solo metadata de sprint, no la lista de tareas.

## Invocación (Front / Otros Servicios)

- `_SprintSheetScreenState._loadReal` carga las hojas y usa la lista como respaldo (`front/lib/features/workspace/sprint_sheet_screen.dart:161-196`). `SocialService.listSprintSheets` envía el GET y convierte `data` a `SprintSheetInfo` (`front/lib/core/services/social_service.dart:443-457`).
- No se encontró consumidor backend.

## Flujo

1. `main()` registra `GET /groups/:id/sprint-sheets` (`back/social/cmd/server/main.go:281`).
2. `SprintHandler.ListSheets` toma el usuario y devuelve la lista o un error (`back/social/internal/handler/http/sprint_sheets.go:40-55`).
3. `SprintService.ListSprintSheets` valida grupo y membresía (`back/social/internal/service/sprint_sheets.go:40-50`) y `SprintRepository.ListSheetsByGroup` consulta sqlc (`back/social/internal/repository/sprint_repository.go:38-40`). `Queries.ListSheetsByGroup` ejecuta la consulta ordenada por período (`back/social/internal/repository/sqlc/sprint.sql.go:24-52`).
4. `sheetJSON` adapta cada fila al JSON de respuesta (`back/social/internal/handler/http/sprint_sheets.go:12-22`); `SprintHandler.ListSheets` responde `200` (`back/social/internal/handler/http/sprint_sheets.go:40-55`). Flutter convierte ese `data` en hojas y actualiza la pantalla en `_loadReal` (`front/lib/core/services/social_service.dart:443-457`; `front/lib/features/workspace/sprint_sheet_screen.dart:161-196`).

## Tests

- `back/social/internal/handler/http/sprint_sheets_test.go:13`, `TestSprintSheetRoutes`.
- `back/social/internal/service/sprint_sheets_test.go:10`, `TestSprintSheetsLifecycleAndIsolation`.
