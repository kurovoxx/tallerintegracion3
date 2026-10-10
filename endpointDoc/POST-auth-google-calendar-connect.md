# POST /auth/google-calendar/connect

## Req
- Método: `POST`, ruta: `/auth/google-calendar/connect` (protegida; `back/auth/cmd/server/main.go:98`).
- Headers requeridos: `Authorization: Bearer <access_token>`, `Content-Type: application/json`.
- Body: `{"oauth_code":"valid-cal-code"}` (opcional `redirect_uri` con allowlist).

## Res
- `200`: `{"connected":true}`.
- `400`: `{"error":{"code":"bad_request|invalid_redirect_uri|invalid_oauth_code","message":"..."}}` (código inválido/expirado).
- `401`: `{"error":{"code":"unauthorized","message":"..."}}` (sin token).
- `502`: `{"error":{"code":"google_unavailable","message":"..."}}` (Google caído).

## Flujo
- `back/auth/internal/handler/http/calendar_handler.go` (`Connect`) → `back/auth/internal/service/calendar_oauth_service.go` (`ConnectWithRedirect`) → `back/auth/internal/repository/oauth_repository.go`; provider OAuth real en prod, moqueado en tests.

## Tests
- `back/auth/internal/handler/http/e2e_full_flow_test.go`: `TestE2E_OAuthPerfilInternasHealth` (`"google-down"` 502, `"valid-cal-code"` 200; provider `e2eCalendarProvider` moqueado, sin red).
