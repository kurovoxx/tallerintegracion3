# PATCH /curricula/{id}

## Req
- Método: `PATCH`, ruta: `/curricula/:id` (protegida, `back/infacadem/cmd/server/main.go:113`).
- Headers: `Authorization: Bearer <access_token>`, `Content-Type: application/json`.
- Body: `{"name":"Malla v2"}` (1-200 caracteres).

## Res
- `200`: recurso actualizado `{"id":"...","user_id":"...","name":"Malla v2","created_at":"..."}`.
- `400`: `{"error":{"code":"bad_request|invalid_name|invalid_uuid","message":"..."}}`.
- `401`: `{"error":{"code":"unauthorized|invalid_token|token_expired","message":"..."}}`.
- `404`: `{"error":{"code":"not_found","message":"..."}}` (no existe o es ajena).

## Flujo
- `back/infacadem/internal/handler/http/curricula_handler.go` (`Update`) → `back/infacadem/internal/service/curriculum_service.go` (`Update`) → `back/infacadem/internal/repository/curriculum_repository.go` (`UpdateName`, `UPDATE ... WHERE id=$1 AND user_id=$2 RETURNING ...`).

## Tests
- `back/infacadem/internal/handler/http/academic_handler_test.go`: `TestHandler_SinToken_401`, `TestHandler_FlujoCompleto`, `TestHandler_CrossUser_404`.
- `back/infacadem/internal/service/academic_service_test.go`: `TestCurriculum_CRUD`.
