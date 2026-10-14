# GET /contents/{id}

## Req
- Método: `GET`, ruta: `/contents/:id` (protegida, `back/infacadem/cmd/server/main.go:176`).
- Headers: `Authorization: Bearer <access_token>`. Sin body.

## Res
- `200`: `{"id":"...","subject_id":"...","title":"...","body":null,"position":0}`.
- `400/401/404`: `invalid_uuid` / auth / ajeno-inexistente.

## Flujo
- `back/infacadem/internal/handler/http/contents_handler.go` (`Get`) → `service` → `repository` (`GetByID`, `JOIN` subjects→curricula).

## Tests
- `back/infacadem/internal/handler/http/content_handler_test.go`.
