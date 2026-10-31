# POST /subjects/{id}/absences

## Req
- Método: `POST`, ruta: `/subjects/:id/absences` (protegida, `back/infacadem/cmd/server/main.go:148`).
- Headers: `Authorization: Bearer <access_token>`, `Content-Type: application/json`.
- Body: `{"date":"2026-10-05","justified":false}` (`date` YYYY-MM-DD requerida; cualquier fecha válida pasada/hoy/futura — decisión documentada en `absence_service.go:32-35`; opcionales `source` —self/teacher— y `type` —Teoría/Lab/…—, varchar libres).

## Res
- `201`: `{"id":"<uuid>"}`.
- `400`: `{"error":{"code":"bad_request|invalid_date|invalid_uuid","message":"..."}}` (`2026-02-30` o no ISO → `invalid_date`).
- `401`: `{"error":{"code":"unauthorized|invalid_token|token_expired","message":"..."}}`.
- `404`: `{"error":{"code":"not_found","message":"..."}}` (materia ajena/inexistente).
- `409`: `{"error":{"code":"conflict","message":"..."}}` (ya hay falta en esa materia+fecha).

## Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/academic/academic_api.dart:602` (`createAbsence`)
  - Pantalla: `front/lib/features/academic/attendance_screen.dart:653` (`_createAbsence`)
- **Otros Microservicios:** No se consume por otros microservicios.

## Flujo
- `back/infacadem/internal/handler/http/absences_handler.go` (`Create`) → `back/infacadem/internal/service/absence_service.go` (`Create`, valida fecha + `ExistsOnDate` → 409) → `back/infacadem/internal/repository/absence_repository.go` (`ExistsOnDate`, `Create` atómico con propiedad).

## Tests
- `back/infacadem/internal/handler/http/attendance_handler_test.go`: `TestAbsences_FlujoCompleto`, `TestAbsences_CrossUser_404`, `TestAbsences_Validaciones`.
- `back/infacadem/internal/service/attendance_service_test.go`: `TestAbsence_CRUD_Duplicado`.
- `back/infacadem/internal/service/academic_integration_test.go`: `TestAcademicIntegration_Attendance`.
