# GET /curricula/{id}/subjects

## Req
- Método: `GET`, ruta: `/curricula/:id/subjects` (protegida, `back/infacadem/cmd/server/main.go:117`).
- Headers: `Authorization: Bearer <access_token>`. Sin body.

## Res
- `200`: `{"items":[...]}` (materias de la malla con `code/professor/room` — null si ausentes; `[]` nunca `null`).
- `400`: `{"error":{"code":"invalid_uuid","message":"..."}}`.
- `401`: `{"error":{"code":"unauthorized|invalid_token|token_expired","message":"..."}}`.
- `404`: `{"error":{"code":"not_found","message":"..."}}` (malla no existe o es ajena).

## Flujo
- `back/infacadem/internal/handler/http/subjects_handler.go` (`ListByCurriculum`) → `back/infacadem/internal/service/subject_service.go` (`List`, 404 si no propia) → `back/infacadem/internal/repository/subject_repository.go` (`ListByCurriculum`, check `EXISTS` de propiedad + `SELECT ... ORDER BY name, id`).

## Tests
- `back/infacadem/internal/handler/http/academic_handler_test.go`: `TestHandler_SinToken_401`, `TestHandler_FlujoCompleto`, `TestHandler_CrossUser_404`.
- `back/infacadem/internal/service/academic_service_test.go`: `TestSubject_CRUD_Validaciones`.
