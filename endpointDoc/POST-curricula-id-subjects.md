# POST /curricula/{id}/subjects

## Req
- Método: `POST`, ruta: `/curricula/:id/subjects` (protegida, `back/infacadem/cmd/server/main.go:116`).
- Headers: `Authorization: Bearer <access_token>`, `Content-Type: application/json`.
- Body: `{"name":"Cálculo"}` + opcionales `semester`, `credits`, `total_classes`, `min_attendance_pct`, `average_decimal_places` (0-2, default 2), `status` (enum, default `in_progress`).

## Res
- `201`: `{"id":"<uuid>"}`.
- `400`: `{"error":{"code":"bad_request|invalid_name|invalid_decimal_places|invalid_status|invalid_uuid","message":"..."}}`.
- `401`: `{"error":{"code":"unauthorized|invalid_token|token_expired","message":"..."}}`.
- `404`: `{"error":{"code":"not_found","message":"..."}}` (malla no existe o es ajena).

## Flujo
- `back/infacadem/internal/handler/http/subjects_handler.go` (`Create`) → `back/infacadem/internal/service/subject_service.go` (`Create`, defaults espejo del DDL) → `back/infacadem/internal/repository/subject_repository.go` (`Create`, `INSERT..SELECT..WHERE EXISTS` atómico con check de propiedad).

## Tests
- `back/infacadem/internal/handler/http/academic_handler_test.go`: `TestHandler_SinToken_401`, `TestHandler_FlujoCompleto`, `TestHandler_CrossUser_404`, `TestHandler_Validaciones_400`.
- `back/infacadem/internal/service/academic_service_test.go`: `TestSubject_CRUD_Validaciones`.
- `back/infacadem/internal/service/academic_integration_test.go`: `TestAcademicIntegration_CRUDCascada`.
