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
- `k8s/manifests.yaml`: 12 documentos YAML válidos; los 5 deployments de servicios Go (incluido `ai`) tienen `readinessProbe` y `livenessProbe`.
- `/health` ya existía en los 4 servicios (responde `{"status":"UP"}`); se reusa para ambos probes sin cambios de handler.
- No se modificó `docker-compose.yml` (compose ya espera que Postgres esté up vía `depends_on` implícito o el contenedor simplemente arranca; el retry de DB cubre el caso de que Postgres acepte conexiones más tarde).

## 4_5_5 — Microservicio IA: scaffold + API

### Objetivo

Levantar `back/ai` como un microservicio Go independiente, con su propio puerto, API REST y estructura de capas (`handler → service → model`), siguiendo el mismo patrón que los demás servicios (`auth`, `notes`, `social`, `infacadem`). Sin dependencia obligatoria de Postgres (modo tolerante/scaffold). La integración real con Gemini (PDF→Markdown) se conecta en la tarea 4_5_6.

### Archivos creados

| Archivo | Contenido |
|---|---|
| `back/ai/go.mod` | Módulo Go independiente (`github.com/kurovoxx/tallerintegracion3/back/ai`, Go 1.26) con `gin` + `godotenv`. |
| `back/ai/internal/config/config.go` | `Config{Port, AIPort, GeminiAPIKey, InternalAPIKey, DatabaseURL}` con carga de `.env` y defaults. Puerto por defecto `8084`. |
| `back/ai/internal/model/ai_models.go` | `GenerateRequest{Input, Operation}`, `GenerateResponse{Output, ID, Provider}`, `HealthResponse{Status, Service}`. |
| `back/ai/internal/service/ai_service.go` | Interfaz `AIService{Generate}` + `MockAIService` (respuesta simulada, sin proveedor externo). |
| `back/ai/internal/handler/http/health_handler.go` | `GET /health` → `{"status":"UP","service":"ai-service"}`. |
| `back/ai/internal/handler/http/ai_handler.go` | `POST /ai/generate` → recibe `GenerateRequest`, llama al servicio, devuelve `GenerateResponse`. |
| `back/ai/cmd/server/main.go` | Servidor Gin con `/health`, `/health/db` y `/ai/generate`. Apagado gracioso con SIGINT/SIGTERM. |
| `back/ai/Dockerfile` | Multistage (golang:1.26-alpine → alpine:3.19), expone `:8084`. |

### Archivos modificados

| Archivo | Cambio |
|---|---|
| `docker-compose.yml` | Agregado servicio `ai` (puerto 8084, `GEMINI_API_KEY`, `INTERNAL_API_KEY`, `DATABASE_URL` opcional, `depends_on: postgres`). |
| `.env.example` | Agregado `AI_PORT=8084` y `GEMINI_API_KEY=` (placeholder). |
| `.github/workflows/build-images.yml` | Agregado `{name: ai, context: back/ai}` a la matriz de build. |
| `k8s/manifests.yaml` | Agregado Deployment `ai` (puerto 8084, `readinessProbe` + `livenessProbe` en `/health`) y Service `ai` (puerto 8084). |

### Endpoints de la API

| Método | Ruta | Responde |
|---|---|---|
| `GET` | `/health` | `{"status":"UP","service":"ai-service"}` |
| `GET` | `/health/db` | `{"status":"UP","service":"ai-service","note":"sin dependencia obligatoria de Postgres (scaffold)"}` |
| `POST` | `/ai/generate` | Recibe `{"input","operation"}`, devuelve `{"output","id","provider"}`. Modo mock devuelve respuesta simulada. |

### Cómo interactúa con el resto del proyecto

- Con **4_5_1**: `back/ai` no tiene migraciones propias en esta tarea (no usa Postgres obligatoriamente). Si en el futuro persiste datos, se agregarán bajo `back/ai/db/migrations/` y se validarán en el gate de deploy.
- Con **4_5_2**: `back/ai` lleva `readinessProbe`/`livenessProbe` en k8s (igual que los demás servicios). No tiene retry de DB porque no depende de Postgres obligatoriamente.
- Con **4_5_6** (Gemini PDF→Markdown): `MockAIService` se reemplazará por una implementación real que llame a Gemini. El endpoint `POST /ai/generate` es el punto de entrada para esa integración.
- Con **front**: el front puede llamar a `/ai/generate` vía el gateway de nginx (prefijo `/api-ai` por definir) una vez exista el endpoint real.

### Verificación

- `go build ./...` + `go vet ./...` en `back/ai`: OK.
- Servidor arranca en `:8084` y responde `/health`, `/health/db`, `/ai/generate` correctamente (modo mock).
- `docker-compose.yml`: parsea correctamente, servicio `ai` con `depends_on: postgres`.
- `k8s/manifests.yaml`: 12 documentos YAML válidos; deployment `ai` tiene ambos probes.
- `.github/workflows/build-images.yml`: `ai` incluido en la matriz de build.

### No entra

- Integración real con Gemini (tarea 4_5_6).
- Persistencia en Postgres (opcional, `DatabaseURL` está en config pero no se usa aún en el scaffold).
- Auth/JWT en endpoints de IA (por ahora público; se añadirá cuando sea necesario).
- `db/migrations/` para `ai` (no hay esquema de BD aún).

## 4_5_6 — Gemini PDF→Markdown (integración + límites)

### Objetivo

Integrar el microservicio IA con la API de Google Gemini para convertir archivos PDF a Markdown. El usuario sube un PDF, el servicio lo envía a Gemini (`gemini-1.5-flash`), recibe el Markdown y se lo devuelve al cliente. No guarda la nota en Notes/Drive todavía (eso se pospone para una tarea posterior).

### Alcance acordado

| Aspecto | Valor |
|---------|-------|
| Endpoint | `POST /ai/pdf-to-markdown` (multipart/form-data, campo `file`) |
| Modelo Gemini | `gemini-1.5-flash` (nivel gratuito en Google AI Studio) |
| Tamaño máximo PDF | 10 MB |
| Rate limit | 5 conversiones por hora por identificador (archivo/IP) |
| Timeout de llamada | 60 segundos |
| Reintentos | 2 (backoff: 2s, 4s ante 429/503) |
| Guardar en Notes | Pospuesto (solo convierte y devuelve el texto) |

### Archivos creados

| Archivo | Contenido |
|---------|-----------|
| `back/ai/internal/service/gemini_ai_service.go` | Implementación real de `AIService` que llama a la API REST de Gemini. Incluye validación de tamaño (10 MB), rate limit en memoria, reintentos (2) y timeout (60s). |
| `back/ai/internal/handler/http/pdf_handler.go` | Handler `POST /ai/pdf-to-markdown`. Recibe `multipart/form-data`, valida que sea PDF, llama al servicio y devuelve `{markdown, filename, size_bytes, provider}`. |

### Archivos modificados

| Archivo | Cambio |
|---------|--------|
| `back/ai/internal/config/config.go` | Agregado `GeminiModel`, `GeminiMaxFileSizeMB` (default 10), `GeminiRateLimit` (default 5). |
| `back/ai/internal/service/ai_service.go` | Agregado `ConvertPDFToMarkdown(pdfData []byte, filename string) (string, error)` a la interfaz `AIService`. Implementado en `MockAIService` (respuesta simulada) y `GeminiAIService` (llamada real a Gemini). |
| `back/ai/cmd/server/main.go` | Wire: si `GEMINI_API_KEY` está configurada, usa `GeminiAIService`; si no, `MockAIService`. Registrada ruta `POST /ai/pdf-to-markdown`. |
| `.env.example` | Agregado `GEMINI_MODEL=gemini-1.5-flash`, `GEMINI_MAX_FILE_SIZE_MB=10`, `GEMINI_RATE_LIMIT=5`. |
| `docker-compose.yml` | Ya tenía `GEMINI_API_KEY` en el servicio `ai`; no requiere cambios adicionales. |
| `k8s/manifests.yaml` | Ya tenía `GEMINI_API_KEY` desde el secret `ti3-secrets`; no requiere cambios adicionales. |

### Endpoints de la API

| Método | Ruta | Responde |
|--------|------|----------|
| `POST` | `/ai/pdf-to-markdown` | Recibe archivo PDF (multipart). Devuelve `{markdown, filename, size_bytes, provider}`. Rechaza si no es PDF, si excede 10 MB, o si se supera el rate limit. |

### Cómo interactúa con el resto del proyecto

- Con **4_5_5**: `back/ai` ya existe; esta tarea agrega la integración real con Gemini sobre el scaffold existente.
- Con **front**: el front puede llamar a `POST /ai/pdf-to-markdown` vía el gateway de nginx (prefijo `/api-ai` por definir).
- Con **Notes/Drive**: por ahora no hay integración. El Markdown se devuelve al cliente; el guardado como nota en Notes/Drive se pospone.

### Verificación

- `go build ./...` + `go vet ./...` en `back/ai`: OK.
- Servidor arranca en `:8084` y responde `/health`, `/health/db`, `/ai/generate`, `/ai/pdf-to-markdown`.
- Validación de archivo: rechaza archivos no-PDF con 400, requiere campo `file`.
- Modo mock: si no hay `GEMINI_API_KEY`, usa `MockAIService` (respuesta simulada sin llamar a Gemini).
- Modo Gemini: si hay `GEMINI_API_KEY`, llama a la API REST de Gemini con reintentos y timeout.

### No entra

- Guardar el Markdown como nota en Notes (pospuesto).
- Subir el Markdown a Google Drive (pospuesto).
- Auth/JWT en el endpoint (por ahora público; se añadirá cuando sea necesario).
- SDK oficial de Google: se usa la API REST directamente con `net/http`.

## 4_5_X — Editar grupo: nombre y descripción (backend + front)

> **Nota:** Esta tarea pertenecía originalmente al ámbito de Benji (Social) y fue asignada a Martín por intercambio de tareas del sprint.

### Objetivo

Permitir que un administrador de grupo edite el nombre y la descripción de un grupo existente, tanto en el backend (Social) como en el front (Flutter).

### Alcance

| Parte | Qué se hace |
|-------|-------------|
| **Backend (Social)** | Endpoint `PATCH /groups/:id` que recibe `{name, description?}` y valida que el solicitante sea admin del grupo. |
| **Front (Flutter)** | Botón "Editar grupo" en `GroupDetailScreen` (solo visible para admins) que abre un diálogo con campos de texto para nombre y descripción. |

### Archivos creados

Ninguno nuevo (todo es modificación de existentes).

### Archivos modificados

| Archivo | Cambio |
|---------|--------|
| `back/social/internal/service/group_service.go` | Agregado `UpdateGroup` a la interfaz `GroupStore` y implementado en `GroupService` (valida UUID, normaliza nombre, llama al store). |
| `back/social/internal/repository/group_repository.go` | Agregado `UpdateGroup` en `GroupRepository` (UPDATE SQL con `FOR UPDATE`, valida que el usuario es admin vía `requireAdminTx`). |
| `back/social/internal/service/store_memory.go` | Agregado `UpdateGroup` en `MemoryGroupStore` para tests (valida admin vía `requireAdminLocked`). |
| `back/social/internal/handler/http/group_handler.go` | Agregado tipo `updateGroupRequest`, handler `EditGroup` y ruta `PATCH /groups/:id` en `RegisterRoutes`. |
| `front/lib/core/services/social_service.dart` | Agregado método `updateGroup({groupId, name, description})` que llama a `PATCH /groups/:id`. |
| `front/lib/features/groups/group_detail_screen.dart` | Agregado botón "Editar grupo" (ícono de lápiz) en el AppBar, visible solo cuando el usuario es admin. Abre `AlertDialog` con campos de nombre y descripción. |
| `documentation/docMartinSprint3.md` | Esta sección. |

### Endpoints de la API

| Método | Ruta | Body | Responde |
|--------|------|------|----------|
| `PATCH` | `/groups/:id` | `{name: string, description?: string}` | `200 {id, name, description}` / `403` si no es admin / `404` si no existe / `400` si nombre vacío |

### Cómo interactúa con el resto del proyecto

- Con **4_5_4** (Política solo-admins de Benji): el endpoint respeta el mismo modelo de permisos (solo admin puede editar).
- Con **front**: el botón solo aparece para administradores del grupo, consistente con otras acciones de administración (regenerar invite, kick, ban, etc.).
- Con **grupos existentes**: el cambio se refleja inmediatamente en `GET /groups/:id` y `GET /groups/me`.

### Verificación

- `go build ./...` + `go vet ./...` en `back/social`: OK.
- `go test ./...` en `back/social`: todos los tests pasan.
- `flutter analyze` en los archivos del front tocados: sin errores nuevos (solo warnings preexistentes).
- El endpoint reusa la validación de nombre existente (`utils.ValidateName`, `utils.NormalizeName`) y la autorización de admin existente (`requireAdminTx`).

### No entra

- Editar otros campos del grupo (owner, notes_restricted_to_staff, invite_token).
- Historial de cambios de nombre/descripción.
- Notificar a miembros cuando el grupo es editado.
