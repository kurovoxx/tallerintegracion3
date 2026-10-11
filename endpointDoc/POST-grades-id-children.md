# POST /grades/{id}/children

## Req
- Método: `POST`, ruta: `/grades/:id/children` (protegida, `back/infacadem/cmd/server/main.go:138`).
- Headers: `Authorization: Bearer <access_token>`, `Content-Type: application/json`.
- Body: igual que alta de nota (`name` + `weight` fracción 0–1 requeridos); fija `parent_grade_id` al `:id` y hereda su `subject_id`.

## Res
- `201`: `{"id":"<uuid>"}`.
- `400/401/404/409`: mismos códigos que `POST /subjects/{id}/grades` (404 si el padre es ajeno/inexistente).

## Flujo
- `back/infacadem/internal/handler/http/grades_handler.go` (`CreateChild`) → `back/infacadem/internal/service/grade_service.go` (`CreateChild`, lee el padre propio y hereda `subject_id`) → `back/infacadem/internal/repository/grade_repository.go` (`Create` con `ParentGradeID`).

## Tests
- `back/infacadem/internal/handler/http/grades_handler_test.go`: `TestGrades_FlujoCompleto` (no hay test dedicado de 404 cross-user para children: ver OBS-2 del veredicto).
- `back/infacadem/internal/service/grade_service_test.go`: `TestGrade_Hijos_Efectivo` (valor efectivo del padre = ponderado de hijos).
