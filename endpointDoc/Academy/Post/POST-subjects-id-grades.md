# POST /subjects/{id}/grades

## Req
- Método: `POST`, ruta: `/subjects/:id/grades` (protegida, `back/infacadem/cmd/server/main.go:133`).
- Headers: `Authorization: Bearer <access_token>`, `Content-Type: application/json`.
- Body: `{"name":"Parcial 1","score":55,"weight":0.3}` + opcionales `status` (default `pending`), `scheduled_date` (`"YYYY-MM-DD"`), `topics`, `notify_discord`, `google_calendar_event_id`, `is_final_exam`. `weight` fracción 0–1 requerida; `score` 10–70 o null.

## Res
- `201`: `{"id":"<uuid>"}`.
- `400`: `{"error":{"code":"bad_request|invalid_name|invalid_weight|invalid_score|invalid_status|invalid_date|invalid_uuid","message":"..."}}`.
- `401`: `{"error":{"code":"unauthorized|invalid_token|token_expired","message":"..."}}`.
- `404`: `{"error":{"code":"not_found","message":"..."}}` (materia ajena/inexistente).
- `409`: `{"error":{"code":"conflict","message":"..."}}` (segundo `is_final_exam` en la materia).

## Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/academic/academic_api.dart:398` (`createGrade`)
  - Pantalla: `front/lib/features/academic/grade_calculator_screen.dart:310` (agregar evaluación)
- **Otros Microservicios:** No se consume por otros microservicios.

## Flujo
- `back/infacadem/internal/handler/http/grades_handler.go` (`Create`) → `back/infacadem/internal/service/grade_service.go` (`Create`, valida rangos/enums/final único) → `back/infacadem/internal/repository/grade_repository.go` (`Create`, `INSERT..SELECT..WHERE EXISTS` atómico).

## Tests
- `back/infacadem/internal/handler/http/grades_handler_test.go`: `TestGrades_FlujoCompleto`, `TestGrades_CrossUser_404`, `TestGrades_Validaciones_400`.
- `back/infacadem/internal/service/grade_service_test.go`: `TestGrade_Create_Validaciones`, `TestGrade_FinalUnico_409`.
