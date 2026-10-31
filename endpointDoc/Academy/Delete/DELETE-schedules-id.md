# DELETE /schedules/{id}

## Req
- Método: `DELETE`, ruta: `/schedules/:id` (protegida, `back/infacadem/cmd/server/main.go:124`).
- Headers: `Authorization: Bearer <access_token>`. Sin body.

## Res
- `204`: sin body.
- `400`: `{"error":{"code":"invalid_uuid","message":"..."}}`.
- `401`: `{"error":{"code":"unauthorized|invalid_token|token_expired","message":"..."}}`.
- `404`: `{"error":{"code":"not_found","message":"..."}}` (no existe o es ajeno).

## Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/academic/academic_api.dart` (`deleteSchedule`)
  - Pantalla: `front/lib/features/academic/schedule_screen.dart:430` (eliminar bloque)
- **Otros Microservicios:** No se consume por otros microservicios.

## Flujo
- `back/infacadem/internal/handler/http/schedules_handler.go` (`Delete`) → `back/infacadem/internal/service/schedule_service.go` (`Delete`) → `back/infacadem/internal/repository/schedule_repository.go` (`Delete`, `DELETE ... WHERE EXISTS` vía subjects→curricula).

## Tests
- `back/infacadem/internal/handler/http/academic_handler_test.go`: `TestHandler_SinToken_401`, `TestHandler_FlujoCompleto`, `TestHandler_CrossUser_404`, `TestHandler_Validaciones_400` (uuid malo).
- `back/infacadem/internal/service/academic_service_test.go`: `TestSchedule_CRUD_Validaciones`.
- `back/infacadem/internal/service/academic_integration_test.go`: `TestAcademicIntegration_CRUDCascada` (delete ajeno 404).
