# DELETE /notes/:id

Microservicio `notes`; ruta del router de producción.

## Req

- JWT HS256 en `Authorization: Bearer <access_token>`; identidad extraída del token.
- Permiso: Solo autor.
- Entrada: Path id UUID.

## Res

- 204 sin cuerpo.
- 404 ajena/ausente; 500 DB.
- `401` por token ausente/inválido/expirado, antes del handler. Errores con forma `{"error":{"code":"not_found","message":"Nota no encontrada"}}`; ver [catálogo común](../documentation/docHector.md#autenticación-y-errores).

## Flujo

`Delete` → `Delete`. Borra metadata y relaciones en PostgreSQL primero y registra cleanup durable para Markdown y adjuntos Drive. Un fallo remoto se reintenta por outbox. Una repetición puede responder 404; el frontend la trata como borrado satisfecho. Nunca usa el usuario de un body para autorizar.

Fuentes: `back/notes/cmd/server/main.go`, `internal/handler/http/` y `internal/service/` dentro de `back/notes`.

## Tests

- Handler/router: `TestHandlerDeleteNoteAndAllFiles`.
- Dominio: `TestDeleteNoteAllDriveFiles; TestDeleteCleanupToleratesMissingDriveFilesAndNetworkFailures`.
- Alcance de integración y E2E: [matriz de cobertura](../documentation/notes_api_matrix.md).
