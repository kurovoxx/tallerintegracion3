# POST /subjects/{id}/schedules

## Req
- Método: `POST`, ruta: `/subjects/:id/schedules` (protegida, `back/infacadem/cmd/server/main.go:122`).
- Headers: `Authorization: Bearer <access_token>`, `Content-Type: application/json`.
- Body: `{"day_of_week":2,"start_time":"10:00","end_time":"11:30","room":"Lab-3"}` (`day_of_week` 0-6 requerido; horas `"HH:MM"` o `"HH:MM:SS"`, `end_time > start_time`; opcionales `modality` y `room` varchar libre; vacío/whitespace = NULL, `room` se recorta con `TrimSpace`). Si se omite `room`, el front usa la sala del curso como fallback (no lo resuelve el backend).

## Res
- `201`: `{"id":"<uuid>"}`.
- `400`: `{"error":{"code":"bad_request|invalid_day_of_week|invalid_time_format|invalid_time_range|invalid_uuid","message":"..."}}`.
- `401`: `{"error":{"code":"unauthorized|invalid_token|token_expired","message":"..."}}`.
- `404`: `{"error":{"code":"not_found","message":"..."}}` (materia no existe o es ajena).

## Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/academic/academic_api.dart:293` (`createSchedule`) y `L328` (`createSchedulesFanOut`)
  - Pantalla: `front/lib/features/academic/schedule_screen.dart:360` (crear horario)
- **Otros Microservicios:** No se consume por otros microservicios.

## Flujo
- `back/infacadem/internal/handler/http/schedules_handler.go` (`Create`, exige `day_of_week` no nulo) → `back/infacadem/internal/service/schedule_service.go` (`Create`, parsea horas y valida rango) → `back/infacadem/internal/repository/schedule_repository.go` (`Create`, `INSERT..SELECT..WHERE EXISTS` atómico vía subjects→curricula).

## Tests
- `back/infacadem/internal/handler/http/academic_handler_test.go`: `TestHandler_SinToken_401`, `TestHandler_FlujoCompleto`, `TestHandler_CrossUser_404`, `TestHandler_Validaciones_400` (día 7/-1/ausente, end==start, end<start, hora mala).
- `back/infacadem/internal/handler/http/subject_fields_test.go`: `TestHandler_ScheduleRoom_Roundtrip` (POST con `"room":"  Lab-3 "` y sin room; GET devuelve `"room":"Lab-3"` y `"room":null`).
- `back/infacadem/internal/service/academic_service_test.go`: `TestSchedule_CRUD_Validaciones`.
- `back/infacadem/internal/service/subject_fields_test.go`: `TestSchedule_Room_Create` (trim, propagación al repo y room vacío/whitespace → NULL).
- `back/infacadem/internal/service/academic_integration_test.go`: `TestAcademicIntegration_CRUDCascada` (room persistida contra Postgres real).
