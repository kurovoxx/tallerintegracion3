# GET /notes/:id

Microservicio `notes`; ruta del router de producción.

## Req

- JWT HS256 en `Authorization: Bearer <access_token>`; identidad extraída del token.
- Permiso: Autor, pública, link o miembro de grupo restricted.
- Entrada: Path id UUID.

## Res

- 200 metadata + content? + attachments:[].
- 404 inexistente/privada sin acceso/Drive ausente; 403 OAuth; 502 Drive; 500 DB.
- `401` por token ausente/inválido/expirado, antes del handler. Errores con forma `{"error":{"code":"not_found","message":"Nota no encontrada"}}`; ver [catálogo común](../documentation/docHector.md#autenticación-y-errores).

Ejemplo de respuesta (IDs y fechas ilustrativos, campos del contrato real):

```json
{"id":"11111111-1111-4111-8111-111111111111","user_id":"22222222-2222-4222-8222-222222222222","title":"Álgebra","visibility":"private","likes_count":0,"version":1,"created_at":"2026-10-22T12:00:00Z","updated_at":"2026-10-22T12:00:00Z","external_file_id":"drive-file-id","drive_url":"https://drive.google.com/file/d/drive-file-id/view","content":"# Matrices\n","attachments":[]}
```

## Flujo

`Get` → `Get + ListAttachments`. Lee metadata en PostgreSQL y contenido desde Drive del autor tras autorizar en la aplicación. subject_id, external_file_id, drive_url, forked_from_note_id y content son condicionales. attachments siempre es una lista. Una nota privada inaccesible es indistinguible de una inexistente (404).

Fuentes: `back/notes/cmd/server/main.go`, `internal/handler/http/` y `internal/service/` dentro de `back/notes`.

## Tests

- Handler/router: `TestHandlerGetWithDriveResilienceAlternative; TestHandlerPrivateAccessNotFound`.
- Dominio: `TestGetWithContent; TestZeroKnowledgePrivateAndMissingIndistinguishable`.
- Alcance de integración y E2E: [matriz de cobertura](../documentation/notes_api_matrix.md).
