# GET /health y GET /health/db (Social)

Sondas de vida del microservicio `social`. Públicas, sin JWT.

## Req
- `GET /health` (`back/social/cmd/server/main.go:226`) y `GET /health/db` (`main.go:230`). Sin headers ni body.

## Res
- `/health` `200`: `{"status":"UP","service":"social-service"}`. No toca la base.
- `/health/db` `200`: `{"status":"UP","database","user","now","checks":{"social_groups":<n>}}`.
- `/health/db` `503`: `{"status":"DOWN","error":{"code":"db_unreachable","message":"..."}}` si falla el ping (timeout 3 s). El mensaje incluye el texto del driver.

## Flujo
- Closures en `startGin`; `/health/db` hace `pool.Ping`, `select now(), current_database(), current_user` y `count(*)` de `social.groups`.
- Por el gateway: `/api-social/health` y `/api-social/health/db` (`front/nginx.conf`).

## Tests
- `test/social_routes_e2e_test.go`: subtest `health_and_group_administration` (ambas rutas con la base hija de pruebas).
- `test/cluster_smoke_test.go`: salud en el despliegue Kubernetes.
