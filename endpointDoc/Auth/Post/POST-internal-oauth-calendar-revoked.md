# POST /internal/oauth/calendar-revoked

## Req
- Método: `POST`, ruta: `/internal/oauth/calendar-revoked` (interna servicio-a-servicio; `back/auth/cmd/server/main.go:106`; grupo `/internal` con `RequireInternalKey`).
- Headers requeridos: `X-Internal-Key: <clave-compartida>`, `Content-Type: application/json`. Sin JWT de usuario.
- Body: `{"user_id":"u1"}` (lo llama Social al recibir 401/403 de Google Calendar).

## Res
- `204`: sin body (idempotente: sin fila también 204 para no filtrar existencia).
- `400`: `{"error":{"code":"bad_request","message":"..."}}` (JSON inválido o `user_id` vacío).
- `401`: `{"error":{"code":"unauthorized","message":"..."}}` (sin `X-Internal-Key` o clave errónea).

## Flujo
- `back/auth/internal/handler/http/internal_oauth_handler.go` (`ReportCalendarRevoked`) → `back/auth/internal/service/calendar_oauth_service.go` (`ReportCalendarPermissionDenied`) → `back/auth/internal/repository/oauth_repository.go`; auth vía `back/auth/internal/middleware/` (`RequireInternalKey`).

## Tests
- `back/auth/internal/handler/http/auth_extra_coverage_test.go`: `TestCoverage_Internas_SinKey_401` (sin key 401).
- Sin test nuevo que ejerza el 204 con key válida en esta ruta (el e2e cubre el flujo OAuth por las rutas públicas; hueco menor, no bloqueante).
