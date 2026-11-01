# DELETE /notes/:id/attachments/:attachmentId

Microservicio `notes`; ruta del router de producción.

## Req

- JWT HS256 en `Authorization: Bearer <access_token>`; identidad extraída del token.
- Permiso: Solo autor; adjunto pertenece a nota.
- Entrada: Path id y attachmentId UUID.

## Res

- 204 sin cuerpo.
- 404 nota/adjunto/autor; 500 DB.
- `401` por token ausente/inválido/expirado, antes del handler. Errores con forma `{"error":{"code":"not_found","message":"Nota no encontrada"}}`; ver [catálogo común](../documentation/docHector.md#autenticación-y-errores).

## Flujo

`DeleteAttachment` → `RemoveAttachment`. Elimina metadata y agenda eliminación durable del archivo. La limpieza remota tolera 404 y se reintenta ante fallo transitorio. Si era inline, el servicio limpia referencias del Markdown con compensación cuando corresponde.

Fuentes: `back/notes/cmd/server/main.go`, `internal/handler/http/` y `internal/service/` dentro de `back/notes`.

## Tests

- Handler/router: `TestHandlerAttachment`.
- Dominio: `TestDeleteAttachmentDurableRecovery`.
- Alcance de integración y E2E: [matriz de cobertura](../documentation/notes_api_matrix.md).
