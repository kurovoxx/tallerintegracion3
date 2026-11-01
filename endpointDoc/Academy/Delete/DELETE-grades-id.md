# DELETE /grades/{id}

## Req
- Método: `DELETE`, ruta: `/grades/:id` (protegida, `back/infacadem/cmd/server/main.go:137`).
- Headers: `Authorization: Bearer <access_token>`. Sin body.

## Res
- `204`: sin body (hijas caen por `ON DELETE CASCADE` en `parent_grade_id`).
- `400`: `{"error":{"code":"invalid_uuid","message":"..."}}`.
- `401`: `{"error":{"code":"unauthorized|invalid_token|token_expired","message":"..."}}`.
- `404`: `{"error":{"code":"not_found","message":"..."}}`.

## Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/academic/academic_api.dart:470` (`deleteGrade`)
  - Pantalla: `front/lib/features/academic/grade_calculator_screen.dart:410`
- **Otros Microservicios:** No se consume por otros microservicios.

## Flujo
- `back/infacadem/internal/handler/http/grades_handler.go` (`Delete`) → `back/infacadem/internal/service/grade_service.go` (`Delete`) → `back/infacadem/internal/repository/grade_repository.go` (`Delete`, `DELETE ... WHERE EXISTS` propiedad).

## Tests
- `back/infacadem/internal/handler/http/grades_handler_test.go`: `TestGrades_FlujoCompleto`, `TestGrades_CrossUser_404`.
- `back/infacadem/internal/service/grade_service_test.go`: `TestGrade_Create_Validaciones`.
