# GET /subjects/{id}/requirements

## Req
- Método: `GET`, ruta: `/subjects/:id/requirements` (protegida, `back/infacadem/cmd/server/main.go:142`).
- Headers: `Authorization: Bearer <access_token>`. Sin body.

## Res
- `200`: `{"items":[{"id":"...","subject_id":"...","source_grade_id":null,"checks_final_average":true,"comparator":">=","threshold_value":4.0,"action":"force_exam","created_at":"..."}]}`.
- `400/401/404`: `invalid_uuid` / auth / materia ajena.

## Flujo
- `back/infacadem/internal/handler/http/requirements_handler.go` (`List`) → `back/infacadem/internal/service/requirement_service.go` (`List`) → `back/infacadem/internal/repository/requirement_repository.go` (`ListBySubject`, check de propiedad + orden `created_at, id`).

## Tests
- `back/infacadem/internal/handler/http/grades_handler_test.go`: `TestGrades_Average_Requisitos_HTTP`.
- `back/infacadem/internal/service/grade_service_test.go`: `TestRequirement_Validaciones`.
