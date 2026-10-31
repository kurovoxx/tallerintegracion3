# POST /notes/upload

Microservicio `notes`; ruta del router de producción.

## Req

- JWT HS256 en `Authorization: Bearer <access_token>`; identidad extraída del token.
- Permiso: Usuario JWT, Drive propio.
- Entrada: Multipart file (obligatorio).

Ejemplo multipart (archivo local de prueba):

```sh
curl -X POST "$NOTES_BASE/notes/upload" -H "Authorization: Bearer $TOKEN" -F "file=@clase.pdf;type=application/pdf"
```

## Res

- 201 {external_file_id,file_url,file_name,file_type,file_size_bytes,is_inline:false}.
- 400 vacío/MIME; 413 tamaño; 415 tipo; 403 OAuth; 502 Drive.
- `401` por token ausente/inválido/expirado, antes del handler. Errores con forma `{"error":{"code":"not_found","message":"Nota no encontrada"}}`; ver [catálogo común](../documentation/docHector.md#autenticación-y-errores).

Ejemplo de respuesta (IDs y fechas ilustrativos, campos del contrato real):

```json
{"external_file_id":"drive-file-id","file_url":"https://drive.google.com/file/d/drive-file-id/view","file_name":"clase.pdf","file_type":"application/pdf","file_size_bytes":24,"is_inline":false}
```

## Flujo

`UploadFile` → `UploadToDrive`. Sube al Drive del usuario sin asociar todavía a una nota. Luego registrar con POST /notes/:id/attachments (JSON). JPEG/PNG/PDF, 10 MiB de archivo, 11 MiB de multipart. Si el cliente abandona antes de registrar, el archivo queda sin vínculo; este endpoint no es idempotente y no garantiza cleanup de un upload independiente.

Fuentes: `back/notes/cmd/server/main.go`, `internal/handler/http/` y `internal/service/` dentro de `back/notes`.

## Tests

- Handler/router: `TestHandlerUploadDirectToDrive; TestHandlerUploadPdfThenLink`.
- Dominio: `UploadToDrive ejercitado por handlers`.
- Alcance de integración y E2E: [matriz de cobertura](../documentation/notes_api_matrix.md).
