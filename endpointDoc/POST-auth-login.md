# POST /auth/login

## Req
- Método: `POST`, ruta: `/auth/login` (pública, sin auth; `back/auth/cmd/server/main.go:90`).
- Headers: `Content-Type: application/json`.
- Body: `{"email":"e2e@test.invalid","password":"Pass1234"}`.

## Res
- `200`: `{"access_token":"<jwt>","refresh_token":"<opaco>","expires_in":900}`.
- `400`: `{"error":{"code":"bad_request","message":"Request inválido"}}` (JSON malformado, campos faltantes).
- `401`: `{"error":{"code":"invalid_credentials","message":"..."}}` (email inexistente o clave incorrecta).

## Flujo
- `back/auth/internal/handler/http/auth_handler.go` (`Login`) → `back/auth/internal/service/auth_service.go` (`Login`, verifica bcrypt y emite JWT + refresh) → `back/auth/internal/repository/user_repository.go` + `back/auth/internal/repository/refresh_token_repository.go`.

## Tests
- `back/auth/internal/handler/http/auth_extra_coverage_test.go`: `TestCoverage_Login_JSONMalformado_400`, `TestCoverage_Login_SinCampos_400`, `TestCoverage_Login_EmailInexistente_401`.
- `back/auth/internal/handler/http/e2e_full_flow_test.go`: `TestE2E_RegisterLoginMeRefreshLogout` (login 200, login clave errónea 401), `TestE2E_ForgotReset` (login con clave nueva/vieja).
- `back/auth/internal/service/auth_integration_test.go`: `TestAuthIntegration_RegisterLoginRefreshLogout` (login válido + `invalid_credentials` contra Postgres real).
