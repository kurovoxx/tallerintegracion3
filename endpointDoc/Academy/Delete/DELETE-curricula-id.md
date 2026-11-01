# DELETE /curricula/{id}

## Req
- Método: `DELETE`, ruta: `/curricula/:id` (protegida, `back/infacadem/cmd/server/main.go:114`).
- Headers: `Authorization: Bearer <access_token>`. Sin body.

## Res
- `204`: sin body (subjects y schedules caen por `ON DELETE CASCADE`).
- `400`: `{"error":{"code":"invalid_uuid","message":"..."}}`.
- `401`: `{"error":{"code":"unauthorized|invalid_token|token_expired","message":"..."}}`.
- `404`: `{"error":{"code":"not_found","message":"..."}}` (no existe o es ajena).

## Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/academic/academic_api.dart` (`deleteCurriculum`)
  - Pantalla: `front/lib/features/academic/curriculum_screen.dart:340` (eliminar malla)
- **Otros Microservicios:** No se consume por otros microservicios.

## Flujo
- `back/infacadem/internal/handler/http/curricula_handler.go` (`Delete`) → `back/infacadem/internal/service/curriculum_service.go` (`Delete`) → `back/infacadem/internal/repository/curriculum_repository.go` (`Delete`, `DELETE ... WHERE id=$1 AND user_id=$2`; cascada por FK).

## Tests
- `back/infacadem/internal/handler/http/academic_handler_test.go`: `TestHandler_SinToken_401`, `TestHandler_FlujoCompleto` (tras delete 404), `TestHandler_CrossUser_404`.
- `back/infacadem/internal/service/academic_service_test.go`: `TestCurriculum_CRUD`.
- `back/infacadem/internal/service/academic_integration_test.go`: `TestAcademicIntegration_CRUDCascada` (huérfanos cero vía query directa).
