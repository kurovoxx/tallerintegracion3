# Arquitectura del Sistema, Modelo de Datos e Infraestructura Técnica

Este documento consolida la arquitectura completa del proyecto, detallando decisiones de diseño, ciclo de vida de los datos, infraestructura Kubernetes, pruebas de carga y el estado real de la implementación tecnológica (comunicación entre microservicios, base de datos y protocolos).

---

## 1. Ciclo de Vida y Manejo de Notas (`notes`)

### 1.1 ¿Dónde se guarda el contenido y qué es el identificador?
El sistema opera bajo un enfoque **Local-First** y **almacenamiento delegado a Google Drive**:
* **En el Cliente (Flutter / Web / Móvil / Desktop):** 
  El usuario crea o edita la nota sin necesidad de conexión. La nota vive localmente en SQLite mediante **Drift** (`LocalNotesCompanion`). Cada nota tiene un UUID (`id`) asignado desde su creación local, un campo de versión (`version`) y un hash para control de concurrencia. Drift utiliza triggers y **FTS5 (Full-Text Search)** para permitir búsquedas instantáneas offline por palabra clave.
* **En el Servidor PostgreSQL (`notes.notes`):**
  PostgreSQL **NO guarda el cuerpo de texto ni el markdown del apunte**. Solo almacena la metadata:
  - `id` (UUID): Identificador único global de la nota.
  - `user_id` (UUID): Autor de la nota.
  - `subject_id` (UUID opcional): Asignatura académica vinculada.
  - `title` (VARCHAR 300): Título del apunte.
  - `external_file_id` (VARCHAR 255): El **ID del archivo en Google Drive**. En los primeros momentos de creación (offline o mientras el background worker procesa), este campo puede ser `NULL` y el estado pasa a `sync_status = 'pending_drive'`.
  - `visibility` ('public' | 'private').
  - `version` (BIGINT): Contador de versión para **Optimistic Locking** (control de concurrencia optimista en peticiones `PATCH`).
* **En Google Drive:**
  El contenido real del documento vive en el Drive del usuario como un archivo `.md`. El backend de `notes` interactúa con la Google Drive API mediante tokens OAuth gestionados por el microservicio `auth`.

### 1.2 ¿Qué ocurre si un usuario crea dos notas con el mismo nombre?
En PostgreSQL, la tabla `notes.notes` está definida de la siguiente manera:
```sql
CREATE TABLE notes.notes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL,
    subject_id uuid,
    title varchar(300) NOT NULL,
    external_file_id varchar(255),
    visibility varchar(20) NOT NULL DEFAULT 'private' CHECK (visibility IN ('public', 'private')),
    likes_count int NOT NULL DEFAULT 0,
    forked_from_note_id uuid REFERENCES notes.notes(id) ON DELETE SET NULL,
    sync_status varchar(30) NOT NULL DEFAULT 'synced',
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_notes_user_id ON notes.notes(user_id);
```
* **Comportamiento ante nombres duplicados:** 
  **No existe una restricción `UNIQUE (user_id, title)`**. Por ende, la base de datos **permite** que un usuario cree múltiples notas con exactamente el mismo título.
* **Diferenciación:** Cada nota tiene su propio `id` (UUID) inmutable e independiente, y en Google Drive se generan archivos separados con identificadores de Drive (`external_file_id`) distintos.
* **Idempotencia:** Lo que sí está protegido es la duplicación accidental por reintentos de red. Si el cliente envía un header `Idempotency-Key`, la tabla `notes.idempotency_keys` registra `(user_id, operation, idempotency_key)` evitando que un doble click o reintento genere dos notas idénticas en la base de datos.

---

## 2. Acceso a Base de Datos: ¿Qué pasó con `pgx` + `sqlc`?

Existía la intención inicial de usar **`sqlc`** junto a **`pgx/v5`** en todos los microservicios. Sin embargo, en el estado actual del repositorio coexisten dos estrategias:

1. **Microservicio `social` (Usa SQLC compilado):**
   - Configuración: `back/social/sqlc.yaml`.
   - Consultas SQL fuente: `back/social/db/queries/*.sql` (`sprint.sql`, `meetings.sql`, `todo.sql`, `hours.sql`, etc.).
   - Código Go generado: `back/social/internal/repository/sqlc/*.go`.
   - Implementación: El repositorio `SprintRepository` envuelve `*sqlc.Queries` pasándole el pool de conexiones `*pgxpool.Pool`.
2. **Microservicios `notes` y `auth` (Uso de `pgx/v5` con SQL nativo directo):**
   - No tienen queries compiladas con sqlc en este momento (las carpetas `sqlc/` solo contienen un `.gitkeep`).
   - Usan directamente la librería de alto rendimiento `github.com/jackc/pgx/v5/pgxpool`.
   - Las sentencias SQL (`INSERT ... RETURNING`, `UPDATE ... WHERE version = $1`, `SELECT`) se encuentran explícitamente escritas en archivos como `back/notes/internal/repository/note_repository.go` y `back/auth/internal/repository/user_repository.go`.
   - **Por qué:** Permitió mayor dinamismo para implementar transacciones complejas, manejo manual de locks optimistas (`version = expected_version`), colas de reconciliación (`notes.drive_reconciliation_queue`) y manejo directo de tipos JSONB sin requerir recompilar binarios de sqlc tras cada cambio menor de esquema.

---

## 3. Comunicación entre Microservicios: gRPC, Protobuf y Modo Dual (HTTP/gRPC)

### 3.1 Estado de gRPC y Protobuf en el Proyecto
El ecosistema gRPC y Protocol Buffers **SÍ está implementado y forma parte del diseño central entre servicios**. La aparente "ausencia" de `.proto` se debe a que la rama actual `kurovoxx/analisis` (derivada de `main`) no tenía aún incorporados los últimos PRs que residen en la rama **`Development`** (el tronco principal del Sprint 3 y entrega técnica).

En **`Development`**, la carpeta `back/proto/` contiene la suite completa de contratos Protocol Buffers gestionados con **Buf**:
- **`back/proto/infacadem/academic.proto` (`academic.v1`)**:
  - Define `AcademicService` con el RPC `GetSubject(GetSubjectRequest) returns (GetSubjectResponse)`.
  - Diseñado con principio *zero-knowledge*: si una asignatura no existe o no pertenece al solicitante (`requester_user_id`), el RPC retorna `NOT_FOUND` de forma indistinguible para evitar fugas de información.
  - El servidor gRPC se levanta en `infacadem` en el puerto **9081** (`GRPC_PORT`).
- **`back/proto/auth/v1/identity.proto` (`identity.v1`)**:
  - Define `IdentityService` (`GetUser`, `LookupUsers`, `VerifyToken`, `GetOAuthToken`, `ReportCalendarRevoked`).
  - Puerto gRPC **9080**. Se usa para que `notes` y `social` obtengan tokens OAuth de Google Drive / Calendar de forma segura.
- **`back/proto/social/v1/social.proto` (`social.v1`)**:
  - Define `SocialService` (`IsMember`, `IsAdmin`, `GetMembers`, `GetMemberEmails`, `PrepareAccountDeletion`, `AccountDeletionCleanup`).
  - Puerto gRPC **9083**.
- **`back/proto/notes/v1/notes.proto` (`notes.v1`)**:
  - Define `NotesService` con métodos de sincronización de apuntes, compartición en grupos y revocación / limpieza (`UnshareAll`).
  - Puerto gRPC **9082**.

### 3.2 Estrategia de Toolchain y Generación (`buf`)
Para evitar discrepancias entre versiones locales de `protoc`:
- Cada módulo proto posee su archivo de configuración `buf.yaml` y de generación `buf.gen.yaml` (o `generate.sh`).
- Se utiliza un Makefile con `make tools` y `make generate` que utiliza **Buf v1.59.0**, `protoc-gen-go` y `protoc-gen-go-grpc` ejecutados de forma hermética.
- **Los stubs generados (`*.pb.go` y `*_grpc.pb.go`) se commitean directamente** dentro de cada microservicio (por ejemplo: `back/infacadem/internal/gen/academicv1/` o `back/notes/internal/gen/notesv1/`) porque cada `Dockerfile` de servicio se compila de forma aislada sin acceso a la raíz del monorepo.

### 3.3 Arquitectura de Comunicación Dual (Dual-Serve: HTTP + gRPC)
Los microservicios implementan una estrategia **dual**:
1. **HTTP/REST (Gin):**
   - Atiende peticiones del **Frontend Flutter** a través del Reverse Proxy de Nginx (`/api-auth/`, `/api-notes/`, etc.).
   - Sirve como fallback y para clientes web.
2. **gRPC (HTTP/2 binario con Protobuf):**
   - Se utiliza para la **comunicación interna servicio-a-servicio**.
   - Los servicios escuchan concurrentemente ambos protocolos:
     - `auth`: HTTP en `8080`, gRPC en `9080`.
     - `infacadem`: HTTP en `8081`, gRPC en `9081`.
     - `notes`: HTTP en `8082`, gRPC en `9082`.
     - `social`: HTTP en `8083`, gRPC en `9083`.
   - **Autenticación interna gRPC:** Las llamadas entre servicios viajan autenticadas mediante metadata gRPC enviando la clave compartida (`x-internal-key: <INTERNAL_API_KEY>`), procesada por interceptores gRPC dedicados (`internal/interceptor/auth.go`).
   - **Degradación controlada y Fallback:** Servicios como `notes` admiten configuración flexible (`SOCIAL_GRPC_ADDR` vs `SOCIAL_SERVICE_URL`). Si la dirección gRPC está configurada (`social:9083`), se usa el cliente gRPC generado; en su defecto, recurre al adaptador HTTP REST.

---

## 4. Kubernetes y Autoscaling (HPA)

### 4.1 Entorno del Clúster
- **Clúster:** `pillan` (vcluster académico UCT).
- **Namespace:** `student-brojas`.
- **Kubeconfig:** `/home/kurovox/Downloads/estudiantes-brojas.kubeconfig`.

### 4.2 Arquitectura de Despliegue
1. **PostgreSQL en Clúster:** Se ejecuta como un pod propio (`service/postgres:5432`) respaldado por un PersistentVolumeClaim (PVC).
2. **Reverse Proxy / Frontend (Nginx):**
   - El contenedor del `front` incluye un Nginx configurado (`front/nginx.conf`) que atiende en el puerto 80.
   - Nginx actúa como **API Gateway**: expone la aplicación Web SPA y redirige rutas como `/api-auth/` a `auth:8080`, `/api-notes/` a `notes:8082` y `/api-social/` a `social:8083`.
   - Bloquea accesos externos a las rutas internas: `location /api-auth/internal/ { return 404; }`.
3. **Ingress:**
   - Dominio público: `https://ti3-brojas.dev.censei.cl`.
   - Conecta directo al Service `front:80`.

### 4.3 Configuración del Autoscaling (HPA - Horizontal Pod Autoscaler)
Los microservicios `notes`, `social` y `infacadem` tienen configurado un **HPA v2** activo:

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: notes # (idem social e infacadem)
  namespace: student-brojas
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: notes
  minReplicas: 1
  maxReplicas: 2
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 60
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 0
      selectPolicy: Max
      policies:
      - type: Pods
        value: 1
        periodSeconds: 15
    scaleDown:
      stabilizationWindowSeconds: 60
      selectPolicy: Max
      policies:
      - type: Pods
        value: 1
        periodSeconds: 15
```
* **Métrica objetivo:** Si el consumo de CPU supera el **60% del Request** asignado (`cpu requests: 50m`), Kubernetes escala automáticamente.
* **Escala rápida (Scale Up):** `stabilizationWindowSeconds: 0` permite crear un pod adicional en cuestión de segundos ante ráfagas de tráfico.
* **Desescalado suave (Scale Down):** `stabilizationWindowSeconds: 60` espera 1 minuto de tranquilidad antes de reducir pods para evitar *flapping*.
* **Rango:** Mínimo 1 pod, máximo 2 pods (ajustado por cuotas del clúster académico).

---

## 5. Pruebas de Carga (Load Testing) y Pruebas Unitarias

### 5.1 Load Generator: Fortio In-Cluster
Para las pruebas de estrés se utiliza **Fortio** (`fortio/fortio`), ejecutado directamente dentro del clúster de Kubernetes mediante el script:
[`scripts/fortio-kanban.sh`](file:///home/kurovox/orca/workspaces/ti3/analisis/scripts/fortio-kanban.sh)

**¿Cómo funciona la prueba?**
1. **Autenticación Previa:** El script hace login vía `POST /auth/login` contra el Ingress público o puerto local, obteniendo un JWT Bearer Token válido.
2. **Ejecución In-Cluster:** Lanza un pod efímero de Fortio en Kubernetes (`kubectl run fortio-kanban-... --rm -i --image=fortio/fortio`).
3. **Target:** Pega directamente al endpoint interno del Kanban `http://social:8083/groups/{id}/sprint-sheet`.
4. **Parámetros configurables:**
   - Concurrencia (`-c`, ej: 4 hilos).
   - QPS (`-qps`, ej: 10 consultas por segundo).
   - Duración (`-t`, ej: 30s).
5. **Monitoreo Simultáneo:** El script levanta una ventana terminal secundaria con `kubectl logs -f deployment/social` para observar la latencia y la respuesta en tiempo real mientras el HPA monitorea el consumo de CPU.

### 5.2 Testing Automatizado
- **Backend (Go):** Tests unitarios e integración en cada carpeta de servicio (ej: `note_handler_test.go`, `drive_permissions_test.go`, `group_repository_integration_test.go`). Se ejecutan con `go test ./...`.
- **Frontend (Flutter):** Suite exhaustiva de pruebas de widgets y estado (`front/test/`), incluyendo aislamiento local con Drift (`local_notes_isolation_test.dart`), recuperación de contraseña y sincronización con Drive. Se ejecutan con `flutter test`.

---

## 6. Código de DBDiagram (DBML Actualizado)

A continuación se presenta la especificación completa en lenguaje **DBML** lista para copiar y pegar en [dbdiagram.io](https://dbdiagram.io) para visualizar el modelo relacional completo de la plataforma:

```dbml
// =====================================================
// DBDiagram.io - Schema Plataforma de Estudio (TI3)
// =====================================================

Table identity_users as "identity.users" {
  id uuid [pk, default: `gen_random_uuid()`]
  email varchar(255) [unique, not null]
  password_hash varchar(255) [not null]
  created_at timestamptz [not null, default: `now()`]
}

Table identity_refresh_tokens as "identity.refresh_tokens" {
  id uuid [pk, default: `gen_random_uuid()`]
  user_id uuid [not null, ref: > identity_users.id]
  token_hash varchar(255) [not null]
  expires_at timestamptz [not null]
  revoked boolean [not null, default: false]
  created_at timestamptz [not null, default: `now()`]
}

Table identity_password_resets as "identity.password_resets" {
  user_id uuid [pk, ref: - identity_users.id]
  code_hash varchar(255) [not null]
  expires_at timestamptz [not null]
  requested_at timestamptz [not null, default: `now()`]
  attempts integer [not null, default: 0]
}

Table identity_profiles as "identity.profiles" {
  user_id uuid [pk, ref: - identity_users.id]
  display_name varchar(150)
  handle varchar(50) [unique]
  bio text
  avatar_url varchar(500)
  career varchar(100)
  university varchar(100)
  updated_at timestamptz [not null, default: `now()`]
}

Table identity_oauth_connections as "identity.oauth_connections" {
  id uuid [pk, default: `gen_random_uuid()`]
  user_id uuid [not null, ref: > identity_users.id]
  provider varchar(50) [not null] // 'google_drive', 'google_calendar'
  encrypted_access_token text [not null]
  encrypted_refresh_token text [not null]
  token_expires_at timestamptz [not null]
  scope text [not null]
  created_at timestamptz [not null, default: `now()`]
  updated_at timestamptz [not null, default: `now()`]
}

// =====================================================
// NOTES SERVICE
// =====================================================

Table notes_notes as "notes.notes" {
  id uuid [pk, default: `gen_random_uuid()`]
  user_id uuid [not null]
  subject_id uuid
  title varchar(300) [not null]
  external_file_id varchar(255) // ID en Google Drive
  visibility varchar(20) [not null, default: 'private'] // 'public' | 'private'
  likes_count int [not null, default: 0]
  forked_from_note_id uuid [ref: > notes_notes.id]
  sync_status varchar(30) [not null, default: 'synced']
  version bigint [not null, default: 1] // Optimistic Lock
  created_at timestamptz [not null, default: `now()`]
  updated_at timestamptz [not null, default: `now()`]
}

Table notes_attachments as "notes.note_attachments" {
  id uuid [pk, default: `gen_random_uuid()`]
  note_id uuid [not null, ref: > notes_notes.id]
  external_file_id varchar(255) [not null]
  file_url varchar(500) [not null]
  file_type varchar(50) [not null]
  file_size_bytes bigint
  is_inline boolean [not null, default: false]
  created_at timestamptz [not null, default: `now()`]
}

Table notes_saved as "notes.saved_notes" {
  id uuid [pk, default: `gen_random_uuid()`]
  user_id uuid [not null]
  note_id uuid [not null, ref: > notes_notes.id]
  saved_at timestamptz [not null, default: `now()`]
}

Table notes_likes as "notes.note_likes" {
  id uuid [pk, default: `gen_random_uuid()`]
  note_id uuid [not null, ref: > notes_notes.id]
  user_id uuid [not null]
  created_at timestamptz [not null, default: `now()`]
}

Table notes_shared as "notes.shared_notes" {
  id uuid [pk, default: `gen_random_uuid()`]
  note_id uuid [not null, ref: > notes_notes.id]
  group_id uuid [not null, ref: > social_groups.id]
  is_admin_note boolean [not null, default: false]
  access_mode varchar(20) [not null, default: 'link'] // 'link' | 'restricted'
  shared_at timestamptz [not null, default: `now()`]
}

Table notes_reconciliation as "notes.drive_reconciliation_queue" {
  id uuid [pk, default: `gen_random_uuid()`]
  operation text [not null]
  note_id uuid
  attachment_id uuid
  external_file_id text
  status text [not null, default: 'pending']
  retry_count integer [not null, default: 0]
  next_attempt_at timestamptz [not null, default: `now()`]
}

// =====================================================
// SOCIAL SERVICE
// =====================================================

Table social_groups as "social.groups" {
  id uuid [pk, default: `gen_random_uuid()`]
  name varchar(150) [not null]
  description text
  created_by uuid [not null]
  created_at timestamptz [not null, default: `now()`]
}

Table social_group_members as "social.group_members" {
  id uuid [pk, default: `gen_random_uuid()`]
  group_id uuid [not null, ref: > social_groups.id]
  user_id uuid [not null]
  role varchar(20) [not null, default: 'member'] // 'admin' | 'member'
  joined_at timestamptz [not null, default: `now()`]
}

Table social_sprints as "social.sprints" {
  id uuid [pk, default: `gen_random_uuid()`]
  group_id uuid [not null, ref: > social_groups.id]
  name varchar(100) [not null]
  start_date date [not null]
  end_date date [not null]
  created_at timestamptz [not null, default: `now()`]
}

Table social_sprint_tasks as "social.sprint_tasks" {
  id uuid [pk, default: `gen_random_uuid()`]
  sprint_id uuid [not null, ref: > social_sprints.id]
  title varchar(200) [not null]
  description text
  status varchar(20) [not null, default: 'todo'] // 'todo'|'in_progress'|'done'
  assigned_to uuid
  estimated_hours numeric(5,2)
  actual_hours numeric(5,2)
  created_at timestamptz [not null, default: `now()`]
  updated_at timestamptz [not null, default: `now()`]
}

Table social_meetings as "social.meetings" {
  id uuid [pk, default: `gen_random_uuid()`]
  group_id uuid [not null, ref: > social_groups.id]
  title varchar(200) [not null]
  start_time timestamptz [not null]
  end_time timestamptz [not null]
  created_by uuid [not null]
  google_event_id varchar(255)
  created_at timestamptz [not null, default: `now()`]
}
```
