# PATCH /contents/{id}

## Req
- Método: `PATCH`, ruta: `/contents/:id` (protegida, `back/infacadem/cmd/server/main.go:177`).
- Headers: `Authorization: Bearer <access_token>`, `Content-Type: application/json`.
- Body parcial: `{"title":"Nuevo","position":2}` (al menos un campo de `title/body/position`).

## Res
- `200`: recurso actualizado.
- `400`: `{"error":{"code":"bad_request|invalid_name|invalid_uuid","message":"..."}}` (body vacío, título inválido).
- `401` / `404` (ajeno/inexistente): códigos habituales.

## Flujo
- `back/infacadem/internal/handler/http/contents_handler.go` (`Update`) → `service` → `repository` (`Update` parcial con propiedad).

## Tests
- `back/infacadem/internal/handler/http/content_handler_test.go`, `back/infacadem/internal/service/content_service_test.go`.
