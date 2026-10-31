# `POST /notes/reconcile`


## Req

- `POST /notes/reconcile`, JWT requerido: `back/notes/cmd/server/main.go:197`.
- Sin body, query ni parámetros. La cuenta se selecciona exclusivamente por JWT.
- Ruta raíz Notes; `/api-notes` es prefijo externo del gateway, no parte de la ruta Gin.

## Res

- `200 OK`: `{"removed_notes":0,"removed_attachments":0,"pending":0,"removed_note_ids":[]}`; los valores son sintéticos. `removed_note_ids` puede ser `null` si no se eliminó ninguna nota.
- `401 Unauthorized`: JWT ausente/inválido/expirado; error `unauthorized`.
- `403 Forbidden`: OAuth Drive ausente/revocado; `forbidden` con `Conecte o renueve su Google Drive`.
- `502 Bad Gateway`: Drive no disponible al listar o confirmar; `internal_error` con `Servicio de almacenamiento no disponible`.
- `500 Internal Server Error`: PostgreSQL u otro fallo interno; `internal_error` con `Error interno`.
- Errores por elemento que no permiten confirmar el borrado incrementan `pending` y la respuesta sigue siendo `200`; no se borra metadata ante una ausencia no confirmada.
- El envelope de error es `{"error":{"code":"...","message":"..."}}`; `handleServiceError` traduce los casos (`back/notes/internal/handler/http/note_handler.go:890-941`) y `respondNotesError` crea el JSON (`back/notes/internal/handler/http/telemetry.go:56-61`).

## Invocación (Front / Otros Servicios)

- `AllNotesScreen._syncFromBackendImpl` dispara la reconciliación durante la sincronización (`front/lib/features/notes/all_notes_screen.dart:994`); `_reconcileDriveDeletions` arma y envía el POST (`front/lib/features/notes/all_notes_screen.dart:1054-1060`).
- La misma función procesa `removed_note_ids` para actualizar almacenamiento local (`front/lib/features/notes/all_notes_screen.dart:1061-1092`). No se encontró otro consumidor de producción.

## Flujo

1. `main()` registra `POST /notes/reconcile` bajo autenticación (`back/notes/cmd/server/main.go:197`).
2. `NoteHandler.Reconcile` toma el usuario del JWT, valida el body y devuelve resumen/error (`back/notes/internal/handler/http/note_handler.go:379-416`).
3. `NoteService.ReconcileDriveDeletions` obtiene los IDs vigentes desde Drive y pagina las notas propias (`back/notes/internal/service/reconcile_drive.go:26-58`); `RealDriveClient.ListAppFileIDs` y `PGNoteStore.ListByUser` ejecutan esas consultas (`back/notes/internal/drive/real_client.go:366-400`; `back/notes/internal/service/store_pg.go:28-30`; `back/notes/internal/repository/note_repository.go:88-120`).
4. `NoteService.reconcileOneNote` verifica cada ausencia con `confirmDriveGone` y solo entonces delega al borrado normal (`back/notes/internal/service/reconcile_drive.go:62-97,185-217`; `back/notes/internal/service/note_service.go:1367-1404`). Errores no concluyentes incrementan pendientes.
5. `NoteHandler.Reconcile` serializa los conteos e IDs de notas propias (`back/notes/internal/handler/http/note_handler.go:379-416`); `_reconcileDriveDeletions` procesa esa respuesta (`front/lib/features/notes/all_notes_screen.dart:1054-1092`).

## Tests

- `back/notes/internal/handler/http/note_handler_test.go:1151`, `TestHandlerReconcileDriveDeletions`.
- `back/notes/internal/service/reconcile_drive_test.go:15`, `conservar` notas existentes; :35 y :140, eliminar solo ante ausencia confirmada; :148, timeout no borra; :175, OAuth revocado no borra.
- `front/test/notes_reconcile_test.dart` cubre el refresco local con respuestas simuladas; no sustituye las pruebas backend.
