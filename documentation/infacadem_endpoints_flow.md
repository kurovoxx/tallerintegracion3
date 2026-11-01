# Documentación Completa de Endpoints: Microservicio Academic (`infacadem`)

Este documento consolida y estandariza **todos los endpoints del microservicio `infacadem`**, siguiendo la estructura técnica recomendada (Req, Res, Flujo Paso a Paso, Dependencias y Tests) e indicando **el archivo y número de línea exacto desde donde son consumidos en el Frontend (Flutter) o por otros microservicios**.

---

## Índice de Endpoints

### 1. Salud del Servicio
- [`GET /health`](#get-health) — Chequeo de salud del proceso HTTP
- [`GET /health/db`](#get-healthdb) — Chequeo de salud con conectividad a PostgreSQL

### 2. Mallas Curriculares (`curricula`)
- [`POST /curricula`](#post-curricula) — Crear malla curricular
- [`GET /curricula`](#get-curricula) — Listar mallas del usuario
- [`GET /curricula/:id`](#get-curriculaid) — Obtener malla por ID
- [`PATCH /curricula/:id`](#patch-curriculaid) — Actualizar nombre de malla
- [`DELETE /curricula/:id`](#delete-curriculaid) — Eliminar malla

### 3. Asignaturas / Materias (`subjects`)
- [`POST /curricula/:id/subjects`](#post-curriculaidsubjects) — Crear materia dentro de una malla
- [`GET /curricula/:id/subjects`](#get-curriculaidsubjects) — Listar materias de una malla
- [`GET /subjects/:id`](#get-subjectsid) — Obtener detalle de materia
- [`PATCH /subjects/:id`](#patch-subjectsid) — Actualizar campos de materia
- [`DELETE /subjects/:id`](#delete-subjectsid) — Eliminar materia

### 4. Horarios de Clases (`schedules`)
- [`POST /subjects/:id/schedules`](#post-subjectsidschedules) — Crear bloque horario de materia
- [`GET /subjects/:id/schedules`](#get-subjectsidschedules) — Listar bloques horarios de materia
- [`DELETE /schedules/:id`](#delete-schedulesid) — Eliminar bloque horario

### 5. Calificaciones y Notas (`grades`)
- [`POST /subjects/:id/grades`](#post-subjectsidgrades) — Crear evaluación principal (top-level)
- [`GET /subjects/:id/grades`](#get-subjectsidgrades) — Listar árbol de evaluaciones
- [`GET /grades/:id`](#get-gradesid) — Obtener evaluación por ID
- [`PATCH /grades/:id`](#patch-gradesid) — Actualizar nota, peso, nombre o estado
- [`DELETE /grades/:id`](#delete-gradesid) — Eliminar evaluación (en cascada a hijas)
- [`POST /grades/:id/children`](#post-gradesidchildren) — Crear sub-evaluación (hija)
- [`GET /subjects/:id/average`](#get-subjectsidaverage) — Obtener promedio ponderado y situación académica

### 6. Prerrequisitos de Asignaturas (`requirements`)
- [`POST /subjects/:id/requirements`](#post-subjectsidrequirements) — Agregar prerrequisito a una materia
- [`GET /subjects/:id/requirements`](#get-subjectsidrequirements) — Listar prerrequisitos de una materia
- [`DELETE /requirements/:id`](#delete-requirementsid) — Eliminar prerrequisito

### 7. Inasistencias y Asistencia (`absences` & `attendance`)
- [`POST /subjects/:id/absences`](#post-subjectsidabsences) — Registrar una inasistencia
- [`GET /subjects/:id/absences`](#get-subjectsidabsences) — Listar inasistencias de la materia
- [`GET /absences/:id`](#get-absencesid) — Obtener detalle de una inasistencia
- [`PATCH /absences/:id`](#patch-absencesid) — Modificar estado (justificada/fuente/tipo)
- [`DELETE /absences/:id`](#delete-absencesid) — Eliminar registro de inasistencia
- [`GET /subjects/:id/attendance`](#get-subjectsidattendance) — Obtener porcentaje y cálculo oficial de asistencia

### 8. Contenidos y Unidades Temáticas (`contents`)
- [`POST /subjects/:id/contents`](#post-subjectsidcontents) — Crear unidad de contenido
- [`GET /subjects/:id/contents`](#get-subjectsidcontents) — Listar contenidos ordenados por posición
- [`GET /contents/:id`](#get-contentsid) — Obtener unidad de contenido por ID
- [`PATCH /contents/:id`](#patch-contentsid) — Actualizar título, cuerpo o posición
- [`DELETE /contents/:id`](#delete-contentsid) — Eliminar unidad de contenido

### 9. Resumen y Dashboard Académico (`overview`)
- [`GET /academic/overview`](#get-academicoverview) — Dashboard global del estudiante
- [`GET /subjects/:id/detail`](#get-subjectsiddetail) — Vista integral de una materia

### 10. Contrato gRPC Inter-Servicio
- [`RPC AcademicService.GetSubject`](#rpc-academicservicegetsubject) — Resolución de materias para Notes

---

## 1. Salud del Servicio

### `GET /health`
Verifica si el proceso del microservicio está activo.

* **Invocación Front / Clúster:** 
  - Kubernetes Liveness / Readiness probe.
  - Front: no lo llama directamente.
* **Req:** `GET /health` (Sin auth).
* **Res:** `200 OK` `{"status":"ok","service":"infacadem"}`.
* **Flujo:** `cmd/server/main.go:61` responde directo en memoria sin tocar base de datos.
* **Tests:** `internal/handler/http/academic_handler_test.go:20`.

---

### `GET /health/db`
Verifica la conectividad activa con el pool de PostgreSQL.

* **Invocación Front / Clúster:** Kubernetes probe o monitoreo interno.
* **Req:** `GET /health/db` (Sin auth).
* **Res:** `200 OK` `{"status":"ok","database":"connected"}` | `503 Unavailable` `{"status":"error","database":"disconnected"}`.
* **Flujo:** `cmd/server/main.go:64-70` ejecuta `pool.Ping(ctx)`.
* **Tests:** `internal/handler/http/academic_handler_test.go:35`.

---

## 2. Mallas Curriculares (`curricula`)

### `POST /curricula`
Crea una nueva malla curricular para el usuario autenticado.

* **Llamada en el Front:**
  - Archivo: `front/lib/features/academic/academic_api.dart`
  - Línea: **L192–L204** (`createCurriculum`)
  - Vista invocadora: `front/lib/features/academic/curriculum_screen.dart`
* **Req:**
  - Headers: `Authorization: Bearer <token>`, `Content-Type: application/json`
  - Body: `{"name":"Ingeniería Civil Informática"}`
  - Validaciones: `name` requerido, 1 a 150 caracteres tras trim.
* **Res:**
  - `201 Created`: `{"id":"uuid"}`
  - `400 Bad Request`: `invalid_body`, nombre vacío o demasiado largo.
  - `401 Unauthorized`: token ausente o expirado.
* **Flujo Técnico:**
  1. **Handler:** `internal/handler/http/curricula_handler.go:44-59` (`Create`). Valida JWT y JSON.
  2. **Service:** `internal/service/curriculum_service.go:25-35`. Normaliza nombre y valida longitud.
  3. **Repository:** `internal/repository/curriculum_repository.go:20-30`. Ejecuta `INSERT INTO academic.curricula (user_id, name) VALUES ($1, $2) RETURNING id`.
* **Tests:** `internal/handler/http/academic_handler_test.go:50`.

---

### `GET /curricula`
Lista todas las mallas curriculares pertenecientes al usuario.

* **Llamada en el Front:**
  - Archivo: `front/lib/features/academic/academic_api.dart`
  - Línea: **L182–L190** (`listCurricula`)
  - Vista invocadora: `front/lib/features/academic/curriculum_screen.dart`
* **Req:** Headers: `Authorization: Bearer <token>`.
* **Res:**
  - `200 OK`: `[{"id":"uuid","name":"Malla 2024","created_at":"2026-10-30T..."}]`
* **Flujo Técnico:**
  1. **Handler:** `internal/handler/http/curricula_handler.go:61-73` (`List`).
  2. **Service:** `internal/service/curriculum_service.go:37-43`.
  3. **Repository:** `internal/repository/curriculum_repository.go:32-45`. `SELECT id, name, created_at FROM academic.curricula WHERE user_id = $1 ORDER BY created_at ASC`.
* **Tests:** `internal/handler/http/academic_handler_test.go:70`.

---

### `GET /curricula/:id`
Obtiene los datos de una malla curricular específica.

* **Req:** Path: `:id` (UUID de la malla). Auth: JWT Bearer.
* **Res:**
  - `200 OK`: `{"id":"uuid","name":"Malla 2024","created_at":"..."}`
  - `404 Not Found`: la malla no existe o pertenece a otro usuario (Zero-Knowledge).
* **Flujo Técnico:**
  1. `CurriculaHandler.Get` (`curricula_handler.go:75-92`) valida UUID.
  2. `CurriculumRepository.GetByID`: `SELECT ... WHERE id = $1 AND user_id = $2`.
* **Tests:** `internal/handler/http/academic_handler_test.go:90`.

---

### `PATCH /curricula/:id`
Actualiza el nombre de una malla curricular propia.

* **Req:** Body: `{"name":"Nuevo Nombre"}`. Auth: JWT Bearer.
* **Res:** `200 OK` `{"id":"uuid","name":"Nuevo Nombre"}` | `404 Not Found`.
* **Flujo Técnico:**
  1. `CurriculaHandler.Update` (`curricula_handler.go:94-115`).
  2. `CurriculumService.Update`: verifica ownership y actualiza vía `UPDATE academic.curricula SET name = $3 WHERE id = $1 AND user_id = $2`.
* **Tests:** `internal/handler/http/academic_handler_test.go:110`.

---

### `DELETE /curricula/:id`
Elimina una malla curricular y sus asignaturas asociadas en cascada.

* **Req:** Path: `:id` (UUID). Auth: JWT Bearer.
* **Res:** `204 No Content` | `404 Not Found`.
* **Flujo Técnico:**
  1. `CurriculaHandler.Delete` (`curricula_handler.go:117-133`).
  2. `CurriculumRepository.Delete`: `DELETE FROM academic.curricula WHERE id = $1 AND user_id = $2`.
* **Tests:** `internal/handler/http/academic_handler_test.go:130`.

---

## 3. Asignaturas / Materias (`subjects`)

### `POST /curricula/:id/subjects`
Registra una nueva asignatura asociada a una malla curricular.

* **Llamada en el Front:**
  - Archivo: `front/lib/features/academic/academic_api.dart`
  - Línea: **L229–L262** (`createSubject`)
  - Vista invocadora: `front/lib/features/academic/courses_screen.dart`
* **Req:**
  - Path: `:id` (UUID de la malla)
  - Headers: `Authorization: Bearer <token>`, `Content-Type: application/json`
  - Body:
    ```json
    {
      "name": "Cálculo I",
      "code": "INFO101",
      "status": "in_progress",
      "semester": 1,
      "credits": 6,
      "color": "#4A90E2",
      "min_grade": 40,
      "max_grade": 70,
      "passing_grade": 40
    }
    ```
* **Res:**
  - `201 Created`: `{"id":"uuid"}`
  - `400 Bad Request`: `invalid_body`, escala de notas incongruente (`min_grade > passing_grade`).
  - `404 Not Found`: la malla no existe o es de otro usuario.
* **Flujo Técnico:**
  1. **Handler:** `internal/handler/http/subjects_handler.go:48-75` (`Create`).
  2. **Service:** `internal/service/subject_service.go:40-65`. Verifica pertenencia de la malla y valida rango de notas (default 10 a 70, aprobación 40).
  3. **Repository:** `internal/repository/subject_repository.go:30-55`. `INSERT INTO academic.subjects (...) VALUES (...) RETURNING id`.
* **Tests:** `internal/handler/http/academic_handler_test.go:150`.

---

### `GET /curricula/:id/subjects`
Lista las asignaturas de una malla ordenadas por semestre.

* **Llamada en el Front:**
  - Archivo: `front/lib/features/academic/academic_api.dart`
  - Línea: **L216–L227** (`listSubjects`)
  - Vista invocadora: `front/lib/features/academic/curriculum_screen.dart`
* **Req:** Path: `:id` (UUID de la malla). Auth: JWT.
* **Res:** `200 OK`: Lista de materias con notas, créditos y color.
* **Flujo:** `SubjectHandler.ListByCurriculum` (`subjects_handler.go:77-94`) $\rightarrow$ `SubjectRepository.ListByCurriculum`.
* **Tests:** `internal/handler/http/academic_handler_test.go:175`.

---

### `GET /subjects/:id`
Obtiene los detalles de una materia.

* **Llamada en el Front:**
  - Archivo: `front/lib/features/academic/academic_api.dart` (en carga de detalle)
  - Invocado también por **gRPC**: `AcademicService.GetSubject` (Notes).
* **Req:** Path: `:id` (UUID). Auth: JWT.
* **Res:** `200 OK`: Datos completos de la materia | `404 Not Found`.
* **Flujo:** `subjects_handler.go:96-113` $\rightarrow$ `SubjectRepository.GetByID`.

---

### `PATCH /subjects/:id`
Actualiza parcialmente una asignatura (estado, nombre, semestre, notas de aprobación).

* **Llamada en el Front:**
  - Archivo: `front/lib/features/academic/academic_api.dart`
  - Línea: **L264–L278** (`patchSubject`)
* **Req:** Body: campos opcionales (`name`, `status`, `color`, `passing_grade`).
* **Res:** `200 OK` con la materia actualizada.
* **Flujo:** `subjects_handler.go:115-142` $\rightarrow$ `SubjectService.Update` (construye update dinámico con pgx).

---

### `DELETE /subjects/:id`
Elimina una asignatura junto con sus horarios, notas e inasistencias en cascada.

* **Req:** Path: `:id` (UUID). Auth: JWT.
* **Res:** `204 No Content` | `404 Not Found`.
* **Flujo:** `subjects_handler.go:144-160` $\rightarrow$ `DELETE FROM academic.subjects WHERE id = $1 AND user_id = $2`.

---

## 4. Horarios de Clases (`schedules`)

### `POST /subjects/:id/schedules`
Registra un bloque semanal de clases para una asignatura.

* **Llamada en el Front:**
  - Archivo: `front/lib/features/academic/academic_api.dart`
  - Línea: **L293–L326** (`createSchedule`) y **L328–L381** (`createSchedulesFanOut` para múltiples días)
  - Vista invocadora: `front/lib/features/academic/schedule_screen.dart`
* **Req:**
  - Path: `:id` (UUID de la materia)
  - Body:
    ```json
    {
      "day_of_week": 1,
      "start_time": "08:30:00",
      "end_time": "10:00:00",
      "classroom": "Lab-3",
      "type": "cátedra"
    }
    ```
  - Validaciones: `day_of_week` 1 (Lunes) a 7 (Domingo), `start_time < end_time`.
* **Res:** `201 Created` `{"id":"uuid"}` | `400 Bad Request` | `404 Not Found`.
* **Flujo Técnico:**
  1. `SchedulesHandler.Create` (`schedules_handler.go:42-70`).
  2. `ScheduleService.Create`: valida pertenencia de la materia y coherencia de horarios.
  3. `ScheduleRepository.Create`: inserta en `academic.schedules`.
* **Tests:** `internal/handler/http/academic_handler_test.go:210`.

---

### `GET /subjects/:id/schedules`
Obtiene la lista de bloques de horario de una materia.

* **Llamada en el Front:**
  - Archivo: `front/lib/features/academic/academic_api.dart`
  - Línea: **L280–L291** (`listSchedules`)
  - Vista invocadora: `front/lib/features/academic/schedule_screen.dart`
* **Req:** Path: `:id` (UUID). Auth: JWT.
* **Res:** `200 OK`: `[{"id":"uuid","day_of_week":1,"start_time":"08:30","end_time":"10:00",...}]`.

---

### `DELETE /schedules/:id`
Elimina un bloque horario específico.

* **Req:** Path: `:id` (UUID del horario). Auth: JWT.
* **Res:** `204 No Content` | `404 Not Found`.
* **Flujo:** `schedules_handler.go:94-110` $\rightarrow$ `ScheduleRepository.Delete`.

---

## 5. Calificaciones y Notas (`grades`)

### `POST /subjects/:id/grades`
Crea una evaluación principal (ej: "Certamen 1", "Tareas") en la estructura de calificaciones.

* **Llamada en el Front:**
  - Archivo: `front/lib/features/academic/academic_api.dart`
  - Línea: **L398–L425** (`createGrade`)
  - Vista invocadora: `front/lib/features/academic/grade_calculator_screen.dart`
* **Req:**
  - Body:
    ```json
    {
      "name": "Certamen 1",
      "score": 55,
      "weight": 0.30,
      "status": "graded",
      "is_final_exam": false
    }
    ```
  - Validaciones: `score` (escala 10-70 o null si pendiente), `weight` (0.0 a 1.0).
* **Res:** `201 Created` `{"id":"uuid"}` | `400 Bad Request`.
* **Flujo Técnico:**
  1. `GradesHandler.Create` (`grades_handler.go:44-72`).
  2. `GradeService.Create`: valida que la suma de pesos no supere el 100% (o advierte) y verifica propiedad de la materia.
  3. `GradeRepository.Create`: inserta en `academic.grades` con `parent_id = NULL`.
* **Tests:** `internal/handler/http/grades_handler_test.go:30`.

---

### `POST /grades/:id/children`
Crea una sub-evaluación subordinada a una nota padre (ej: "Pregunta 1" dentro de "Certamen 1").

* **Llamada en el Front:**
  - Archivo: `front/lib/features/academic/academic_api.dart`
  - Línea: **L427–L452** (`createGradeChild`)
  - Vista invocadora: `front/lib/features/academic/grade_calculator_screen.dart`
* **Req:** Path: `:id` (UUID de la evaluación padre). Body: `{name, score, weight}`.
* **Res:** `201 Created` `{"id":"uuid"}`.
* **Flujo:** `grades_handler.go:120-145` $\rightarrow$ `GradeRepository.CreateChild` (asigna `parent_id = :id`).

---

### `GET /subjects/:id/grades`
Retorna el árbol completo de calificaciones de una asignatura (padres con sus hijos anidados).

* **Llamada en el Front:**
  - Archivo: `front/lib/features/academic/academic_api.dart`
  - Línea: **L383–L396** (`listGrades`)
  - Vista invocadora: `front/lib/features/academic/grade_calculator_screen.dart`
* **Res:** `200 OK`: `[{"id":"uuid","name":"C1","weight":0.3,"score":55,"children":[...]}]`.
* **Flujo:** `grades_handler.go:74-90` $\rightarrow$ `GradeService.ListTree`.

---

### `PATCH /grades/:id`
Actualiza el puntaje, peso, nombre o estado de una evaluación.

* **Llamada en el Front:**
  - Archivo: `front/lib/features/academic/academic_api.dart`
  - Línea: **L454–L468** (`patchGrade`) y **L749** (`renameGrade`)
* **Req:** Body: campos opcionales `{score, weight, name, status}`.
* **Res:** `200 OK` con la evaluación actualizada.

---

### `DELETE /grades/:id`
Elimina una evaluación (y todas sus sub-notas hijas asociadas en cascada).

* **Llamada en el Front:**
  - Archivo: `front/lib/features/academic/academic_api.dart`
  - Línea: **L470–L484** (`deleteGrade`)
* **Res:** `204 No Content` | `404 Not Found`.

---

### `GET /subjects/:id/average`
Calcula el promedio ponderado actual y proyectado, y la situación de aprobación de la materia.

* **Llamada en el Front:**
  - Archivo: `front/lib/features/academic/academic_api.dart`
  - Línea: **L552–L570** (`getAverage`)
  - Vista invocadora: `front/lib/features/academic/grade_calculator_screen.dart`
* **Res:**
  ```json
  {
    "current_average": 52.5,
    "projected_average": 56.0,
    "required_final_grade": 40.0,
    "status": "in_progress",
    "is_passing": true
  }
  ```
* **Flujo:** `grades_handler.go:147-165` $\rightarrow$ `GradeService.CalculateAverage` (aplica ponderaciones de notas cerradas y simula notas pendientes).

---

## 6. Prerrequisitos de Asignaturas (`requirements`)

### `POST /subjects/:id/requirements`
Declara que la materia `:id` requiere tener aprobada previamente otra materia (`required_subject_id`).

* **Req:**
  - Path: `:id` (UUID de la materia dependiente)
  - Body: `{"required_subject_id":"uuid"}`
* **Res:** `201 Created` | `400 Bad Request` (evita dependencias circulares: A no puede requerir a A).
* **Flujo:** `requirements_handler.go:38-65` $\rightarrow$ `RequirementService.Create` $\rightarrow$ `INSERT INTO academic.subject_requirements`.

---

### `GET /subjects/:id/requirements`
Lista los prerrequisitos que deben cumplirse para cursar la asignatura.

* **Llamada en el Front:**
  - Archivo: `front/lib/features/academic/academic_api.dart`
  - Línea: **L572–L585** (`listRequirements`)
* **Res:** `200 OK`: `[{"id":"uuid","required_subject_id":"uuid","required_subject_name":"Cálculo I"}]`.

---

### `DELETE /requirements/:id`
Elimina un prerrequisito entre dos asignaturas.

* **Req:** Path: `:id` (UUID del vínculo). Auth: JWT.
* **Res:** `204 No Content`.

---

## 7. Inasistencias y Asistencia (`absences` & `attendance`)

### `POST /subjects/:id/absences`
Registra una inasistencia a clases de una asignatura.

* **Llamada en el Front:**
  - Archivo: `front/lib/features/academic/academic_api.dart`
  - Línea: **L602–L626** (`createAbsence`)
  - Vista invocadora: `front/lib/features/academic/attendance_screen.dart`
* **Req:**
  - Body:
    ```json
    {
      "date": "2026-10-15",
      "justified": false,
      "source": "cátedra",
      "type": "completa"
    }
    ```
* **Res:** `201 Created` `{"id":"uuid"}`.
* **Flujo:** `absences_handler.go:42-70` $\rightarrow$ `AbsenceService.Create` $\rightarrow$ `INSERT INTO academic.absences`.
* **Tests:** `internal/handler/http/absence_fields_test.go`.

---

### `GET /subjects/:id/absences`
Lista el historial de inasistencias de la materia.

* **Llamada en el Front:**
  - Archivo: `front/lib/features/academic/academic_api.dart`
  - Línea: **L587–L600** (`listAbsences`)
  - Vista invocadora: `front/lib/features/academic/attendance_screen.dart`
* **Res:** `200 OK`: `[{"id":"uuid","date":"2026-10-15","justified":false,...}]`.

---

### `GET /absences/:id`
Obtiene los detalles de una inasistencia específica.

* **Res:** `200 OK` | `404 Not Found`.

---

### `PATCH /absences/:id`
Actualiza el estado de una inasistencia (por ejemplo, justificar una falta).

* **Llamada en el Front:**
  - Archivo: `front/lib/features/academic/academic_api.dart`
  - Línea: **L628–L642** (`patchAbsence`)
* **Req:** Body: `{"justified": true, "source": "certificado médico"}`.
* **Res:** `200 OK`.

---

### `DELETE /absences/:id`
Elimina un registro de inasistencia errónea.

* **Llamada en el Front:**
  - Archivo: `front/lib/features/academic/academic_api.dart`
  - Línea: **L644–L657** (`deleteAbsence`)
* **Res:** `204 No Content`.

---

### `GET /subjects/:id/attendance`
Calcula y entrega el porcentaje de asistencia oficial del alumno y si está en riesgo de reprobación por faltas.

* **Llamada en el Front:**
  - Archivo: `front/lib/features/academic/academic_api.dart`
  - Línea: **L659–L676** (`getAttendance`)
  - Vista invocadora: `front/lib/features/academic/attendance_screen.dart`
* **Res:**
  ```json
  {
    "total_classes": 36,
    "attended_classes": 32,
    "unjustified_absences": 4,
    "justified_absences": 0,
    "attendance_percentage": 88.89,
    "min_required_percentage": 75.0,
    "is_at_risk": false
  }
  ```
* **Flujo:** `attendance_handler.go:35-55` $\rightarrow$ `AttendanceService.Calculate` (cruza los bloques semanales de `schedules`, el calendario del semestre y las faltas no justificadas de `absences`).
* **Tests:** `internal/handler/http/attendance_handler_test.go`.

---

## 8. Contenidos y Unidades Temáticas (`contents`)

### `POST /subjects/:id/contents`
Crea una unidad tematica o materia dentro de la asignatura.

* **Llamada en el Front:**
  - Archivo: `front/lib/features/academic/academic_api.dart`
  - Línea: **L693–L716** (`createContent`)
* **Req:** Body: `{"title":"Unidad 1: Límites y Continuidad","body":"Apuntes introductorios","position":1}`.
* **Res:** `201 Created` `{"id":"uuid"}`.
* **Flujo:** `contents_handler.go:40-68` $\rightarrow$ `ContentRepository.Create`.

---

### `GET /subjects/:id/contents`
Lista las unidades temáticas ordenadas secuencialmente por `position`.

* **Llamada en el Front:**
  - Archivo: `front/lib/features/academic/academic_api.dart`
  - Línea: **L678–L691** (`listContents`)
* **Res:** `200 OK`: `[{"id":"uuid","title":"Unidad 1","position":1,...}]`.

---

### `GET /contents/:id`
Obtiene una unidad temática por su ID.

* **Res:** `200 OK` | `404 Not Found`.

---

### `PATCH /contents/:id`
Modifica el título, cuerpo o reordena la posición de la unidad temática.

* **Llamada en el Front:**
  - Archivo: `front/lib/features/academic/academic_api.dart`
  - Línea: **L718–L732** (`patchContent`)
* **Res:** `200 OK`.

---

### `DELETE /contents/:id`
Elimina una unidad temática.

* **Llamada en el Front:**
  - Archivo: `front/lib/features/academic/academic_api.dart`
  - Línea: **L734–L747** (`deleteContent`)
* **Res:** `204 No Content`.

---

## 9. Resumen y Dashboard Académico (`overview`)

### `GET /academic/overview`
Provee una radiografía completa del rendimiento académico del estudiante para la pantalla de inicio / perfil.

* **Llamada en el Front:**
  - Archivo: `front/lib/core/services/overview_service.dart` o `academic_api.dart`
  - Vista: `front/lib/features/academic/profile_screen.dart`
* **Req:** Headers: `Authorization: Bearer <token>`.
* **Res:**
  ```json
  {
    "total_subjects": 6,
    "passed_subjects": 4,
    "in_progress_subjects": 2,
    "overall_gpa": 55.4,
    "credits_completed": 24,
    "current_schedules_today": [...]
  }
  ```
* **Flujo:** `overview_handler.go:35-55` $\rightarrow$ `OverviewService.GetStudentSummary` (agrega en una sola consulta mallas, notas cerradas y horario del día).
* **Tests:** `internal/handler/http/overview_handler_test.go`.

---

### `GET /subjects/:id/detail`
Retorna en una sola llamada agregada toda la información de una asignatura (detalles, horarios, notas, asistencia y contenidos).

* **Req:** Path: `:id` (UUID de la materia).
* **Res:** `200 OK`: JSON agregado con `subject`, `schedules`, `grades`, `attendance_summary` y `contents`.
* **Flujo:** `overview_handler.go:57-80` $\rightarrow$ `OverviewService.GetSubjectDetail`.

---

## 10. Contrato gRPC Inter-Servicio

### `RPC AcademicService.GetSubject`
Permite a otros microservicios (especialmente **`notes`**) verificar la existencia y pertenencia de una asignatura antes de vincular apuntes a ella.

* **Invocador Técnico:**
  - Microservicio: `notes` (`back/notes/internal/service/note_service.go` vía cliente gRPC `academicv1.NewAcademicServiceClient`).
  - Dirección K8s: `academic:9081` (puerto `GRPC_PORT`).
* **Req (Protobuf):**
  - Archivo Proto: `back/proto/infacadem/academic.proto`
  ```protobuf
  message GetSubjectRequest {
    string subject_id = 1;       // UUID de la materia (notes.notes.subject_id)
    string requester_user_id = 2; // user_id extraído del JWT en Notes
  }
  ```
* **Res (Protobuf):**
  - `GetSubjectResponse`: `{ exists: true, subject_id: "...", owner_user_id: "...", name: "Cálculo I", status: "in_progress" }`.
  - Códigos de Error:
    - `codes.NotFound`: Si la materia no existe o es de otro usuario (**Zero-Knowledge** estricto).
    - `codes.InvalidArgument`: Si el UUID está malformado.
    - `codes.Internal`: Falla de base de datos.
* **Flujo en Código:**
  1. Servidor gRPC en `back/infacadem/cmd/server/main.go:94–105` escuchando en TCP `:9081`.
  2. Implementación en `back/infacadem/internal/handler/grpc/academic_server.go:30–55` (`GetSubject`).
  3. Consulta `SubjectRepository.GetByID(ctx, subjectID, requesterUserID)`.
* **Tests:** `back/infacadem/internal/handler/grpc/academic_server_test.go:40–190`.
