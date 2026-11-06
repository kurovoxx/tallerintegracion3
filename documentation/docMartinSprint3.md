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
| `scripts/validate-migrations.sh` | Script Bash que, dado un `DATABASE_URL`, crea los schemas `identity`, `academic`, `notes`, `social`, `ai` si no existen y aplica todas las migraciones de `auth`, `notes`, `social`, `infacadem` en orden numérico con `psql -v ON_ERROR_STOP=1`. Sale con error si cualquiera falla. |
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

- Formato de archivo SQL: comentario inicial descriptivo, `CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`, schema explícito (`academic.`, `identity.`, etc.), `user_id` es referencia lógica sin FK física.
- Numeración secuencial: `001` antes que `002`, nunca reordenar ni editar migraciones ya aplicadas en producción; se crea una nueva `NNN` para corregir.
- `CREATE SCHEMA IF NOT EXISTS <schema>` al inicio de cada migración inicial para que sea autónoma.

### Verificación

- Migración `infacadem 001` aplicada contra `postgres:16-alpine` vacía: 7 tablas creadas (`absences`, `contents`, `curricula`, `grade_requirements`, `grades`, `schedules`, `subjects`), sin errores.
- Segunda ejecución de la misma migración: idempotente (NOTICE "already exists", sin fallar).
- `go build ./...` + `go vet ./...` en `back/infacadem`: OK (sin cambios de Go, solo archivos SQL nuevos).
- YAMLs de ambos workflows: parsean correctamente (`yaml.safe_load`).
- `docker compose config`: no se modificó, sigue válido.

### No entra

- Migrator automático al arrancar cada servicio (el `cmd/server/main.go` de infacadem sigue sin aplicar migraciones solas; solo hace check best-effort de `academic.curricula`). Queda para una tarea posterior o para 4_5_2 (retry de DB).
- `initContainer` en `k8s/manifests.yaml` para aplicar migraciones antes de los deployments: k8s sigue sin probes ni initContainers en los servicios (tema de 4_5_2).
- Migraciones nuevas para `realtime`: no tiene esquema propio definido aún.
- Cambios en migraciones existentes de `auth`, `notes`, `social`: no se modificaron, solo se validan en el gate.
