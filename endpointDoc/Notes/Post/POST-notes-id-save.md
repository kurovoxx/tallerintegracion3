# POST /notes/:id/save

Microservicio `notes`; ruta del router de producción.

## Req

- JWT HS256 en `Authorization: Bearer <access_token>`; identidad extraída del token.
- Permiso: Lectura autorizada.
- Entrada: Path id UUID; sin body.

## Res

- 201 sin cuerpo.
- 404 sin acceso/ausente; 409 already_saved; 500 DB.
- `401` por token ausente/inválido/expirado, antes del handler. Errores con forma `{"error":{"code":"not_found","message":"Nota no encontrada"}}`; ver [catálogo común](../documentation/docHector.md#autenticación-y-errores).

## Flujo

`Save` → `Save`. Crea un bookmark en notes.saved_notes. No clona ni otorga permisos Drive. La duplicación se comprueba antes y después de la inserción concurrente. Unsave existe como método interno, pero no hay ruta HTTP DELETE save registrada.

Fuentes: `back/notes/cmd/server/main.go`, `internal/handler/http/` y `internal/service/` dentro de `back/notes`.

## Tests

- Handler/router: `TestHandlerSaveConflict`.
- Dominio: `TestSaveAndUnsave`.
- Alcance de integración y E2E: [matriz de cobertura](../documentation/notes_api_matrix.md).
