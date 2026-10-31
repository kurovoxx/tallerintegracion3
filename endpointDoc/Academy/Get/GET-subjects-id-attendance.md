# GET /subjects/{id}/attendance

## Req
- Método: `GET`, ruta: `/subjects/:id/attendance` (protegida, `back/infacadem/cmd/server/main.go:153`).
- Headers: `Authorization: Bearer <access_token>`. Sin body. Solo lectura, no persiste nada.

## Res
- `200`: `{"total_classes":32,"unjustified_count":5,"justified_count":2,"percentage":0.84375,"min_attendance_pct":75,"meets_minimum":true,"remaining":3}`.
  - `percentage` fracción 0–1 (`1 - injustificadas/total`; justificadas no restan; valor crudo — el front aplica `*100` + clamp para display, divergencia documentada en `attendance_service.go:13-16`).
  - `total_classes` null/0 → `percentage/meets_minimum/remaining` null; `min_attendance_pct` null → `meets_minimum/remaining` null (`percentage` sí calcula).
  - `meets_minimum`: `percentage >= min/100`; `remaining`: `floor(total*(1-min/100)) - injustificadas` (negativo si se excedió, igual que el front).
- `400/401/404`: `invalid_uuid` / auth / materia ajena.

## Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/academic/academic_api.dart:659` (`getAttendance`)
  - Pantalla: `front/lib/features/academic/attendance_screen.dart:648` (`_loadData`)
- **Otros Microservicios:** No se consume por otros microservicios.

## Flujo
- `back/infacadem/internal/handler/http/attendance_handler.go` (`Get`) → `back/infacadem/internal/service/attendance_service.go` (`Summary`: subject + `CountsBySubject`) → `repository` (`SubjectRepository.GetByID`, `AbsenceRepository.CountsBySubject` con `FILTER`).

## Tests
- `back/infacadem/internal/handler/http/attendance_handler_test.go`: `TestAttendance_Resumen_HTTP`, `TestAbsences_CrossUser_404`.
- `back/infacadem/internal/service/attendance_service_test.go`: `TestAttendance_FormulaFrente` (réplica exacta front: 32/5+2 → 0.84375, restantes 3), `TestAttendance_Nulos`.
- `back/infacadem/internal/service/academic_integration_test.go`: `TestAcademicIntegration_Attendance` (conteos reales + cascada subject→absences).
