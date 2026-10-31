# GET /auth/google-calendar/status

## Req
- Método: `GET`, ruta: `/auth/google-calendar/status` (protegida; `back/auth/cmd/server/main.go:99`).
- Headers requeridos: `Authorization: Bearer <access_token>`.

## Res
- `200`: `{"connected":true}` o `{"connected":true,"external_email":"cal@example.com"}` con header `Cache-Control: private, no-store` (nunca expone tokens).
- `401`: `{"error":{"code":"unauthorized","message":"..."}}` (sin token).

## Flujo
- `back/auth/internal/handler/http/calendar_handler.go` (`Status`) → `back/auth/internal/service/calendar_oauth_service.go` (`GetCalendarConnectionStatus`) → `back/auth/internal/repository/oauth_repository.go`; auth vía `back/auth/internal/middleware/`.

## Tests
- `back/auth/internal/handler/http/auth_extra_coverage_test.go`: `TestCoverage_Protegidas_SinToken_401` (sin token 401).
- `back/auth/internal/handler/http/e2e_full_flow_test.go`: `TestE2E_OAuthPerfilInternasHealth` (status conectado tras connect válido).
