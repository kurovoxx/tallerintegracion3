# POST /notes/{id}/copy

Clona una nota legible en el Drive del usuario que copia: crea una nota nueva, privada, con `forked_from_note_id` apuntando al original y un `.md` propio en su carpeta `Apuntes TI3`. Ruta interna del backend Notes (`:8082`). Desde el navegador: `POST https://ti3-brojas.dev.censei.cl/api-notes/notes/<id>/copy` ([prefijos](../COMUN-notes.md#1-rutas-internas-y-prefijos-externos)). No tiene equivalente gRPC.

## Req

- **Registro:** `back/notes/cmd/server/main.go:204` → `protected.POST("/notes/:id/copy", h.Copy)`. Handler `back/notes/internal/handler/http/note_handler.go:640-671` (`Copy`).
- **Autenticación:** `RequireAuth` (`main.go:192`). Ver [COMUN §2](../COMUN-notes.md#2-autenticación-http).
- **Permiso:** debe poder leer el original (`hasReadAccess`, `back/notes/internal/service/note_service.go:1722-1728`). El autor también puede copiar su propia nota.
- **Límite de uso (en memoria, por réplica):** máximo 10 copias por minuto por usuario y 100 por minuto en total; ventana deslizante de 1 min (`note_handler.go:20-122`, aplicado en `L655-663`) antes de llamar al servicio.
- **Path:** `id` UUID del original (no UUID → 404 `not_found`).
- **Header opcional:** `X-Idempotency-Key` (`L664`). Misma clave y mismo original → devuelve el mismo clon; misma clave con otro original → 409 (`note_service.go:1742-1770`).
- **Body:** no tiene.

## Res

**201 Created** (`note_handler.go:670`)

```json
{
  "note_id": "55555555-5555-4555-8555-555555555555",
  "forked_from_note_id": "11111111-1111-4111-8111-111111111111",
  "user_id": "33333333-3333-4333-8333-333333333333",
  "title": "Álgebra",
  "visibility": "private",
  "sync_status": "synced"
}
```

- `note_id`: UUID del clon.
- `forked_from_note_id`: el `id` del path (el original).
- `user_id`: dueño del clon = usuario del JWT (nunca el autor original).
- `title`, `visibility` (`private` siempre), `sync_status`: metadata del clon para que el cliente lo pinte sin re-sincronizar. Todos los valores son strings; el clon nace siempre con `version` 1.

**Caso "copiar desde Guardados" o desde la vista previa de un perfil:** el flujo es el mismo para un apunte guardado o público de otro usuario. El contenido Markdown se duplica con `drive.Client.CopyFile` en el Drive del solicitante y se crea una fila nueva en `notes.notes` con `user_id` = solicitante. **Los adjuntos (imágenes y PDF) también se copian** al Drive del solicitante con `drive.Client.CopyAttachment` y las referencias `attachment:<uuid>` del Markdown clonado se reescriben a los adjuntos nuevos (`service/copy_attachments.go`): la copia no depende de los archivos del autor.

**Una copia por apunte:** un usuario solo puede tener un clon vivo de cada original (índice único parcial `uq_notes_user_forked_from`, migración 006). Una segunda copia responde `409 already_copied`; si el clon anterior quedó `failed_sync`, el nuevo intento lo reutiliza y retoma la copia.

**Errores**

| HTTP | `error.code` | Condición |
| --- | --- | --- |
| 403 | `forbidden` "Conecte o renueve su Google Drive" | Quien copia no tiene Drive conectado (`note_service.go:1833-1836`) |
| 404 | `not_found` "nota no encontrada" | Original inexistente, privado sin acceso, sin `external_file_id`, o su archivo no se puede leer en Drive (404/403). Todos indistinguibles (`L1719-1731`, `L1840-1843`) |
| 410 | `note_deleted` "Apunte no disponible (eliminado por el autor)" | El autor borró el original y el solicitante lo tenía guardado o era público (lápida en `notes.note_tombstones`, `missingNoteError`). No se crea fila ni archivo |
| 409 | `conflict` | Idempotencia: otra solicitud con la misma clave en curso o con otro original |
| 409 | `already_copied` "Ya copiaste este apunte a tus apuntes" | El usuario ya tiene un clon vivo (no `failed_sync`) de ese original, o perdió la carrera contra otra copia concurrente (`uq_notes_user_forked_from`) |
| 429 | `rate_limited` "Límite de clonación excedido, intente más tarde" | Más de 10 copias/min del usuario (`note_handler.go:660`) |
| 429 | `rate_limited` "Límite global de clonación excedido, intente más tarde" | Más de 100 copias/min en total (`L658`) |
| 500 | `internal_error` | Fallo de Postgres |
| 502 | `internal_error` "Servicio de almacenamiento no disponible" | Drive caído/timeout, o el clon recibió el mismo `fileID` que el original (`note_service.go:1844-1845`, `L1850-1861`) |
| 401 / 423 / 503 | ver [COMUN §2](../COMUN-notes.md#2-autenticación-http) | Middleware |

**Efectos antes de responder** (síncronos):

1. Clave de idempotencia (si viene) con el id del clon preasignado.
2. Fila en `notes.notes` del usuario que copia: mismo `title`, `visibility='private'`, `subject_id` **siempre nulo** (no se hereda), `forked_from_note_id=<original>`, `sync_status='pending_drive'` (`L1776-1792`).
3. Drive (`CopyFile`, `back/notes/internal/drive/real_client.go:296-335`): descarga el original con el token de **quien copia** (sirve si el archivo es público o le dieron permiso); si falla, reintenta con el token del autor; luego crea el archivo en la carpeta `Apuntes TI3` de quien copia.
4. Adjuntos: cada fila de `notes.note_attachments` del original se clona en el Drive de quien copia y se registra con `copied_from_attachment_id` (idempotente por `uq_note_attachments_copy_source`); luego se reescriben las referencias del Markdown clonado.
5. `external_file_id` y `synced` en el clon.

No crea permisos de Drive (el clon es privado). No copia shares.

**Fallos:** si Drive falla (también al copiar un adjunto), el clon queda `failed_sync`; con clave de idempotencia queda `recoverable` para reintentar sin duplicar, y el reintento reutiliza los adjuntos ya copiados. Si falla Postgres tras crear el archivo, se borra el archivo o se encola `delete_file` (`L1865-1885`).

## Invocación (Front / Otros Servicios)

- **Pantalla:** detalle de nota (`_NoteDetailSheet`, `front/lib/features/notes/all_notes_screen.dart:3802`). Botón **Guardar copia**, visible solo si la nota **no** es propia (`L4938-4944`) → `_confirmClone` (`L4172`) → `widget.onClone` (`L4174`) → `_cloneFlow` (`L1966-2000`).
- **Petición:** `POST $notesBaseUrl/notes/<id>/copy` (`L1967`) con `_authHeaders()` (Bearer `SessionManager.token`), **sin** `X-Idempotency-Key`, timeout 8 s, vía `AuthedHttp.run`.
- **Respuesta:** con 201 añade `note_id` a `_ownedNoteIds` y lanza `_syncFromBackend()` para traer el clon con `GET /notes/me` y guardarlo en Drift (`L1974-1988`). Muestra "Sin conexión: no se pudo guardar la copia." o "No se pudo guardar la copia. Inténtalo más tarde." en error (`L4167-4170`).
- `AllNotesScreen` lista las notas de `GET /notes/me` (todas propias), así que en la práctica el botón solo aparece para notas que la pantalla no reconoce como propias (`_isMine`, `L1770-1773`).
- **Vista previa de un apunte de otro perfil:** `PublicNotePreviewScreen` (`front/lib/features/notes/public_note_preview_screen.dart`) → botón **Copiar a mis apuntes** → `NotesApi.copyNote` con una `X-Idempotency-Key` estable por apertura; `already_copied` deja el apunte como copiado.
- **Otros microservicios:** No se encontró invocación en el código revisado.
- **Tests/scripts:** `scripts/test-notes-e2e.py:137-140` (copia la propia nota con clave de idempotencia).

## Flujo

1. `main.go:204` → `RequireAuth` (`auth_middleware.go:91`).
2. `NoteHandler.Copy` (`note_handler.go:640`): `validateNoteID`, `copyLimiter.allow`, lee `X-Idempotency-Key`.
3. `NoteService.Copy` (`note_service.go:1707`): `GetByID` original, `hasReadAccess`, idempotencia, `PGNoteStore.Create` (`service/store_pg.go:22`).
4. Drive síncrono: `CopyFile` con tokens pedidos a Identity (`drive/oauth_grpc.go:62`).
5. `UpdateExternalFileID` (`repository/note_repository.go:208`) → relectura → respuesta 201.

Finalmente devuelve `201 {note_id, forked_from_note_id}`, y la pantalla utiliza ese resultado para marcar el clon como propio y refrescar la lista desde `GET /notes/me`.

## Tests

| Archivo:línea | Prueba | Qué comprueba | Tipo |
| --- | --- | --- | --- |
| `back/notes/internal/handler/http/note_handler_test.go:254` | `TestHandlerLikeAndCopy` | Copia por HTTP → 201 | Unitaria (Drive mock) |
| `note_handler_spec95_test.go:199` | `TestHandlerCopyIdempotencyReplayAndConflict` | Replay y 409 | Unitaria |
| `note_handler_ratelimit_test.go:49,95,145,175` | `TestHandlerCopyRateLimitReturns429`, `TestCopyRateLimiterConcurrencyAndWindowReset`, `TestCopyRateLimiterGlobalCap`, `TestHandlerCopyGlobalRateLimitReturns429` | Límite por usuario y global | Unitaria |
| `audit_fixes_test.go:198,235,251,328,444` | `TestHandlerCopySourceDownloadErrorCodes`, `TestHandlerCopyDecoupledFromAuthorOAuth`, `TestHandlerCopyDstWithoutOAuthRejectedViaDisconnect`, `TestHandlerCopyInverseOrphanCompensation`, `TestHandlerCopyErrorDistinction` | Códigos de error y OAuth de origen/destino | Unitaria |
| `back/notes/internal/service/note_service_test.go:375,409,1084,1110` | `TestCopyClonesFile`, `TestCopyPrivateWithoutAccessNotFound`, `TestDriveCopyCreatesIndependentFile`, `TestDriveCopyMissingSourceMapsNotFound` | Reglas de servicio | Unitaria |
| `service/rollback_test.go:26,351,396,422` | `TestCopyRollbackOnDBFailureNoOrphan`, `TestCopyDstWithoutOAuthRejectedNoRecord`, `TestCopyDecoupledAuthorWithoutOAuthSucceeds`, `TestCopySourceMissingNoRecordNoOrphan` | Compensaciones y OAuth | Unitaria |
| `service/rollback_test.go:648,678,706,810` | `TestIdempotencyCopy*` | Idempotencia y recuperación | Unitaria |
| `service/idempotency_recoverable_test.go:193` | `TestIdempotencyCopyRecoverableImmediateRetryResumesClone` | Reintento inmediato | Unitaria |
| `service/remediation_v3_test.go:233` | `TestCopySourceAccessDeniedDistinctFromDstOAuth` | 404 de origen vs 403 de destino | Unitaria |
| `back/notes/internal/drive/real_client_test.go:332,379,403` | `TestRealCopyFileFallback*`, `TestRealCopyFileDstNoOAuthNoFallback` | Descarga con fallback al token del autor | Unitaria (servidor Drive falso) |

| `back/notes/internal/service/copy_integrity_test.go` | Copia de adjuntos, reescritura de referencias, segunda copia `already_copied`, reintento de `failed_sync` | Unitaria |
| `back/notes/internal/handler/http/note_handler_copy_integrity_test.go` | 409 `already_copied` por HTTP | Unitaria |
| `front/test/public_note_preview_test.dart` | Copia desde la vista previa, clave estable y `already_copied` | Widget |

No hay prueba de widget en el front para "Guardar copia" de `AllNotesScreen`.
