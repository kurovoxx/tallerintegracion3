# `POST /groups/:id/sprint-sheets`


## Req

- `POST /groups/:id/sprint-sheets`, JWT y membresía requeridos; registro `back/social/cmd/server/main.go:282`.
- JSON: name, period_start y period_end requeridos; las fechas usan YYYY-MM-DD. No acepta un sheetId en path.
- Aplican límite de 200 caracteres al nombre, fechas válidas y end no anterior al start.

## Res

- `201`: {"data":{"id":"<uuid>","name":"Sprint 1","period_start":"YYYY-MM-DD","period_end":"YYYY-MM-DD"}}.
- 400 por body/fecha/nombre inválido; 401 sesión inválida; 403 no miembro; 404 grupo ausente; 500 interno. sheetError usa {"error":{"code":"...","message":"..."}}.
- Inserta una hoja; la lista del grupo cambia tras guardar en PostgreSQL.

## Invocación (Front / Otros Servicios)

- `front/lib/core/services/social_service.dart:459`, `saveSprintSheet` envía POST cuando sheetId es null.
- `_SprintSheetScreenState._manageSprint` abre `SprintEditorDialog`, llama `SocialService.saveSprintSheet` y recarga la hoja seleccionada (`front/lib/features/workspace/sprint_sheet_screen.dart:230-253`).
- No se encontró consumidor de producción desde otro servicio.

## Flujo

1. `main()` registra `POST /groups/:id/sprint-sheets` (`back/social/cmd/server/main.go:282`).
2. `SprintHandler.CreateSheet` delega en `SprintHandler.writeSheet`, que toma el actor, enlaza el JSON y prepara la respuesta (`back/social/internal/handler/http/sprint_sheets.go:57,72-97`).
3. `SprintService.CreateSprintSheet` valida acceso y campos (`back/social/internal/service/sprint_sheets.go:52-65`); `applySheetPatch` normaliza nombre y fechas (`back/social/internal/service/sprint_sheets.go:122-142`).
4. `SprintRepository.CreateSheet` delega en sqlc (`back/social/internal/repository/sprint_repository.go:42-44`); `Queries.CreateSheet` ejecuta el `INSERT INTO social.sprint_sheets` (`back/social/internal/repository/sqlc/sprint.sql.go:61-85`).
5. `SprintHandler.writeSheet` responde `201` con `data` usando `sheetJSON` (`back/social/internal/handler/http/sprint_sheets.go:12-20,83-97`). `_SprintSheetScreenState._manageSprint` toma el ID y recarga la hoja creada (`front/lib/features/workspace/sprint_sheet_screen.dart:230-253`).

## Tests

- `back/social/internal/handler/http/sprint_sheets_test.go:13`, `TestSprintSheetRoutes`.
- `back/social/internal/service/sprint_sheets_test.go:10`, `TestSprintSheetsLifecycleAndIsolation`.
