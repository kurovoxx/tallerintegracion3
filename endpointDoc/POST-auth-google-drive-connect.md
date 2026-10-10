# POST /auth/google-drive/connect

## Req
- Método: `POST`, ruta: `/auth/google-drive/connect` (protegida; `back/auth/cmd/server/main.go:95`).
- Headers requeridos: `Authorization: Bearer <access_token>`, `Content-Type: application/json`.
- Body: `{"oauth_code":"valid-drive-code"}` (opcionales: `expected_email`, `redirect_uri` con allowlist).

## Res
- `200`: `{"connected":true}`.
- `400`: `{"error":{"code":"bad_request|email_mismatch|invalid_redirect_uri|invalid_oauth_code","message":"..."}}` (código inválido/expirado, `oauth_code` ausente).
- `401`: `{"error":{"code":"unauthorized","message":"..."}}` (sin token).
- `502`: `{"error":{"code":"google_unavailable","message":"..."}}` (Google caído; rama del handler, ejercitada en Calendar pero no en Drive por los tests nuevos).

## Flujo
- `back/auth/internal/handler/http/drive_handler.go` (`Connect`) → `back/auth/internal/service/drive_oauth_service.go` (`Connect`) → `back/auth/internal/repository/oauth_repository.go` (upsert conexión); provider OAuth real en prod, moqueado en tests.

## Tests
- `back/auth/internal/handler/http/e2e_full_flow_test.go`: `TestE2E_OAuthPerfilInternasHealth` (código `"bad"` 400, `"valid-drive-code"` 200 con `"connected":true`; provider `e2eDriveProvider` moqueado, sin red).
