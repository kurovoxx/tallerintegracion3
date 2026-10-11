# GET /health/db (academic-service)

## Req
- Método: `GET`, ruta: `/health/db` (pública, sin auth; `back/infacadem/cmd/server/main.go:59`). Servicio `academic-service`.
- Sin headers ni body.

## Res
- `200`: `{"status":"UP","database":"...","user":"...","now":"..."}` (pool Ping OK).
- `503`: `{"status":"DOWN","error":{"code":"db_unconfigured","message":"..."}}` (sin `DATABASE_URL` o pool no disponible al arrancar) o `{"status":"DOWN","error":{"code":"db_unreachable","message":"..."}}` (Ping fallido en runtime).

## Flujo
- `back/infacadem/cmd/server/main.go` (closure inline) → pool creado al arrancar vía `back/infacadem/internal/repository/db.go` (`NewPool`, `Ping`).

## Tests
- No existen scripts de test en `back/infacadem` (scaffold sin `*_test.go`). Verificado manualmente por Reviewer: sin `DATABASE_URL` → 503 `db_unconfigured` gracioso, sin panic y sin bloquear el arranque.
