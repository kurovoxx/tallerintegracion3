# Base de Datos en el Proyecto: `sqlc + pgx` vs `pgx` Nativo

Este documento explica en detalle cómo los microservicios se comunican con PostgreSQL, por qué coexisten dos estrategias en el repositorio, la comparativa de rendimiento y cómo se estructuran las llamadas en el código.

---

## 1. El Estado Real de la Base de Datos en el Repositorio

Al revisar la rama principal del proyecto (`Development`), encontramos una aparente inconsistencia en el acceso a datos:

| Microservicio | Archivos `.sql` | `sqlc.yaml` | Código compilado en `repository/sqlc/` | Estrategia Real en Código |
|---|:---:|:---:|:---:|---|
| **`social`** | **SÍ** (`back/social/db/queries/*.sql`) | **SÍ** | **SÍ** (`db.go`, `sprint.sql.go`, etc.) | **`sqlc` compilado** alimentado por `pgxpool.Pool` |
| **`notes`** | **SÍ** (`back/notes/db/queries/notes.sql`) | **NO** | **NO** (solo `.gitkeep`) | **`pgx/v5` nativo directo** (SQL manual en Go) |
| **`auth`** | **NO** (solo `.gitkeep`) | **NO** | **NO** (solo `.gitkeep`) | **`pgx/v5` nativo directo** |
| **`infacadem`**| **NO** (solo `.gitkeep`) | **NO** | **NO** (solo `.gitkeep`) | **`pgx/v5` nativo directo** |

---

## 2. ¿Para qué sirven los archivos `.sql` y qué rol cumple cada herramienta?

Los archivos `.sql` de las carpetas `db/queries/` **no son de `pgx`, son archivos de entrada para `sqlc`**:
- **Los `.sql` (`back/social/db/queries/*.sql`):** Son consultas SQL estándar de PostgreSQL con anotaciones especiales (ej: `-- name: GetSheetByID :one`) que le indican a `sqlc` el nombre de la función y si espera una fila (`:one`), una lista (`:many`) o solo ejecutar (`:exec`).
- **`sqlc` (el generador/traductor):** Lee esos archivos `.sql` y el esquema de la base de datos, y **genera código Go tipado que invoca a la librería `pgx`**.
- **`pgx` (`github.com/jackc/pgx/v5`):** Es el driver de red que realmente abre la conexión TCP hacia PostgreSQL, ejecuta los queries y transfiere los datos.

---

## 3. Flujo de Llamadas en el Código

### Opción A: Con `sqlc + pgx` (Caso real de `social`)

1. **Definición de la consulta:**  
   En `back/social/db/queries/sprint.sql`:
   ```sql
   -- name: GetSheetByID :one
   SELECT id, group_id, name, start_date, end_date, created_at
   FROM social.sprint_sheets
   WHERE id = $1;
   ```

2. **Código generado automáticamente por `sqlc`:**  
   En `back/social/internal/repository/sqlc/sprint.sql.go`:
   ```go
   const getSheetByID = `-- name: GetSheetByID :one
   SELECT id, group_id, name, start_date, end_date, created_at
   FROM social.sprint_sheets
   WHERE id = $1
   `

   func (q *Queries) GetSheetByID(ctx context.Context, id pgtype.UUID) (SocialSprintSheet, error) {
       row := q.db.QueryRow(ctx, getSheetByID, id) // <-- sqlc usa pgx por debajo
       var i SocialSprintSheet
       err := row.Scan(
           &i.ID,
           &i.GroupID,
           &i.Name,
           &i.StartDate,
           &i.EndDate,
           &i.CreatedAt,
       )
       return i, err
   }
   ```

3. **Llamada desde el Repositorio de la aplicación:**  
   En `back/social/internal/repository/sprint_repository.go`:
   ```go
   type SprintRepository struct {
       pool    *pgxpool.Pool
       queries *sqlc.Queries
   }

   func NewSprintRepository(pool *pgxpool.Pool) *SprintRepository {
       return &SprintRepository{
           pool:    pool,
           queries: sqlc.New(pool), // Se inyecta el pool de pgx a sqlc
       }
   }

   func (r *SprintRepository) ObtenerSprint(ctx context.Context, sprintID pgtype.UUID) (*sqlc.SocialSprintSheet, error) {
       // Llamada a método fuertemente tipado:
       sheet, err := r.queries.GetSheetByID(ctx, sprintID)
       if err != nil {
           return nil, err
       }
       return &sheet, nil
   }
   ```

---

### Opción B: Con `pgx` Nativo Directo (Caso real de `notes`)

En `notes`, el archivo `notes.sql` quedó como boceto y no se generó `sqlc`. La llamada se hace directamente con strings SQL dentro del archivo Go en `back/notes/internal/repository/note_repository.go`:

```go
func (r *NoteRepository) GetByID(ctx context.Context, db DBTX, id string) (*model.Note, error) {
    if db == nil {
        db = r.pool
    }
    query := `
        SELECT id, user_id, subject_id, title, external_file_id, visibility, likes_count, sync_status, version, created_at, updated_at
        FROM notes.notes 
        WHERE id = $1
    `
    var n model.Note
    // Llamada directa y mapeo manual en pgx:
    err := db.QueryRow(ctx, query, id).Scan(
        &n.ID, &n.UserID, &n.SubjectID, &n.Title, &n.ExternalFileID,
        &n.Visibility, &n.LikesCount, &n.SyncStatus, &n.Version,
        &n.CreatedAt, &n.UpdatedAt,
    )
    if err != nil {
        return nil, err
    }
    return &n, nil
}
```

---

## 4. Comparativa de Velocidad y Rendimiento

> **¿Cuál es más rápido?**  
> **Tienen exactamente la misma velocidad (0% de diferencia en runtime).**

`sqlc` no es un ORM pesado ni compila SQL a código máquina. Lo único que hace es escribir el código Go que harías tú a mano.
- Ambos usan el mismo driver de red (`pgx/v5`).
- Ambos usan `pool.QueryRow(...)` y `row.Scan(...)`.
- Ninguno usa reflexión (`reflect`) en tiempo de ejecución.

### Tabla Comparativa

| Aspecto | `sqlc + pgx` | `pgx` Nativo |
|---|---|---|
| **Latencia / QPS** | Máxima velocidad posible en Go. | Máxima velocidad posible en Go. |
| **Type Safety** | Valida sintaxis y tipos en tiempo de compilación. Si la BD cambia, no compila. | Errores de tipo o columnas rotas solo se ven al ejecutar en runtime. |
| **Consultas Dinámicas (PATCH)** | Rígido. Difícil de concatenar filtros opcionales. | Muy flexible. Permite armar `WHERE` o `SET` concatenando strings en Go. |
| **Mantenimiento del código** | Cero código repetitivo de mapeo manual (`Scan`). | Requiere escribir `Scan(&a, &b, ...)` campo por campo en cada consulta. |

---

## 5. ¿Está justificada la diferencia en el proyecto?

Existe una justificación pragmática según el dominio de cada microservicio:

1. **`social` encaja perfecto con `sqlc`:**  
   Maneja un modelo altamente relacional y estructurado (grupos, miembros, sprints, tareas kanban, horas y reuniones). Las consultas son estables y predecibles. `sqlc` evita tener que mapear a mano decenas de campos y garantiza consistencia en todo el modelo de colaboración.

2. **`notes` requería la flexibilidad de `pgx` nativo:**  
   Notes es un motor de sincronización Local-First hacia Google Drive. Implementa:
   - **PATCH dinámico:** Actualiza únicamente los campos presentes en el payload.
   - **Optimistic Locking:** Obliga a incrementar `version = version + 1` y condicionar `WHERE version = $expected_version`.
   - **Transacciones heterogéneas:** Toca notas, llaves de idempotencia (`idempotency_keys`) y colas asíncronas de Drive (`drive_reconciliation_queue`) en una sola transacción `pgx.Tx`. Intentar encajar esto en `sqlc` agregaba fricción innecesaria.

3. **`auth` e `infacadem`:**  
   En estos dos servicios la razón fue velocidad de desarrollo e independencia del equipo, optando directamente por `pgx` nativo para sus consultas básicas.
