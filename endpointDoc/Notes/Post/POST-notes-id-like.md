# POST /notes/:id/like

Microservicio `notes`; ruta del router de producción.

## Req

- JWT HS256 en `Authorization: Bearer <access_token>`; identidad extraída del token.
- Permiso: Lectura autorizada.
- Entrada: Path id UUID; sin body.

## Res

- 201 sin cuerpo.
- 404 sin acceso/ausente; 409 already_liked; 500 DB.
- `401` por token ausente/inválido/expirado, antes del handler. Errores con forma `{"error":{"code":"not_found","message":"Nota no encontrada"}}`; ver [catálogo común](../documentation/docHector.md#autenticación-y-errores).

## Flujo

`Like` → `Like`. Registra like y aumenta likes_count atómicamente en PostgreSQL. Autoriza antes de comprobar duplicados para no revelar existencia ni likes previos de una nota privada.

Fuentes: `back/notes/cmd/server/main.go`, `internal/handler/http/` y `internal/service/` dentro de `back/notes`.

## Tests

- Handler/router: `TestHandlerLikeAndCopy; TestConcurrentLikes`.
- Dominio: `TestLikeReadAccess`.
- Alcance de integración y E2E: [matriz de cobertura](../documentation/notes_api_matrix.md).
