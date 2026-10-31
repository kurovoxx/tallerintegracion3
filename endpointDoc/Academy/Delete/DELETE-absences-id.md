# DELETE /absences/{id}

## Req
- Método: `DELETE`, ruta: `/absences/:id` (protegida, `back/infacadem/cmd/server/main.go:152`).
- Headers: `Authorization: Bearer <access_token>`. Sin body.

## Res
- `204`: sin body (restaura el porcentaje al eliminar la falta).
- `400/401/404`: `invalid_uuid` / auth / ajena-inexistente.

## Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/academic/academic_api.dart:644` (`deleteAbsence`)
  - Pantalla: `front/lib/features/academic/attendance_screen.dart:662` (`_deleteAbsence`) y `L1381` (botón de eliminar inasistencia)
- **Otros Microservicios:** No se consume por otros microservicios (operación propia de usuario).

## Flujo
- `back/infacadem/internal/handler/http/absences_handler.go` (`Delete`) → `back/infacadem/internal/service/absence_service.go` (`Delete`) → `back/infacadem/internal/repository/absence_repository.go` (`Delete`, `DELETE ... WHERE EXISTS` propiedad).

## Tests
- `back/infacadem/internal/handler/http/attendance_handler_test.go`: `TestAbsences_FlujoCompleto`, `TestAbsences_CrossUser_404`.
- `back/infacadem/internal/service/academic_integration_test.go`: `TestAcademicIntegration_Attendance`.
