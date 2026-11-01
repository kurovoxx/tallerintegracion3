# GET /curricula/{id}

## Req
- Método: `GET`, ruta: `/curricula/:id` (protegida, `back/infacadem/cmd/server/main.go:112`).
- Headers: `Authorization: Bearer <access_token>`. `:id` UUID.

## Res
- `200`: `{"id":"...","user_id":"...","name":"...","created_at":"..."}`.
- `400`: `{"error":{"code":"invalid_uuid","message":"..."}}` (UUID malformado; convención documentada en `service/errors.go`).
- `401`: `{"error":{"code":"unauthorized|invalid_token|token_expired","message":"..."}}`.
- `404`: `{"error":{"code":"not_found","message":"..."}}` (no existe o es ajena; nunca 403).

## Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/academic/academic_api.dart:206` (`ensureCurriculum`)
  - Pantalla: `front/lib/features/academic/curriculum_screen.dart:180`
- **Otros Microservicios:** No se consume por otros microservicios.

## Flujo
- `back/infacadem/internal/handler/http/curricula_handler.go` (`Get`) → `back/infacadem/internal/service/curriculum_service.go` (`Get`, valida UUID) → `back/infacadem/internal/repository/curriculum_repository.go` (`GetByID`, `WHERE id=$1 AND user_id=$2`).

## Tests
- `back/infacadem/internal/handler/http/academic_handler_test.go`: `TestHandler_SinToken_401`, `TestHandler_FlujoCompleto`, `TestHandler_CrossUser_404`, `TestHandler_Validaciones_400` (uuid malo).
- `back/infacadem/internal/service/academic_service_test.go`: `TestCurriculum_CRUD`.
- `back/infacadem/internal/service/academic_integration_test.go`: `TestAcademicIntegration_CRUDCascada` (propiedad cruzada a nivel SQL).
