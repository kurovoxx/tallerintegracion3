# POST /auth/logout

## Req
- Método: `POST`, ruta: `/auth/logout` (pública a nivel de router, sin middleware; `back/auth/cmd/server/main.go:92`).
- Headers: `Content-Type: application/json`.
- Body: `{"refresh_token":"<token-opaco>"}`.

## Res
- `204`: sin body (revoca el refresh; idempotente: token ya revocado, expirado o inexistente también da 204).
- `400`: `{"error":{"code":"bad_request","message":"..."}}` (JSON malformado o `refresh_token` vacío).

## Flujo
- `back/auth/internal/handler/http/auth_handler.go` (`Logout`) → `back/auth/internal/service/auth_service.go` (`Logout`) → `back/auth/internal/repository/refresh_token_repository.go`.

## Tests
- `back/auth/internal/handler/http/e2e_full_flow_test.go`: `TestE2E_RegisterLoginMeRefreshLogout` (logout 204 sin body, logout repetido 204, refresh posterior 401).
- `back/auth/internal/service/auth_integration_test.go`: `TestAuthIntegration_RegisterLoginRefreshLogout` (logout + idempotencia a nivel service contra Postgres real).
