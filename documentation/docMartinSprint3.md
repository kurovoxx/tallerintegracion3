# Documentación de tareas — Martín (Sprint 3)

Doc vivo: una sección por tarea (4_5_N), en orden de ejecución. Endpoints por
separado en `endpointDoc/`.

## 4_5_1 — Migraciones versionadas + gate de deploy

### Objetivo

Que cada cambio de esquema de base de datos quede como un archivo SQL
versionado, numerado e idempotente, y que el CI impida construir/subir
imágenes si las migraciones de algún servicio no aplican limpiamente contra
una Postgres real (gate de deploy previo a `build-images`).

### Alcance de esta sesión

- `back/infacadem` no tenía carpeta `db/migrations/` (solo `db/queries`).
- `auth`, `notes` y `social` ya tenían migraciones versionadas (`001`, `002`…);
  se valida que sigan aplicando limpiamente en el gate.
- Se crea un workflow de CI que levanta Postgres efímera, aplica todas las
  migraciones de todos los servicios y falla si cualquiera devuelve error.
- `build-images.yml` ahora tiene un job `validate-migrations` con `needs:`
  en el job `build`: si las migraciones fallan, no se construyen ni suben
  imágenes a GHCR.

### Archivos creados

| Archivo | Contenido |
|---|---|
| `back/infacadem/db/migrations/001_initial_schema.sql` | Esquema inicial del servicio académico (infacadem) en schema `academic`: `curricula`, `subjects`, `schedules`, `contents`, `grades`, `grade_requirements`, `absences`. Idempotente (`CREATE TABLE IF NOT EXISTS` / `CREATE INDEX IF NOT EXISTS`), con `CREATE SCHEMA IF NOT EXISTS academic` al inicio para que sea autónoma contra una Postgres vacía. |
| `scripts/validate-migrations.sh` | Script Bash que, dado un `DATABASE_URL`, primero aplica `docker/postgres/init.sql` (esquema base: `identity.users` y tablas de todos los servicios) y luego aplica todas las migraciones versionadas de `auth`, `notes`, `social`, `infacadem` en orden numérico con `psql -v ON_ERROR_STOP=1`. Acepta `INIT_SQL_PATH` para sobrescribir la ruta del `init.sql`. Sale con error si cualquiera falla. |
| `.github/workflows/test-migrations.yml` | Workflow independiente de GitHub Actions: levanta `postgres:16-alpine` como service container, ejecuta `scripts/validate-migrations.sh`. Se dispara en push/PR a `Development` cuando cambian archivos bajo `back/**/db/migrations/**`, `docker/postgres/init.sql` o el propio workflow. |

### Archivos modificados

| Archivo | Cambio |
|---|---|
| `.github/workflows/build-images.yml` | Agregado job `validate-migrations` (Postgres service container + `scripts/validate-migrations.sh`) y `needs: [validate-migrations]` en el job `build`. Es el gate de deploy: si las migraciones fallan, no se build-pushea a GHCR. |

### Versiones y tecnologías

- `postgres:16-alpine` (service container en CI)
- `psql` con `-v ON_ERROR_STOP=1` (falla al primer error)
- Migraciones SQL idempotentes, numeradas de 3 dígitos (`001`, `002`…)

### Comandos

- Local: `DATABASE_URL=postgresql://... scripts/validate-migrations.sh`
- CI: workflow `test-migrations.yml` (autónomo) o job `validate-migrations` dentro de `build-images.yml`

### Convenciones

- El validador aplica primero `docker/postgres/init.sql` (esquema base con `identity.users` y tablas de todos los servicios) y luego las migraciones versionadas encima. Las migraciones de auth/notes/social asumen que esas tablas base ya existen.
- Formato de archivo SQL: comentario inicial descriptivo, `CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`, schema explícito (`academic.`, `identity.`, etc.), `user_id` es referencia lógica sin FK física.
- Numeración secuencial: `001` antes que `002`, nunca reordenar ni editar migraciones ya aplicadas en producción; se crea una nueva `NNN` para corregir.
- `CREATE SCHEMA IF NOT EXISTS <schema>` al inicio de cada migración inicial para que sea autónoma.

### Verificación

- Migración `infacadem 001` aplicada contra `postgres:16-alpine` vacía: 7 tablas creadas (`absences`, `contents`, `curricula`, `grade_requirements`, `grades`, `schedules`, `subjects`), sin errores.
- Segunda ejecución de la misma migración: idempotente (NOTICE "already exists", sin fallar).
- Gate completo (init.sql + migraciones de auth/notes/social/infacedadem) probado contra `postgres:16-alpine` vacía: todos los SQL aplican sin errores, código de salida 0.
- `go build ./...` + `go vet ./...` en `back/infacadem`: OK (sin cambios de Go, solo archivos SQL nuevos).
- YAMLs de ambos workflows: parsean correctamente (`yaml.safe_load`).
- `docker compose config`: no se modificó, sigue válido.

### No entra

- Migrator automático al arrancar cada servicio (el `cmd/server/main.go` de infacadem sigue sin aplicar migraciones solas; solo hace check best-effort de `academic.curricula`). Queda para una tarea posterior.
- `initContainer` en `k8s/manifests.yaml` para aplicar migraciones antes de los deployments: no se agregó; los servicios confían en que el esquema base ya exista (vía `docker/postgres/init.sql` en compose o aplicado manualmente en pillan).
- Migraciones nuevas para `realtime`: no tiene esquema propio definido aún.
- Cambios en migraciones existentes de `auth`, `notes`, `social`: no se modificaron, solo se validan en el gate.

## 4_5_2 — Readiness/liveness probes + retry de DB al arrancar

### Objetivo

Que k8s sepa cuándo cada servicio está listo para recibir tráfico (readiness) y cuándo está vivo (liveness), y que cada servicio no falle si Postgres aún no está lista al arrancar (retry con backoff en lugar de `log.Fatalf` al primer intento).

### Archivos modificados

| Archivo | Cambio |
|---|---|
| `back/auth/cmd/server/main.go` | `NewPool` + `Ping` envueltos en loop de retry (máx. 30 intentos, backoff lineal 1s/2s/3s…). Si tras todos los intentos falla → `log.Fatalf`. Se agregó import de `pgxpool`. |
| `back/notes/cmd/server/main.go` | Igual que auth. Respeta el modo memoria cuando `DATABASE_URL` está vacío. |
| `back/social/cmd/server/main.go` | Igual que auth. |
| `back/infacadem/cmd/server/main.go` | Igual patrón de retry, pero se mantiene el comportamiento tolerante: si tras todos los intentos falla, `pool = nil` (sin `log.Fatalf`) y `/health/db` responde 503. |
| `k8s/manifests.yaml` | Agregado `readinessProbe` y `livenessProbe` (httpGet a `/health`) a los 4 deployments de servicios Go (auth, notes, social, infacadem). El front no lleva probes (nginx estático, sin DB). |

### Detalles de los probes (k8s)

| Probe | Path | Puerto | initialDelaySeconds | periodSeconds | failureThreshold |
|---|---|---|---|---|---|
| readiness | `/health` | HTTP del servicio | 10 | 5 | 3 |
| liveness | `/health` | HTTP del servicio | 15 | 10 | 5 |

### Detalles del retry de DB

- Máximo 30 intentos.
- Backoff lineal: `time.Duration(i+1) * time.Second` (1s, 2s, 3s… 30s).
- Cada intento: `NewPool` con contexto de 5s + `Ping` con mismo contexto.
- Si `NewPool` o `Ping` falla, se cierra el pool parcial (si existe) y se reintenta.
- Si agota los intentos: `log.Fatalf` en auth/notes/social; `pool = nil` (advertencia) en infacadem.

### Cómo interactúa con el resto del proyecto

- Con **4_5_1**: el gate de deploy valida que las migraciones aplican; el retry de DB asegura que el servicio no muera si Postgres tarda en estar lista al desplegar. Son complementarios.
- Con **front**: los probes evitan que nginx/k8s envíe tráfico a pods que aún no están listos, reduciendo 503 en el front durante despliegues o reinicios.
- Con **4_5_5** (microservicio IA): cuando se cree `back/ai`, debería llevar el mismo patrón de retry de DB y probes si usa Postgres.

### Verificación

- `go build ./...` + `go vet ./...` en auth, notes, social, infacadem: OK en los 4.
- `k8s/manifests.yaml`: 10 documentos YAML válidos; los 4 deployments de servicios Go tienen `readinessProbe` y `livenessProbe`.
- `/health` ya existía en los 4 servicios (responde `{"status":"UP"}`); se reusa para ambos probes sin cambios de handler.
- No se modificó `docker-compose.yml` (compose ya espera que Postgres esté up vía `depends_on` implícito o el contenedor simplemente arranca; el retry de DB cubre el caso de que Postgres acepte conexiones más tarde).
