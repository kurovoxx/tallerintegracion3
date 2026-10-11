# GET /health (academic-service)

## Req
- Método: `GET`, ruta: `/health` (pública, sin auth; `back/infacadem/cmd/server/main.go:52`). Servicio `academic-service` (puerto 8081 por defecto).
- Sin headers ni body. Responde 200 aun sin BD configurada.

## Res
- `200`: `{"status":"UP","service":"academic-service"}`.

## Flujo
- `back/infacadem/cmd/server/main.go` (closure inline, sin service/repository; scaffold 3_1_1 sin handlers todavía).

## Tests
- No existen scripts de test en `back/infacadem` (scaffold sin `*_test.go`; verificar con Tester en tareas 3_1_2+). Verificado manualmente por Reviewer: arranque sin `DATABASE_URL` → 200 `{"status":"UP","service":"academic-service"}`.
