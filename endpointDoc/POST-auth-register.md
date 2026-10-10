# POST /auth/register

## Req
- Método: `POST`, ruta: `/auth/register` (pública, sin auth; `back/auth/cmd/server/main.go:89`).
- Headers: `Content-Type: application/json`.
- Body: `{"email":"e2e@test.invalid","password":"Pass1234"}` (opcionales: `display_name`, `photo_url`, `phone`, `institution`, `description`, `visibility`).

## Res
- `201`: `{"id":"<uuid>","email":"e2e@test.invalid","created_at":"2026-10-10T12:00:00Z"}`.
- `400`: `{"error":{"code":"bad_request|invalid_email|weak_password|invalid_visibility","message":"..."}}` (JSON malformado, campos faltantes, `visibility` inválida).
- `409`: `{"error":{"code":"email_taken","message":"..."}}` (email duplicado).

## Flujo
- `back/auth/internal/handler/http/auth_handler.go` (`Register`) → `back/auth/internal/service/auth_service.go` (`Register`) → `back/auth/internal/repository/user_repository.go` + `back/auth/internal/repository/profile_repository.go` (creación de usuario + perfil en la misma tx).

## Tests
- `back/auth/internal/handler/http/auth_extra_coverage_test.go`: `TestCoverage_Register_JSONMalformado_400`, `TestCoverage_Register_CamposFaltantes_400`, `TestCoverage_Register_VisibilityInvalida_400`, `TestCoverage_RegistroConcurrente_NoDuplica`.
- `back/auth/internal/handler/http/e2e_full_flow_test.go`: `TestE2E_RegisterLoginMeRefreshLogout` (201 + duplicado 409), `TestE2E_ForgotReset`, `TestE2E_OAuthPerfilInternasHealth`.
- `back/auth/internal/service/auth_integration_test.go`: `TestAuthIntegration_RegisterLoginRefreshLogout` (register + duplicado `email_taken` contra Postgres real).
