# DELETE /notes/{id}/attachments/{attachmentId}

Quita un adjunto de una nota propia: borra la fila en Postgres (dejando encolada la limpieza en Drive) y después intenta borrar el archivo en Drive. Ruta interna del backend Notes (`:8082`). Desde el navegador: `DELETE https://ti3-brojas.dev.censei.cl/api-notes/notes/<id>/attachments/<attachmentId>` ([prefijos](../COMUN-notes.md#1-rutas-internas-y-prefijos-externos)). No tiene equivalente gRPC.

## Req

- **Registro:** `back/notes/cmd/server/main.go:202` → `protected.DELETE("/notes/:id/attachments/:attachmentId", h.DeleteAttachment)`. Handler `back/notes/internal/handler/http/note_handler.go:594-617` (`DeleteAttachment`).
- **Autenticación:** `RequireAuth` (`main.go:192`). Solo el autor de la nota (otro usuario → 403) (`back/notes/internal/service/note_service.go:1508-1510`). Ver [COMUN §2](../COMUN-notes.md#2-autenticación-http).
- **Path:** `id` (UUID de la nota) y `attachmentId` (UUID del adjunto). Cualquiera no UUID → 404 (`note_handler.go:602-611`).
- **Query / body:** no tiene.

## Res

**204 No Content**, sin body (`note_handler.go:616`).

**Errores**

| HTTP | `error.code` | Condición |
| --- | --- | --- |
| 403 | `forbidden` "Acceso denegado" | La nota es de otro usuario (`note_service.go:1508-1510`, `L1526-1528`) |
| 404 | `not_found` "nota no encontrada" | `id` no UUID o nota inexistente (`L1505-1507`) |
| 404 | `not_found` "adjunto no encontrado" | `attachmentId` no UUID, el adjunto no existe o pertenece a otra nota (`L1515-1517`, `L1523-1525`) |
| 500 | `internal_error` | Fallo de Postgres (`L1529-1530`) |
| 401 / 423 / 503 | ver [COMUN §2](../COMUN-notes.md#2-autenticación-http) | Middleware |

Un fallo de Drive no produce error: la respuesta sigue siendo 204 (`L1540-1543`).

**Efectos antes de responder**

1. Transacción (`AttachmentRepository.DeleteAttachmentWithDriveCleanup`, `back/notes/internal/repository/attachment_repository.go:91-134`): bloquea la nota y verifica dueño, bloquea el adjunto, encola `delete_attachment` en `notes.drive_reconciliation_queue` si tiene archivo, borra la fila de `notes.note_attachments` y hace commit.
2. Drive, síncrono y best-effort: `DeleteAttachment` con el token del autor, 15 s y 3 intentos (`note_service.go:1535-1543`; `back/notes/internal/drive/real_client.go:292`).

No toca el contenido Markdown de la nota: si el texto aún referencia `attachment:<id>`, debe limpiarlo el cliente con un PATCH.

**Segundo plano:** el reconciliador (cada 30 s) procesa la fila encolada (`reconcileDriveOperations`, `note_service.go:2913`). Si el borrado síncrono ya funcionó, Drive responde 404 y cuenta como éxito.

## Invocación (Front / Otros Servicios)

- **Pantalla:** detalle de nota propia en modo edición (`_NoteDetailSheet`, `front/lib/features/notes/all_notes_screen.dart:3802`).
- **Acción:** el botón **Quitar adjunto** (`_ResourceRemoveButton`, `L6620`, etiqueta `L6637`) solo marca el adjunto (`_pendingRemovedIds.add`, `L4348`). Al pulsar **GUARDAR** (`_confirmEdit`, `L4047`) primero se quita la referencia del Markdown y se hace el PATCH (`L4055-4068`); solo si el PATCH sale bien se borran los adjuntos marcados (`L4099-4108`).
- **Petición:** `_deleteAttachmentFromServer` (`L4355-4380`): `DELETE $notesBaseUrl/notes/<noteId>/attachments/<id>` con `_notesAuthHeaders()` (Bearer `SessionManager.token`, `L5265-5272`), timeout 30 s, vía `AuthedHttp.run`.
- **Respuesta:** 200/204 → quita el recurso de la lista; otro código → lanza error, muestra mensaje y recarga desde el servidor (`L4372-4374`; `L4119`).
- **Otros microservicios:** No se encontró invocación en el código revisado.
- **Tests/scripts:** `scripts/test-notes-e2e.py:127-128`.

## Flujo

1. `main.go:202` → `RequireAuth` (`auth_middleware.go:91`).
2. `NoteHandler.DeleteAttachment` (`note_handler.go:594`): valida `id` y `attachmentId`.
3. `NoteService.RemoveAttachment` (`note_service.go:1497`): `GetByID` nota, autor, `GetByID` adjunto (`service/store_pg.go:61`) y pertenencia a la nota.
4. `PGAttachmentStore.DeleteAttachmentWithDriveCleanup` (`store_pg.go:204`) → transacción con outbox.
5. Drive síncrono best-effort `DeleteAttachment`.
6. Respuesta 204.

Finalmente devuelve `204`, y la pantalla de detalle utiliza ese resultado para quitar el adjunto de la lista mostrada una vez guardada la edición.

## Tests

| Archivo:línea | Prueba | Qué comprueba | Tipo |
| --- | --- | --- | --- |
| `back/notes/internal/handler/http/note_handler_test.go:344` | `TestHandlerAttachment` | Sube un adjunto y lo borra por HTTP (DELETE en `L378`) | Unitaria (Drive mock) |
| `note_handler_test.go:1098` | `TestHandlerDeleteNoteAndAllFiles` | DELETE de adjunto por otro usuario → 403 y por el dueño → 204 (`L1124-1127`) | Unitaria |
| `back/notes/internal/service/note_service_test.go:303` | `TestAttachmentUploadAndDelete` | Alta y baja de adjunto | Unitaria (Drive mock) |
| `back/notes/internal/service/delete_cleanup_test.go:205` | `TestDeleteAttachmentDurableRecovery` | Limpieza durable vía outbox | Unitaria |
| `back/notes/internal/repository/simple_protocol_delete_test.go:124` | `TestSimpleProtocolDeleteSingleAttachment` | Transacción en Postgres | Integración (`NOTES_TEST_DATABASE_URL`) |
| `back/notes/internal/repository/drive_outbox_integration_test.go:78` | `TestDriveCleanupTransactions` | Encolado en la outbox | Integración |
| `back/notes/cmd/server/router_contract_test.go:15` | `TestHTTPRouterContract` | Ruta registrada; sin token → 401 | Unitaria |
| `front/test/notes_attachment_removal_test.dart:120,231` | `X individual, undo y paridad PDF en Editar`, `sin flag: X siempre visible en Editar propio` | Marcar/quitar desde la UI | Widget (HTTP falso) |
| `front/test/notes_edit_lifecycle_test.dart:236` | `borrar solo Markdown NO elimina attachment` | Editar el texto no borra el adjunto | Widget |
