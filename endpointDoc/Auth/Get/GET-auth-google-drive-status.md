# GET /auth/google-drive/status

## Req
- Método: `GET`, ruta: `/auth/google-drive/status` (protegida; `back/auth/cmd/server/main.go:96`).
- Headers requeridos: `Authorization: Bearer <access_token>`.

## Res
- `200`: `{"connected":true|false,"reconnect_required":false}` con header `Cache-Control: private, no-store` (nunca expone tokens).
- `401`: `{"error":{"code":"unauthorized","message":"..."}}` (sin token / sin user_id en contexto).

## Flujo
- `back/auth/internal/handler/http/drive_handler.go` (`Status`) → `back/auth/internal/service/drive_oauth_service.go` (`GetGoogleDriveConnectionStatus`) → `back/auth/internal/repository/oauth_repository.go`; auth vía `back/auth/internal/middleware/`.

## Tests
- `back/auth/internal/handler/http/auth_extra_coverage_test.go`: `TestCoverage_Protegidas_SinToken_401` (sin token 401), `TestCoverage_DriveStatus_Conectado_Desconectado` (conectado/desconectado 200, `Cache-Control`, no fuga de tokens, sin user 401).
- `back/auth/internal/handler/http/e2e_full_flow_test.go`: `TestE2E_OAuthPerfilInternasHealth` (status conectado tras connect válido).
