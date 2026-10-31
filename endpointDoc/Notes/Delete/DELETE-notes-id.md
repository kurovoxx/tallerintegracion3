# `DELETE /notes/:id`


## Req

- `DELETE /notes/:id`, JWT requerido: `back/notes/cmd/server/main.go:209`.
- id es UUID de una nota propia. No hay body ni query.
- El user_id siempre sale del JWT.

## Res

- `204 No Content`: elimina la metadata local y deja en la outbox los archivos que deben borrarse de Drive.
- `401 Unauthorized`: falta o no es válido el JWT; body `{"error":{"code":"unauthorized","message":"No autorizado: token ausente, inválido o expirado"}}`.
- `404 Not Found`: UUID malformado, nota inexistente o de otra cuenta; body `{"error":{"code":"not_found","message":"nota no encontrada"}}`.
- `500 Internal Server Error`: fallo al leer/eliminar metadata o persistir la outbox; body `{"error":{"code":"internal_error","message":"Error interno"}}`. `handleServiceError` aplica ese mapeo (`back/notes/internal/handler/http/note_handler.go:890-941`).
- El fallo de Drive posterior no cambia el `204`: `NoteService.Delete` registra la intención durable antes de retirar metadata (`back/notes/internal/service/note_service.go:1367-1405`). Este endpoint borra una nota elegida explícitamente; el borrado de cuenta conserva notas propias.

## Invocación (Front / Otros Servicios)

- `_deleteNoteRemote` y `_deleteNoteRemoteById` envían el DELETE autenticado (`front/lib/features/notes/all_notes_screen.dart:1736-1758`).
- No se encontró consumidor de producción de otro servicio.

## Flujo

1. `main()` registra `DELETE /notes/:id` bajo autenticación (`back/notes/cmd/server/main.go:209`).
2. `NoteHandler.Delete` obtiene el actor, valida el ID y traduce el resultado a `204` o error (`back/notes/internal/handler/http/note_handler.go:357-373`).
3. `NoteService.Delete` comprueba existencia/autoría y llama al almacenamiento (`back/notes/internal/service/note_service.go:1367-1404`).
4. `PGNoteStore.DeleteWithDriveCleanup` delega a `NoteRepository.DeleteWithDriveCleanup` (`back/notes/internal/service/store_pg.go:200-202`; `back/notes/internal/repository/note_repository.go:489-565`), que guarda operaciones de Drive en la outbox antes de borrar metadata y shares en PostgreSQL.
5. `NoteService.Delete` intenta las operaciones externas; si Drive falla, la outbox conserva el trabajo para `NoteService.reconcileDriveOperations` (`back/notes/internal/service/note_service.go:1416-1430,2913-2954`). `_deleteNoteRemote` consume el `204` y la pantalla actualiza sus datos (`front/lib/features/notes/all_notes_screen.dart:1736-1758`).

## Tests

- `back/notes/internal/handler/http/note_handler_test.go:1098`, `TestHandlerDeleteNoteAndAllFiles`.
- `back/notes/internal/service/delete_cleanup_test.go:41`, `TestDeleteNoteAllDriveFiles`; :150, tolerancia/recovery del cleanup.
- `back/notes/internal/service/rollback_test.go:271`, `fallo` PG conserva archivo; :303, fallo Drive no pierde el cleanup local.
