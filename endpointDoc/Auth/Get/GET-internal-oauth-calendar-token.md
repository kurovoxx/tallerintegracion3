# GET /internal/oauth/calendar-token

## Req
- Método: `GET`, ruta: `/internal/oauth/calendar-token` (interna servicio-a-servicio; `back/auth/cmd/server/main.go:105`; grupo `/internal` con `RequireInternalKey`).
- Headers requeridos: `X-Internal-Key: <clave-compartida>`. Sin JWT de usuario.
- Query: `?user_id=<uuid>` (requerido).

## Res
- `200`: `{"access_token":"<token>"}` (el service refresca si está vencido).
- `400`: `{"error":{"code":"bad_request","message":"user_id requerido"}}` (sin `user_id`).
- `401`: `{"error":{"code":"unauthorized","message":"..."}}` (sin `X-Internal-Key` o clave errónea).
- `404`: `{"error":{"code":"not_connected","message":"..."}}` / `403` `calendar_connection_invalid` / `502` `google_unavailable` (ramas del handler, no todas cubiertas por tests nuevos).

## Flujo
- `back/auth/internal/handler/http/internal_oauth_handler.go` (`GetCalendarToken`) → `back/auth/internal/service/calendar_oauth_service.go` (`GetValidAccessToken`) → `back/auth/internal/repository/oauth_repository.go`; auth vía `back/auth/internal/middleware/` (`RequireInternalKey`).

## Tests
- `back/auth/internal/handler/http/auth_extra_coverage_test.go`: `TestCoverage_Internas_SinKey_401` (sin key 401 en las 3 rutas internas; lookup con key 200 `[]`).
- `back/auth/internal/handler/http/e2e_full_flow_test.go`: `TestE2E_OAuthPerfilInternasHealth` (con key 200 con `access_token`; sin `user_id` 400).
