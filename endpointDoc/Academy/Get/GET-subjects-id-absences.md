# GET /subjects/{id}/absences

## Req
- Método: `GET`, ruta: `/subjects/:id/absences` (protegida, `back/infacadem/cmd/server/main.go:149`).
- Headers: `Authorization: Bearer <access_token>`. Sin body.

## Res
- `200`: `{"items":[{"id":"...","subject_id":"...","date":"2026-10-05","justified":false,"source":"self","type":"Teoría","created_at":"..."}]}` (orden `date ASC, id ASC`; `source/type` null si ausentes; `[]` nunca `null`).
- `400/401/404`: `invalid_uuid` / auth / materia ajena.

## Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/academic/academic_api.dart:587` (`listAbsences`)
  - Pantalla: `front/lib/features/academic/attendance_screen.dart:647` (`_loadData`)
- **Otros Microservicios:** No se consume por otros microservicios.

## Flujo
- `back/infacadem/internal/handler/http/absences_handler.go` (`ListBySubject`, fecha serializada `YYYY-MM-DD`) → `back/infacadem/internal/service/absence_service.go` (`List`) → `back/infacadem/internal/repository/absence_repository.go` (`ListBySubject`, check de propiedad + orden por fecha).

## Tests
- `back/infacadem/internal/handler/http/attendance_handler_test.go`: `TestAbsences_FlujoCompleto`, `TestAbsences_CrossUser_404`.
- `back/infacadem/internal/service/attendance_service_test.go`: `TestAbsence_CRUD_Duplicado`.
