# POST /subjects/{id}/requirements

## Req
- Método: `POST`, ruta: `/subjects/:id/requirements` (protegida, `back/infacadem/cmd/server/main.go:141`).
- Headers: `Authorization: Bearer <access_token>`, `Content-Type: application/json`.
- Body: `{"checks_final_average":true,"comparator":">=","threshold_value":4.0,"action":"force_exam"}` o con `source_grade_id` (nota de la misma materia) en vez de `checks_final_average`. Fuente XOR (exactamente una). `threshold_value` en escala 1.0–7.0.

## Res
- `201`: `{"id":"<uuid>"}`.
- `400`: `{"error":{"code":"invalid_requirement|invalid_comparator|invalid_action|invalid_uuid","message":"..."}}` (XOR violado, fuente de otra materia/ajena, enum inválido, threshold ausente).
- `401` / `404` (materia ajena): códigos habituales.

## Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/academic/academic_api.dart` (`createRequirement`)
- **Otros Microservicios:** No se consume por otros microservicios.

## Flujo
- `back/infacadem/internal/handler/http/requirements_handler.go` (`Create`) → `back/infacadem/internal/service/requirement_service.go` (`Create`, XOR + `GradeSubjectOf` misma materia) → `back/infacadem/internal/repository/requirement_repository.go` (`Create` + `GradeSubjectOf`).

## Tests
- `back/infacadem/internal/handler/http/grades_handler_test.go`: `TestGrades_Average_Requisitos_HTTP`.
- `back/infacadem/internal/service/grade_service_test.go`: `TestRequirement_Validaciones`, `TestRequirement_FuentePendiente_NoDispara`.
