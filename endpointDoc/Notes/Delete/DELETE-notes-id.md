# DELETE /notes/{id}

Borra una nota propia: primero su metadata y dependencias en Postgres (en una transacción que deja encolada la limpieza de Drive) y después, de forma best-effort, el `.md` y los adjuntos en Drive. Ruta interna del backend Notes (`:8082`). Desde el navegador: `DELETE https://ti3-brojas.dev.censei.cl/api-notes/notes/<id>` ([prefijos](../COMUN-notes.md#1-rutas-internas-y-prefijos-externos)). Equivalente gRPC: `NotesService/DeleteNote`.

## Req

- **Registro:** `back/notes/cmd/server/main.go:209` → `protected.DELETE("/notes/:id", h.Delete)`. Handler `back/notes/internal/handler/http/note_handler.go:357-373` (`Delete`).
- **Autenticación:** `RequireAuth` (`main.go:192`). Solo el autor; para cualquier otro usuario responde 404 (`back/notes/internal/service/note_service.go:1378-1380`). Ver [COMUN §2](../COMUN-notes.md#2-autenticación-http).
- **Path:** `id` UUID obligatorio (no UUID → 404 `not_found`).
- **Query / body:** no tiene.

## Res

**204 No Content**, sin body (`note_handler.go:372`).

**Errores**

| HTTP | `error.code` | Condición |
| --- | --- | --- |
| 404 | `not_found` "nota no encontrada" | No existe, no es del usuario, ya se borró (un segundo DELETE da 404) o se borró en paralelo (`note_service.go:1371-1392`) |
| 500 | `internal_error` "Error interno" | Fallo de Postgres en la lectura o en la transacción de borrado (`L1372-1374`, `L1393-1396`) |
| 401 / 423 / 503 | ver [COMUN §2](../COMUN-notes.md#2-autenticación-http) | Middleware |

Un fallo de Drive **nunca** produce error: la respuesta sigue siendo 204 (`L1416-1429`).

**Efectos antes de responder**

1. Transacción en Postgres (`NoteRepository.DeleteWithDriveCleanup`, `back/notes/internal/repository/note_repository.go:489-573`):
   - `SELECT … FOR UPDATE` de la nota y verificación de dueño (`L497-506`).
   - Lee los adjuntos (`L512-527`) y encola en `notes.drive_reconciliation_queue` un `delete_attachment` por adjunto con archivo y un `delete_file` para el `.md` (`L528-540`).
   - Borra `notes.note_attachments`, `notes.saved_notes`, `notes.note_likes`, `notes.shared_notes` de esa nota (`L544-553`), la fila de `notes.notes` (`L556-562`) y las filas de `notes.drive_managed_permissions` (`L566-568`). Commit (`L569`).
2. Fuera de la transacción: `DeleteManagedPermissionsByNote` (redundante, solo se registra si falla) (`note_service.go:1401-1403`).
3. Drive, síncrono y best-effort: por cada archivo ejecuta `executeDriveOperation` (`delete_file` / `delete_attachment`) con el token del autor, 15 s y 3 intentos (`L1405-1429`, `L2956-2973`). Un 404 de Drive cuenta como éxito.

**Permisos de Drive:** no se revocan uno a uno; al borrar el archivo desaparecen sus permisos (ver [COMUN §5](../COMUN-notes.md#5-ciclo-de-vida-de-los-permisos-de-drive)).

**Segundo plano:** las filas encoladas en `notes.drive_reconciliation_queue` las procesa el reconciliador (cada 30 s, `reconcileDriveOperations` `note_service.go:2913-2954`), que las marca completadas o las reprograma con backoff hasta 1 h. Si el borrado síncrono ya funcionó, el reintento vuelve a llamar a Drive y obtiene 404 (éxito).

## Invocación (Front / Otros Servicios)

- **Pantalla:** detalle de nota propia (`_NoteDetailSheet`, `front/lib/features/notes/all_notes_screen.dart:3802`). Botón **Eliminar** (`L4951-4955`) → diálogo "ELIMINAR NOTA" (`_confirmDelete`, `L4123-4162`) → `widget.onDelete` (`L4141`) → `_deleteFlow` (`L2004`) → `_deleteNoteRemote` / `_deleteNoteRemoteById` (`L1736-1766`).
- **Petición:** `DELETE $notesBaseUrl/notes/<id>` (`L1740`) con `_authHeaders()` (Bearer `SessionManager.token`), timeout 30 s, vía `AuthedHttp.run`.
- **Respuesta:**
  - 200/204 → borra la nota local (`_applyLocalDelete`, `L2028-2034`).
  - 404 → también la borra localmente y avisa "La nota ya no existía en el servidor; se eliminó la copia local." (`L2035-2049`).
  - Sin red → la borra localmente y guarda un tombstone con `remotePending: true` en Drift (`L2050-2060`; tabla `note_tombstones`, `front/lib/core/database/app_database.dart:36`). **REFRESCAR / SINCRONIZAR DRIVE** reintenta esos DELETE (`L886-905`).
  - Nota local sin UUID cuyo POST quizá llegó al servidor: oculta la nota, reenvía el POST con la misma clave para obtener el UUID y luego borra (`L2007-2024`, `_settleDeletedPendingCreate` `L1522-1538`).
- **Otros microservicios:** No se encontró invocación HTTP. Internamente el reconciliador llama a `NoteService.Delete` cuando confirma que el `.md` ya no existe en Drive (`note_service.go:2906`).
- **Tests/scripts:** `scripts/test-notes-e2e.py:113` (DELETE de otro usuario → 404).

## Flujo

1. `main.go:209` → `RequireAuth` (`auth_middleware.go:91`).
2. `NoteHandler.Delete` (`note_handler.go:357`): `validateNoteID`.
3. `NoteService.Delete` (`note_service.go:1367`): `GetByID` y verificación de autor.
4. `PGNoteStore.DeleteWithDriveCleanup` (`service/store_pg.go:200`) → `NoteRepository.DeleteWithDriveCleanup` (transacción con outbox).
5. `SharedStore.DeleteManagedPermissionsByNote` (`store_pg.go:170`).
6. Drive síncrono best-effort: `executeDriveOperation` → `RealDriveClient.DeleteFile` / `DeleteAttachment` (`back/notes/internal/drive/real_client.go:228,292`).
7. Respuesta 204.

Finalmente devuelve `204`, y la pantalla de detalle utiliza ese resultado para borrar la nota de Drift, cerrar el detalle y refrescar la lista.

## Tests

| Archivo:línea | Prueba | Qué comprueba | Tipo |
| --- | --- | --- | --- |
| `back/notes/internal/handler/http/note_handler_test.go:1098` | `TestHandlerDeleteNoteAndAllFiles` | 204 y borrado de `.md` y adjuntos en Drive mock | Unitaria |
| `back/notes/internal/service/note_service_test.go:251` | `TestDeleteOnlyAuthor` | Solo el autor | Unitaria |
| `note_service_test.go:1043` | `TestDriveDeleteCascadesDriveAndPostgres` | Borra en ambos lados | Unitaria |
| `service/delete_cleanup_test.go:41,150` | `TestDeleteNoteAllDriveFiles`, `TestDeleteCleanupToleratesMissingDriveFilesAndNetworkFailures` | Limpieza completa y tolerancia a fallos de Drive | Unitaria |
| `service/rollback_test.go:271,303,327` | `TestDeletePGFailurePreservesDriveFile`, `TestDeleteDriveFailureDoesNotBlockPGDelete`, `TestDeleteIdempotentSecondCallNotFound` | Orden PG→Drive y 404 en el segundo borrado | Unitaria |
| `service/reconciliation_test.go:48` | `TestReconcileOutboxRetriesDeleteAndCompletes` | El reconciliador drena la outbox | Unitaria |
| `back/notes/internal/repository/drive_outbox_integration_test.go:78,166,307` | `TestDriveCleanupTransactions`, `TestDeleteNoteCleansRestrictiveForeignKeysAtomically`, `TestDeleteNoteQueuesAllAttachmentsBeforeCascade` | Transacción y outbox en Postgres real | Integración (`NOTES_TEST_DATABASE_URL`) |
| `repository/simple_protocol_delete_test.go:89` | `TestSimpleProtocolDeleteNoteWithAttachments` | Borrado con adjuntos en modo simple protocol | Integración |
| `back/notes/internal/handler/grpc/server_test.go:313,330` | `TestDeleteNoteRemovesIt`, `TestDeleteNoteByStrangerIsNotFound` | Equivalente gRPC (bufconn) | Unitaria gRPC |
| `front/test/notes_delete_flow_test.dart:31` | `borrado Notes, Cancelar/Guardar y error=$failure` | Diálogo de borrado y errores | Widget (HTTP falso) |
| `front/test/note_deletion_test.dart:23` | `delete status=$status closedCache=$closedCache refreshes list` | Respuesta 200/204/404 refresca la lista | Widget |
| `front/test/notes_phase1_idempotency_tombstones_test.dart:317` | `DELETE offline persiste tombstone…` | Tombstone offline y reintento | Widget |
