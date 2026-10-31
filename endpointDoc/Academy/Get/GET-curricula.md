# GET /curricula

## Req
- Método: `GET`, ruta: `/curricula` (protegida, `back/infacadem/cmd/server/main.go:111`).
- Headers: `Authorization: Bearer <access_token>`. Sin body.

## Res
- `200`: `{"items":[{"id":"...","user_id":"...","name":"...","created_at":"..."}]}` (solo propias, `[]` nunca `null`).
- `401`: `{"error":{"code":"unauthorized|invalid_token|token_expired","message":"..."}}`.

## Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/academic/academic_api.dart:182` (`listCurricula`)
  - Pantalla: `front/lib/features/academic/curriculum_screen.dart:170` (`_loadData`)
- **Otros Microservicios:** No se consume por otros microservicios.

## Flujo
- `back/infacadem/internal/handler/http/curricula_handler.go` (`List`) → `back/infacadem/internal/service/curriculum_service.go` (`List`) → `back/infacadem/internal/repository/curriculum_repository.go` (`ListByUser`, `WHERE user_id=$1`); auth vía `back/infacadem/internal/middleware/auth_middleware.go`.

## Tests
- `back/infacadem/internal/handler/http/academic_handler_test.go`: `TestHandler_SinToken_401`, `TestHandler_FlujoCompleto`, `TestHandler_CrossUser_404`.
- `back/infacadem/internal/service/academic_service_test.go`: `TestCurriculum_CRUD`.
