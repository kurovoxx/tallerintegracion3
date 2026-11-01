# POST /notes/{id}/attachments

Adjunta una imagen (JPEG/PNG) o PDF a una nota propia. Acepta dos formas: **multipart** (el backend sube el binario al Drive del autor) o **JSON** con un `external_file_id` ya subido (normalmente con [POST /notes/upload](POST-notes-upload.md)). Ruta interna del backend Notes (`:8082`). Desde el navegador: `POST https://ti3-brojas.dev.censei.cl/api-notes/notes/<id>/attachments` ([prefijos](../COMUN-notes.md#1-rutas-internas-y-prefijos-externos)). No tiene equivalente gRPC.

## Req

- **Registro:** `back/notes/cmd/server/main.go:200` → `protected.POST("/notes/:id/attachments", h.UploadAttachment)`. Handler `back/notes/internal/handler/http/note_handler.go:415-528` (`UploadAttachment`).
- **Autenticación:** `RequireAuth` (`main.go:192`). Solo el autor de la nota (otro usuario → 403) (`back/notes/internal/service/note_service.go:1445-1447`, `L1565-1567`). Ver [COMUN §2](../COMUN-notes.md#2-autenticación-http).
- **Path:** `id` UUID de la nota (no UUID → 404 `not_found`).
- La rama se elige por `Content-Type`: si contiene `application/json` → JSON (`note_handler.go:428-479`); si no → multipart (`L481-527`).

**Variante multipart** (límite del body 11 MiB, `L482`):

| Campo | Tipo | Obligatorio | Validación |
| --- | --- | --- | --- |
| `file` | archivo | sí | No vacío (`L505-508`); máx. 10 MiB (`L500-504`). El tipo real se detecta con los primeros 512 bytes y debe ser `image/jpeg`, `image/png` o `application/pdf`; si el `Content-Type` de la parte es específico y no coincide → 400 (`L515-521`; `back/notes/internal/drive/mime.go:125-135`) |
| `is_inline` | `"true"` | no | Query `?is_inline=true` o campo de formulario (`L510`) |

**Variante JSON** (límite 1 MiB, `L438`):

| Campo | Tipo | Obligatorio | Validación |
| --- | --- | --- | --- |
| `external_file_id` | string | sí | No vacío (`L447-450`); el archivo debe existir y ser accesible con el token del autor (`note_service.go:1569-1583`) |
| `file_name` | string | no | Se usa para inferir el MIME |
| `file_type` | string | no | Por defecto `application/octet-stream`; el tipo declarado (o resuelto por extensión) debe estar en la whitelist (`note_handler.go:456-470`) |
| `file_size_bytes` | entero | no | Se guarda tal cual, sin verificar |
| `is_inline` | bool | no | Por defecto `false` |

## Res

**201 Created** (ambas variantes, `note_handler.go:476` y `L527`)

```json
{"attachment_id":"33333333-3333-4333-8333-333333333333","is_inline":true,"file_url":"https://drive.google.com/file/d/1XyZ/view","external_file_id":"1XyZ"}
```

- `attachment_id`: UUID de la fila en `notes.note_attachments`.
- `is_inline`: si se muestra incrustado en el Markdown.
- `file_url`: enlace web de Drive. En la variante JSON se construye como `https://drive.google.com/file/d/<id>/view` (`note_service.go:1590`); en multipart es el que devuelve el cliente Drive.
- `external_file_id`: ID del archivo en Drive.

**Errores**

| HTTP | `error.code` | Condición |
| --- | --- | --- |
| 400 | `bad_request` | Multipart sin campo `file` (`L491`), archivo vacío (`L506`), MIME declarado distinto del detectado (`L407-408`); JSON inválido o sin `external_file_id` (`L443`, `L448`) |
| 403 | `forbidden` "Acceso denegado" | La nota es de otro usuario |
| 403 | `forbidden` "Conecte o renueve su Google Drive" | OAuth del autor ausente/revocado (`note_service.go:1470-1472`, `L1576-1578`) |
| 404 | `not_found` "nota no encontrada" | `id` no UUID o nota inexistente |
| 404 | `not_found` "archivo externo no encontrado o sin permisos en Drive" | JSON: Drive responde 404/403 al verificar (`L1579-1581`) |
| 413 | `file_too_large` "Archivo muy grande" | Body > 11 MiB (multipart) o > 1 MiB (JSON), archivo > 10 MiB, o Drive 413 |
| 415 | `unsupported_media_type` "Tipo de archivo no permitido: solo image/jpeg, image/png o application/pdf" | Tipo real (multipart) o declarado (JSON) fuera de la whitelist (`L405-406`, `L467-469`) |
| 500 | `internal_error` | Error leyendo el archivo (`L495-499`) o fallo de Postgres |
| 502 | `internal_error` "Servicio de almacenamiento no disponible" | Drive caído/timeout |
| 401 / 423 / 503 | ver [COMUN §2](../COMUN-notes.md#2-autenticación-http) | Middleware |

**Efectos antes de responder**

- Multipart: sube el binario a la carpeta `Apuntes TI3` del Drive del autor (`UploadAttachment`, `back/notes/internal/drive/real_client.go:242-290`) y luego inserta la fila en `notes.note_attachments` (`note_service.go:1461-1480`). Si el INSERT falla, borra el archivo subido; si eso también falla, encola `delete_attachment` (`L1481-1492`).
- JSON: verifica el archivo en Drive (`VerifyFileAccess`, `real_client.go:351`) e inserta la fila (`note_service.go:1569-1595`). No sube nada.
- No crea permisos de Drive para el adjunto.
- Nada queda en segundo plano en el camino de éxito.

## Invocación (Front / Otros Servicios)

- **Multipart** — `_uploadAttachmentBinary` (`front/lib/features/notes/all_notes_screen.dart:5471`): `http.MultipartRequest('POST', '$notesBaseUrl/notes/<id>/attachments')` con `Authorization: Bearer ${SessionManager.token}`, campo `is_inline` y archivo `file` (`L5505-5513`), timeout 30 s. Se llama desde `_uploadPendingAttachmentsToNote` (`L1082-1132`, llamada en `L1110`) cuando una nota con adjuntos pendientes (`attachment-pending://…`) ya tiene UUID: tras crearla (`L754`, `L790`, `L2709`) o al pulsar **REFRESCAR / SINCRONIZAR DRIVE** (`L953`). Con 201 guarda el `attachment_id` en Drift (`recordAttachmentUpload`, `L1117`; `front/lib/core/database/local_notes_repository.dart:372`) y reescribe la referencia del Markdown a `attachment:<id>` (`L1119-1122`).
- **JSON (vincular lo ya subido a Drive)**:
  - Al crear la nota: `_linkAttachmentsToNote` (`L1556-1622`, petición en `L1571-1587`), llamado en `L2718`.
  - Al editar: barra de adjuntos del editor → subida con `_uploadAttachmentToDrive` (`POST /notes/upload`, `L5348`) → `_onAttachmentUploadedInEdit` (`L4241-4300`), que envía `{external_file_id, file_name, file_type, file_size_bytes, is_inline}` con `_notesAuthHeaders()` (`L4256-4272`; `L5265`). Con 201 añade el recurso a la lista y cambia la URL de Drive por `attachment:<id>` en el texto.
- **Otros microservicios:** No se encontró invocación en el código revisado.
- **Tests/scripts:** `scripts/test-notes-e2e.py:116-117,124`.

## Flujo

1. `main.go:200` → `RequireAuth` (`auth_middleware.go:91`).
2. `NoteHandler.UploadAttachment` (`note_handler.go:415`): valida id, elige rama por `Content-Type`.
3. Multipart: límite 11 MiB → `FormFile("file")` → tamaño → `drive.ValidateAttachmentMime` (`mime.go:125`) → `NoteService.AddAttachment` (`note_service.go:1434`): `GetByID`, autor, `DetectMimeType`, Drive `UploadAttachment` (síncrono, token del autor) → `PGAttachmentStore.Create` (`service/store_pg.go:58` → `repository/attachment_repository.go:21`).
4. JSON: bind → whitelist por tipo declarado/extensión → `NoteService.AddAttachmentExternal` (`note_service.go:1551`): autor, Drive `VerifyFileAccess`, `Create`.
5. Respuesta 201.

Finalmente devuelve `201 {attachment_id, is_inline, file_url, external_file_id}`, y la pantalla de notas utiliza ese resultado para sustituir en el Markdown la referencia temporal o la URL de Drive por `attachment:<id>` y registrar el adjunto en Drift.

## Tests

| Archivo:línea | Prueba | Qué comprueba | Tipo |
| --- | --- | --- | --- |
| `back/notes/internal/handler/http/note_handler_test.go:344` | `TestHandlerAttachment` | Multipart → 201 | Unitaria (Drive mock) |
| `note_handler_test.go:516` | `TestHandlerExternalAttachment` | Variante JSON | Unitaria |
| `note_handler_upload_test.go:63` | `TestHandlerUploadPdfThenLink` | `/notes/upload` + vincular por JSON | Unitaria |
| `note_handler_upload_test.go:165,190` | `TestHandlerAttachmentPreservesDeclaredMime`, `TestHandlerAttachmentPersistsNoteIDAndDriveURL` | MIME y URL guardados | Unitaria |
| `note_handler_upload_test.go:225,245,263,299` | `TestHandlerAttachmentSniffingWinsOverExtension`, `TestHandlerAttachmentRejectsMimeMismatch`, `TestHandlerAttachmentRejectsUnsupportedContent`, `TestHandlerExternalAttachmentRejectsUnsupportedType` | Sniffing, 400 y 415 | Unitaria |
| `audit_fixes_test.go:489` | `TestHandlerAttachmentBoundary10MB` | Límite de 10 MiB | Unitaria |
| `back/notes/internal/service/note_service_test.go:303,339` | `TestAttachmentUploadAndDelete`, `TestAttachmentTooLarge` | Servicio | Unitaria |
| `service/rollback_test.go:74` | `TestAttachmentRollbackOnDBFailureNoOrphan` | Compensación si falla el INSERT | Unitaria |
| `back/notes/internal/drive/mime_test.go:94,111` | `TestIsAllowedAttachmentMimeType`, `TestValidateAttachmentMime` | Whitelist y validación | Unitaria |
| `front/test/notes_manual_sync_test.dart:197` | `3. attachment pendiente se procesa (multipart + PATCH)` | Subida multipart desde REFRESCAR | Widget (HTTP falso) |
| `front/test/notes_create_sync_test.dart:249` | `8/9/10/12/14. click durante sync con imagen: espera y muestra` | Creación con imagen | Widget |
