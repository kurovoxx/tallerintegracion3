# GET /internal/users/lookup

## Req
- Método: `GET`, ruta: `/internal/users/lookup` (interna servicio-a-servicio; `back/auth/cmd/server/main.go:107`; grupo `/internal` con `RequireInternalKey`).
- Headers requeridos: `X-Internal-Key: <clave-compartida>`. Sin JWT de usuario.
- Query: `?ids=<uuid1>,<uuid2>` (opcional; sin ids responde `[]`; inexistentes se omiten, nunca 404 parcial).

## Res
- `200`: `[{"user_id":"...","email":"..."}]` (siempre array, nunca `null`; `[]` si sin ids).
- `401`: `{"error":{"code":"unauthorized","message":"..."}}` (sin `X-Internal-Key` o clave errónea).

## Flujo
- `back/auth/internal/handler/http/internal_users_handler.go` (`Lookup`) → `back/auth/internal/service/users_service.go` (`Lookup`) → `back/auth/internal/repository/user_repository.go`; auth vía `back/auth/internal/middleware/` (`RequireInternalKey`).

## Tests
- `back/auth/internal/handler/http/auth_extra_coverage_test.go`: `TestCoverage_Internas_SinKey_401` (sin key 401; con key válida lookup vacío 200 `[]`).
- `back/auth/internal/handler/http/e2e_full_flow_test.go`: `TestE2E_OAuthPerfilInternasHealth` (sin key 401, con key 200).
