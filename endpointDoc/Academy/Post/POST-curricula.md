# POST /curricula

## Req
- Método: `POST`, ruta: `/curricula` (protegida, `back/infacadem/cmd/server/main.go:110`).
- Headers: `Authorization: Bearer <access_token>`, `Content-Type: application/json`.
- Body: `{"name":"Malla 2026"}` (1-200 caracteres).

## Res
- `201`: `{"id":"<uuid>"}`.
- `400`: `{"error":{"code":"bad_request|invalid_name","message":"..."}}` (JSON malformado o `name` vacío/>200).
- `401`: `{"error":{"code":"unauthorized|invalid_token|token_expired","message":"..."}}` (sin token o inválido).
- `503`: `{"error":{"code":"db_unconfigured","message":"..."}}` (arranque sin pool; no 404 para no confundir con recurso ajeno).

## Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/academic/academic_api.dart:192` (`createCurriculum`)
  - Pantalla: `front/lib/features/academic/curriculum_screen.dart:290` (diálogo crear malla)
- **Otros Microservicios:** No se consume por otros microservicios.

## Flujo
- `back/infacadem/internal/handler/http/curricula_handler.go` (`Create`) → `back/infacadem/internal/service/curriculum_service.go` (`Create`, valida `name`) → `back/infacadem/internal/repository/curriculum_repository.go` (`Create`, `INSERT ... RETURNING`); auth vía `back/infacadem/internal/middleware/auth_middleware.go`.

## Tests
- `back/infacadem/internal/handler/http/academic_handler_test.go`: `TestHandler_SinToken_401`, `TestHandler_FlujoCompleto`, `TestHandler_Validaciones_400`, `TestHandler_DoblePOST_Concurrente`.
- `back/infacadem/internal/service/academic_service_test.go`: `TestCurriculum_CRUD`, `TestCurriculum_ConcurrentCreate`.
- `back/infacadem/internal/service/academic_integration_test.go`: `TestAcademicIntegration_CRUDCascada` (skip sin `ACADEMIC_TEST_DATABASE_URL`).
