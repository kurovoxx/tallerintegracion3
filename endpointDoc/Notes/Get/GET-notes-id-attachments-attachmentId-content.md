# GET /notes/:id/attachments/:attachmentId/content

Microservicio `notes`; ruta del router de producción.

## Req

- JWT HS256 en `Authorization: Bearer <access_token>`; identidad extraída del token.
- Permiso: Acceso de lectura a nota y pertenencia del adjunto a ella.
- Entrada: Path id y attachmentId UUID.

## Res

- 200 bytes con Content-Type.
- 404 nota/adjunto ausente o sin acceso; 403 OAuth; 502 Drive; 500 DB.
- `401` por token ausente/inválido/expirado, antes del handler. Errores con forma `{"error":{"code":"not_found","message":"Nota no encontrada"}}`; ver [catálogo común](../documentation/docHector.md#autenticación-y-errores).

## Flujo

`AttachmentContent` → `GetAttachmentContent`. El backend descarga el binario con credenciales del autor después de autorizar al lector. Content-Disposition inline para tipos visualizables (incluido PDF), attachment para otros; filename se escapa con mime.FormatMediaType. Siempre Cache-Control: private, no-store y X-Content-Type-Options: nosniff. La respuesta exitosa no es JSON ni HTML de Drive.

Fuentes: `back/notes/cmd/server/main.go`, `internal/handler/http/` y `internal/service/` dentro de `back/notes`.

## Tests

- Handler/router: `TestAttachmentContentAuthorizationAndMIME`.
- Dominio: `TestAttachmentContentSharePolicy`.
- Alcance de integración y E2E: [matriz de cobertura](../documentation/notes_api_matrix.md).
