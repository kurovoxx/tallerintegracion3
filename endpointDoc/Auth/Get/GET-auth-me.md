# GET /auth/me

## Req
- Método: `GET`, ruta: `/auth/me` (protegida; `back/auth/cmd/server/main.go:115`; grupo con `RequireAuth`).
- Headers requeridos: `Authorization: Bearer <access_token>`.

## Res
- `200`: `{"user_id":"<uuid>"}` (user_id inyectado por el middleware desde el JWT).
- `401`: `{"error":{"code":"unauthorized","message":"..."}}` (sin token, token inválido/expirado o con esquema erróneo).

## Flujo
- `back/auth/cmd/server/main.go` (closure inline) ← `back/auth/internal/middleware/` (`RequireAuth`, `GetUserID`; firma + expiración JWT vía `back/auth/internal/service/jwt_service.go`). Sin repository (no consulta BD).

## Tests
- `back/auth/internal/handler/http/auth_extra_coverage_test.go`: `TestCoverage_Me_ConToken_200`, `TestCoverage_Me_SinToken_401` (monta `/auth/me` igual que `main.go`: middleware real + closure).
- `back/auth/internal/handler/http/e2e_full_flow_test.go`: `TestE2E_RegisterLoginMeRefreshLogout` (sin token 401, con token 200 y `user_id` coherente), `TestE2E_OAuthPerfilInternasHealth` (token inválido 401).
