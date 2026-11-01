# POST /notes/:id/attachments

Microservicio `notes`; ruta del router de producción.

## Req

- JWT HS256 en `Authorization: Bearer <access_token>`; identidad extraída del token.
- Permiso: Solo autor.
- Entrada: Multipart file, is_inline?; o JSON external_file_id, file_name?, file_type?, file_size_bytes?, is_inline?.

```json
{"external_file_id":"drive-file-id","file_name":"clase.pdf","file_type":"application/pdf","is_inline":false}
```

Ejemplo multipart (archivo local de prueba):

```sh
curl -X POST "$NOTES_BASE/notes/$NOTE_ID/attachments" -H "Authorization: Bearer $TOKEN" -F "file=@clase.pdf;type=application/pdf"
```

## Res

- 201 {attachment_id,is_inline,file_url,external_file_id}.
- 400 vacío/MIME declarado distinto; 404 nota ajena/ausente; 413 tamaño; 415 tipo; 403 OAuth; 502 Drive; 500 DB.
- `401` por token ausente/inválido/expirado, antes del handler. Errores con forma `{"error":{"code":"not_found","message":"Nota no encontrada"}}`; ver [catálogo común](../documentation/docHector.md#autenticación-y-errores).

Ejemplo de respuesta (IDs y fechas ilustrativos, campos del contrato real):

```json
{"attachment_id":"33333333-3333-4333-8333-333333333333","is_inline":false,"file_url":"https://drive.google.com/file/d/drive-file-id/view","external_file_id":"drive-file-id"}
```

## Flujo

`UploadAttachment` → `AddAttachment / AddAttachmentExternal`. Archivo hasta 10 MiB; envoltorio multipart hasta 11 MiB. Solo JPEG, PNG o PDF por sniffing del contenido. JSON hasta 1 MiB registra un archivo existente y valida tipo declarado/extensión; no aporta binario para sniffing. Multipart sube al Drive del autor y guarda metadata; JSON enlaza el archivo previamente subido. No dispone de clave de idempotencia durable para uploads.

Fuentes: `back/notes/cmd/server/main.go`, `internal/handler/http/` y `internal/service/` dentro de `back/notes`.

## Tests

- Handler/router: `TestHandlerAttachmentBoundary10MB; TestHandlerExternalAttachment; TestHandlerAttachmentRejectsMimeMismatch`.
- Dominio: `TestAddAttachment`.
- Alcance de integración y E2E: [matriz de cobertura](../documentation/notes_api_matrix.md).
