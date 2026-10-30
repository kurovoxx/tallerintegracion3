# GET /absences/{id}

## Req
- Método: `GET`, ruta: `/absences/:id` (protegida, `back/infacadem/cmd/server/main.go:150`). Endpoint extra consistente con los demás recursos (el plan pedía PATCH/DELETE; el GET viene gratis por el mismo patrón).
- Headers: `Authorization: Bearer <access_token>`. Sin body.

## Res
- `200`: `{"id":"...","subject_id":"...","date":"...","justified":false,"source":null,"type":null,"created_at":"..."}`.
- `400/401/404`: `invalid_uuid` / auth / ajena-inexistente.

## Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/academic/academic_api.dart` (consumido vía listAbsences o navegación directa)
- **Otros Microservicios:** No se consume por otros microservicios.

## Flujo
- `back/infacadem/internal/handler/http/absences_handler.go` (`Get`) → `back/infacadem/internal/service/absence_service.go` (`Get`) → `back/infacadem/internal/repository/absence_repository.go` (`GetByID`, `JOIN` subjects→curricula).

## Tests
- `back/infacadem/internal/handler/http/attendance_handler_test.go`: `TestAbsences_FlujoCompleto`, `TestAbsences_CrossUser_404`.
