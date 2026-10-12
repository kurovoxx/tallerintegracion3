# GET /subjects/{id}

## Req
- Método: `GET`, ruta: `/subjects/:id` (protegida, `back/infacadem/cmd/server/main.go:118`).
- Headers: `Authorization: Bearer <access_token>`. Sin body.

## Res
- `200`: `{"id":"...","curriculum_id":"...","name":"...","code":"MAT-101","professor":"...","room":"...","semester":null,"credits":10,...,"status":"..."}` (`code/professor/room` null si ausentes).
- `400`: `{"error":{"code":"invalid_uuid","message":"..."}}`.
- `401`: `{"error":{"code":"unauthorized|invalid_token|token_expired","message":"..."}}`.
- `404`: `{"error":{"code":"not_found","message":"..."}}` (no existe o su malla es ajena).

## Flujo
- `back/infacadem/internal/handler/http/subjects_handler.go` (`Get`) → `back/infacadem/internal/service/subject_service.go` (`Get`) → `back/infacadem/internal/repository/subject_repository.go` (`GetByID`, `JOIN academic.curricula` para propiedad).

## Tests
- `back/infacadem/internal/handler/http/academic_handler_test.go`: `TestHandler_SinToken_401`, `TestHandler_FlujoCompleto`, `TestHandler_CrossUser_404`.
- `back/infacadem/internal/service/academic_service_test.go`: `TestSubject_CRUD_Validaciones`.
