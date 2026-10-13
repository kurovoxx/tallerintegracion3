# DELETE /contents/{id}

## Req
- Método: `DELETE`, ruta: `/contents/:id` (protegida, `back/infacadem/cmd/server/main.go:178`).
- Headers: `Authorization: Bearer <access_token>`. Sin body.

## Res
- `204`: sin body.
- `400/401/404`: `invalid_uuid` / auth / ajeno-inexistente.

## Flujo
- `back/infacadem/internal/handler/http/contents_handler.go` (`Delete`) → `service` → `repository` (`Delete`, `DELETE ... WHERE EXISTS` propiedad).

## Tests
- `back/infacadem/internal/handler/http/content_handler_test.go`, integración (cascada).
