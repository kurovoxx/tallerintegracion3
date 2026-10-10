# PATCH /profile/me

## Req
- Método: `PATCH`, ruta: `/profile/me` (protegida; `back/auth/cmd/server/main.go:120`).
- Headers requeridos: `Authorization: Bearer <access_token>`, `Content-Type: application/json`.
- Body (parcial): `{"display_name":"Nuevo"}` (cualquiera de `display_name`, `photo_url`, `phone`, `institution`, `description`, `visibility`; `user_id` prohibido).

## Res
- `200`: perfil actualizado (misma forma que `GET /profile/me`).
- `400`: `{"error":{"code":"bad_request|invalid_visibility|invalid_display_name|...","message":"..."}}` (`visibility` inválida, body vacío, `user_id` enviado, JSON malformado).
- `401`: `{"error":{"code":"unauthorized","message":"..."}}` (sin token / inválido).
- `404`: `{"error":{"code":"profile_not_found","message":"..."}}` (rama del handler, no cubierta por tests nuevos).

## Flujo
- `back/auth/internal/handler/http/profile_handler.go` (`PatchProfile`) → `back/auth/internal/service/profile_service.go` (`UpdateProfile`) → `back/auth/internal/repository/profile_repository.go`; auth vía `back/auth/internal/middleware/`.

## Tests
- `back/auth/internal/handler/http/auth_extra_coverage_test.go`: `TestCoverage_Protegidas_SinToken_401` (sin token 401).
- `back/auth/internal/handler/http/e2e_full_flow_test.go`: `TestE2E_OAuthPerfilInternasHealth` (`visibility:"invisible"` 400, patch válido 200 con `"Nuevo"` en body).
