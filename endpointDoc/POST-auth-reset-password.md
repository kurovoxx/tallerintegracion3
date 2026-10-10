# POST /auth/reset-password

## Req
- Método: `POST`, ruta: `/auth/reset-password` (pública, sin auth; `back/auth/cmd/server/main.go:94`).
- Headers: `Content-Type: application/json`, `Cache-Control: no-store` en respuesta.
- Body: `{"email":"reset@test.invalid","code":"12345678","password":"NewPass123"}`.

## Res
- `200`: `{"message":"Contraseña actualizada. Ya puedes iniciar sesión."}`.
- `400`: `{"error":{"code":"bad_request|invalid_reset_code","message":"..."}}` (campos faltantes, código inválido/expirado o ya consumido).

## Flujo
- `back/auth/internal/handler/http/password_reset_handler.go` (`Reset`) → `back/auth/internal/service/password_reset_service.go` (`Reset`) → `back/auth/internal/repository/password_reset_repository.go` + `back/auth/internal/repository/user_repository.go` (actualiza hash, revoca sesiones, consume el código).

## Tests
- `back/auth/internal/handler/http/e2e_full_flow_test.go`: `TestE2E_ForgotReset` (código inválido 400, código válido 200, reuso del mismo código 400, login con clave nueva 200 / vieja 401).
