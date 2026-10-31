# GET /health

## Req
- Método: `GET`, ruta: `/health` (pública, sin auth; `back/auth/cmd/server/main.go:123`).
- Sin headers ni body.

## Res
- `200`: `{"status":"UP","service":"auth-service"}`.

## Flujo
- `back/auth/cmd/server/main.go` (closure inline, sin service/repository).

## Tests
- `back/auth/internal/handler/http/auth_extra_coverage_test.go`: `TestCoverage_Health_200` (monta `/health` igual que `main.go`, assert de payload).
- `back/auth/internal/handler/http/e2e_full_flow_test.go`: `TestE2E_OAuthPerfilInternasHealth` (200 con `"UP"` en body).
- Nota: `GET /health/db` (main.go:135) NO está cubierto por los 3 tests nuevos; no se documenta aquí.
