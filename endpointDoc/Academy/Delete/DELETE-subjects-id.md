# DELETE /subjects/{id}

## Req
- Método: `DELETE`, ruta: `/subjects/:id` (protegida, `back/infacadem/cmd/server/main.go:120`).
- Headers: `Authorization: Bearer <access_token>`. Sin body.

## Res
- `204`: sin body (schedules caen por `ON DELETE CASCADE`).
- `400`: `{"error":{"code":"invalid_uuid","message":"..."}}`.
- `401`: `{"error":{"code":"unauthorized|invalid_token|token_expired","message":"..."}}`.
- `404`: `{"error":{"code":"not_found","message":"..."}}` (no existe o su malla es ajena).

## Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/academic/academic_api.dart` (`deleteSubject`)
  - Pantalla: `front/lib/features/academic/courses_screen.dart:410` (eliminar materia)
- **Otros Microservicios:** No se consume por otros microservicios.

## Flujo
- `back/infacadem/internal/handler/http/subjects_handler.go` (`Delete`) → `back/infacadem/internal/service/subject_service.go` (`Delete`) → `back/infacadem/internal/repository/subject_repository.go` (`Delete`, `DELETE ... WHERE EXISTS` propiedad).

## Tests
- `back/infacadem/internal/handler/http/academic_handler_test.go`: `TestHandler_SinToken_401`, `TestHandler_FlujoCompleto`, `TestHandler_CrossUser_404`.
- `back/infacadem/internal/service/academic_service_test.go`: `TestSubject_CRUD_Validaciones`.
