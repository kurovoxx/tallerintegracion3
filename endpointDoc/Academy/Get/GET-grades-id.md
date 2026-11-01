# GET /grades/{id}

## Req
- Método: `GET`, ruta: `/grades/:id` (protegida, `back/infacadem/cmd/server/main.go:135`).
- Headers: `Authorization: Bearer <access_token>`. Sin body.

## Res
- `200`: `{"id":"...","subject_id":"...","parent_grade_id":null,"is_final_exam":false,"status":"...","name":"...","score":55,"weight":0.3,"scheduled_date":null,...,"created_at":"..."}`.
- `400`: `{"error":{"code":"invalid_uuid","message":"..."}}`.
- `401`: `{"error":{"code":"unauthorized|invalid_token|token_expired","message":"..."}}`.
- `404`: `{"error":{"code":"not_found","message":"..."}}` (ajena/inexistente).

## Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/academic/academic_api.dart` (vía listGrades)
- **Otros Microservicios:** No se consume por otros microservicios.

## Flujo
- `back/infacadem/internal/handler/http/grades_handler.go` (`Get`) → `back/infacadem/internal/service/grade_service.go` (`Get`) → `back/infacadem/internal/repository/grade_repository.go` (`GetByID`, `JOIN` subjects→curricula).

## Tests
- `back/infacadem/internal/handler/http/grades_handler_test.go`: `TestGrades_FlujoCompleto`, `TestGrades_CrossUser_404`.
- `back/infacadem/internal/service/grade_service_test.go`: `TestGrade_Create_Validaciones`.
