# DELETE /notes/:id/like

Microservicio `notes`; ruta del router de producción.

## Req

- JWT HS256 en `Authorization: Bearer <access_token>`; identidad extraída del token.
- Permiso: JWT; solo elimina el like propio.
- Entrada: Path id UUID; sin body.

## Res

- 204 sin cuerpo.
- 404 formato UUID inválido; 500 DB.
- `401` por token ausente/inválido/expirado, antes del handler. Errores con forma `{"error":{"code":"not_found","message":"Nota no encontrada"}}`; ver [catálogo común](../documentation/docHector.md#autenticación-y-errores).

## Flujo

`Unlike` → `Unlike`. Decrementa el contador solo si realmente se eliminó una fila del usuario y nunca por debajo de cero. La repetición sin like es 204; no exige lectura actual ni confirma existencia de la nota.

Fuentes: `back/notes/cmd/server/main.go`, `internal/handler/http/` y `internal/service/` dentro de `back/notes`.

## Tests

- Handler/router: `TestHandlerLikeAndCopy`.
- Dominio: `TestLikeUnlikeOnlyDecrementsDeletedLike`.
- Alcance de integración y E2E: [matriz de cobertura](../documentation/notes_api_matrix.md).
