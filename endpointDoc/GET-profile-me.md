# GET /profile/me

## Req
- Método: `GET`, ruta: `/profile/me` (protegida; `back/auth/cmd/server/main.go:119`).
- Headers requeridos: `Authorization: Bearer <access_token>`.

## Res
- `200`: `{"display_name":"...","photo_url":null,"phone":null,"institution":null,"description":null,"visibility":"private"}` (+ `email` si se resolvió desde `identity.users`).
- `401`: `{"error":{"code":"unauthorized","message":"..."}}` (sin token / inválido).
- `404`: `{"error":{"code":"profile_not_found","message":"..."}}` (no cubierto por los tests nuevos; existe como rama del handler).

## Flujo
- `back/auth/internal/handler/http/profile_handler.go` (`GetProfile`) → `back/auth/internal/service/profile_service.go` (`GetProfile`) → `back/auth/internal/repository/profile_repository.go` (+ `user_repository.go` para el email best-effort); auth vía `back/auth/internal/middleware/`.

## Tests
- `back/auth/internal/handler/http/auth_extra_coverage_test.go`: `TestCoverage_Protegidas_SinToken_401` (sin token 401), `TestCoverage_Profile_ConMiddleware_TokenInvalido_401`.
- `back/auth/internal/handler/http/e2e_full_flow_test.go`: `TestE2E_RegisterLoginMeRefreshLogout` (con token 200), `TestE2E_OAuthPerfilInternasHealth`.
