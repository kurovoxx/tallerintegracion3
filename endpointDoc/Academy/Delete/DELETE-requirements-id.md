# DELETE /requirements/{id}

## Req
- Método: `DELETE`, ruta: `/requirements/:id` (protegida, `back/infacadem/cmd/server/main.go:143`).
- Headers: `Authorization: Bearer <access_token>`. Sin body.

## Res
- `204`: sin body.
- `400`: `{"error":{"code":"invalid_uuid","message":"..."}}`.
- `401`: `{"error":{"code":"unauthorized|invalid_token|token_expired","message":"..."}}`.
- `404`: `{"error":{"code":"not_found","message":"..."}}` (ajeno/inexistente).

## Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/academic/academic_api.dart` (`deleteRequirement`)
- **Otros Microservicios:** No se consume por otros microservicios.

## Flujo
- `back/infacadem/internal/handler/http/requirements_handler.go` (`Delete`) → `back/infacadem/internal/service/requirement_service.go` (`Delete`) → `back/infacadem/internal/repository/requirement_repository.go` (`Delete`, `DELETE ... WHERE EXISTS` vía subjects→curricula).

## Tests
- `back/infacadem/internal/handler/http/grades_handler_test.go`: `TestGrades_Average_Requisitos_HTTP`.
- Sin test dedicado de cross-user 404 para este DELETE (cubierto a nivel service por el patrón `Delete` con propiedad; ver OBS-2 del veredicto).
