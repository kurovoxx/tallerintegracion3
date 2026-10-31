# DELETE /auth/google-drive/connection

## Req
- Método: `DELETE`, ruta: `/auth/google-drive/connection` (protegida; `back/auth/cmd/server/main.go:97`).
- Headers requeridos: `Authorization: Bearer <access_token>`. Sin body.

## Res
- `204`: sin body (revoca en Google best-effort y borra la fila; idempotente sin conexión previa).
- `401`: `{"error":{"code":"unauthorized","message":"..."}}` (sin token).

## Flujo
- `back/auth/internal/handler/http/drive_handler.go` (`Disconnect`) → `back/auth/internal/service/drive_oauth_service.go` (`Disconnect`) → `back/auth/internal/repository/oauth_repository.go`; auth vía `back/auth/internal/middleware/`.

## Tests
- `back/auth/internal/handler/http/auth_extra_coverage_test.go`: `TestCoverage_Protegidas_SinToken_401` (sin token 401).
- `back/auth/internal/handler/http/e2e_full_flow_test.go`: `TestE2E_OAuthPerfilInternasHealth` (disconnect 204; vacía tokens antes del DELETE para evitar revoke HTTP real a Google).
