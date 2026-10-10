# POST /auth/refresh

## Req
- Método: `POST`, ruta: `/auth/refresh` (pública, sin auth; `back/auth/cmd/server/main.go:91`).
- Headers: `Content-Type: application/json`.
- Body: `{"refresh_token":"<token-opaco>"}`.

## Res
- `200`: `{"access_token":"<jwt-nuevo>","refresh_token":"<token-rotado>","expires_in":900}` (rotación: el token usado queda revocado).
- `400`: `{"error":{"code":"bad_request","message":"Request inválido: refresh_token requerido"}}` (JSON malformado / campo ausente).
- `401`: `{"error":{"code":"invalid_token|token_expired|unauthorized","message":"..."}}` (expirado, reutilizado tras rotación, revocado por logout o inexistente).

## Flujo
- `back/auth/internal/handler/http/auth_handler.go` (`Refresh`) → `back/auth/internal/service/auth_service.go` (`Refresh`, valida hash, expiración y revocación, rota) → `back/auth/internal/repository/refresh_token_repository.go`.

## Tests
- `back/auth/internal/handler/http/auth_extra_coverage_test.go`: `TestCoverage_Refresh_JSONMalformado_400`, `TestCoverage_Refresh_Expirado_401`, `TestCoverage_Refresh_Rotado_Reutilizado_401`.
- `back/auth/internal/handler/http/e2e_full_flow_test.go`: `TestE2E_RegisterLoginMeRefreshLogout` (refresh 200 con rotación, reuso 401, refresh tras logout 401).
- `back/auth/internal/service/auth_integration_test.go`: `TestAuthIntegration_RegisterLoginRefreshLogout` (refresh + reuso + post-logout contra Postgres real).
