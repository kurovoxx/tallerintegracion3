# Benjamín Rojas

## 3_3_6 — Puertos gRPC Social en Docker y Kubernetes

- Social escucha HTTP en `8083` y gRPC en `9083`.
- `docker-compose.yml` publica `9083:9083` una sola vez y configura
  `SOCIAL_GRPC_PORT=9083`. La imagen declara ambos puertos en `EXPOSE`.
- `k8s/manifests.yaml` ya declara `containerPort: 9083`,
  `SOCIAL_GRPC_PORT=9083` y el puerto `grpc` del Service `social`
  (`port: 9083`, `targetPort: 9083`). No fue necesario modificarlo.
- Dentro de Compose y Kubernetes, los clientes usan `social:9083`;
  Notes ya tiene `SOCIAL_GRPC_ADDR` configurada con esa dirección.
- Las llamadas requieren metadata `x-internal-key` con la misma
  `INTERNAL_API_KEY` del servidor. Kubernetes la obtiene de `ti3-secrets`.
- Para acceder desde el equipo al clúster durante una prueba:
  `kubectl -n student-brojas port-forward svc/social 9083:9083`.

## 3_3_11 — CI de Social

Se amplió `.github/workflows/test-social-groups.yml`, conservando el nombre
del workflow y del job para mantener los checks existentes.

Se ejecuta en push, pull request o manualmente. Sus filtros incluyen Social,
los protos Social/Auth/Common, el esquema de PostgreSQL, Compose, los
manifiestos de Kubernetes y el propio workflow.

El job usa la versión de Go de `back/social/go.mod` y PostgreSQL 16 en una
base temporal `social_test`. Inicializa `docker/postgres/init.sql`, ejecuta
`go vet ./...`, `go build ./...` y la suite completa:

```sh
cd back/social
go test ./... -race -p 1 -count=1 -timeout=5m -json
```

Incluye tests unitarios, gRPC con bufconn, integración y E2E existentes de
Social. Falla ante errores o tests omitidos, y exige que
`TestGroupsPostgresE2E_MembershipLifecycle` produzca un PASS. Los paquetes
sin archivos de pruebas no se consideran tests omitidos. La salida JSON
queda visible en el log de Actions, incluso si falla una prueba.

Para integración local se necesita `SOCIAL_TEST_DATABASE_URL` apuntando a
una base exclusiva de pruebas con el esquema inicializado. Sin esa variable,
las pruebas que requieren PostgreSQL se omiten. Esta suite no reemplaza el
E2E entre procesos Social↔Notes↔Identity de la tarea 3_3_5.

### Verificación local

- `go test ./... -count=1`, `go vet ./...` y `go build ./...`: correctos.
- Las pruebas con PostgreSQL se omitieron porque no hay una base de pruebas
  configurada. Docker y kubectl no están disponibles en este entorno;
  queda pendiente ejecutar Actions y comprobar el despliegue real.
