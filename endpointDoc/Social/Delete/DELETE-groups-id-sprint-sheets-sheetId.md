# `DELETE /groups/:id/sprint-sheets/:sheetId`


## Req

- `DELETE /groups/:id/sprint-sheets/:sheetId`, JWT y membresía requeridos; registro `back/social/cmd/server/main.go:284`.
- :id y :sheetId identifican grupo y hoja. Sin body ni query.
- La última hoja del grupo no se puede borrar.

## Res

- `200`: {"data":{"id":"...","name":"...","period_start":"...","period_end":"..."}} con la hoja eliminada.
- 400 con error invalid_body si es la única hoja; 401 sin sesión; 403 no miembro; 404 grupo/hoja ausente; 500 error interno. sheetError usa {"error":{"code":"...","message":"..."}}.
- PostgreSQL elimina en cascada las tareas y horas vinculadas a la hoja. No hay borrado manual en handler.

## Invocación (Front / Otros Servicios)

- `front/lib/core/services/social_service.dart:494`, `deleteSprintSheet` envía DELETE y requiere éxito.
- `_SprintSheetScreenState._confirmDeleteSheet` envía la eliminación y vuelve a cargar la hoja seleccionada (`front/lib/features/workspace/sprint_sheet_screen.dart:1380-1434`).
- Sin consumidor de otro microservicio.

## Flujo

1. `main()` registra `DELETE /groups/:id/sprint-sheets/:sheetId` (`back/social/cmd/server/main.go:284`).
2. `SprintHandler.DeleteSheet` obtiene el usuario y los IDs y construye la respuesta (`back/social/internal/handler/http/sprint_sheets.go:59-70`).
3. `SprintService.DeleteSprintSheet` valida acceso, confirma que hay más de una hoja y delega el borrado (`back/social/internal/service/sprint_sheets.go:96-119`).
4. `SprintRepository.DeleteSheet` llama la consulta sqlc (`back/social/internal/repository/sprint_repository.go:70-72`); `Queries.DeleteSheet` elimina la fila (`back/social/internal/repository/sqlc/sprint.sql.go:302-318`) y PostgreSQL aplica las cascadas de tareas/horas.
5. El handler serializa la fila eliminada con `200` (`back/social/internal/handler/http/sprint_sheets.go:59-70`). `_confirmDeleteSheet` recibe éxito y recarga seleccionando una hoja vecina (`front/lib/features/workspace/sprint_sheet_screen.dart:1380-1434`).

## Front
- Cliente: `SocialService.deleteSprintSheet` (`front/lib/core/services/social_service.dart:494`); acepta `200` o `204`; el error del backend (p. ej. último sprint protegido) se muestra tal cual vía `_toError`.
- Pantalla: `_SprintSheetScreenState` (`front/lib/features/workspace/sprint_sheet_screen.dart:1420`), eliminar sprint.

## Tests

- `back/social/internal/handler/http/sprint_sheets_test.go:13`, `TestSprintSheetRoutes`.
- `back/social/internal/service/sprint_sheets_test.go:108`, `TestDeleteSprintSheet` cubre hojas múltiples, última hoja y aislamiento.
