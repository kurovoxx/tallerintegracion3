# PATCH /absences/{id}

## Req
- Método: `PATCH`, ruta: `/absences/:id` (protegida, `back/infacadem/cmd/server/main.go:151`).
- Headers: `Authorization: Bearer <access_token>`, `Content-Type: application/json`.
- Body: `{"justified":true}` (requerido; mueve el conteo justificada↔injustificada) + opcionales `source`/`type` (`""` limpia a NULL).

## Res
- `200`: recurso actualizado.
- `400`: `{"error":{"code":"bad_request|invalid_uuid","message":"..."}}` (sin `justified` o UUID malo).
- `401` / `404` (ajena/inexistente): códigos habituales.

## Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/academic/academic_api.dart:628` (`patchAbsence`)
  - Pantalla: `front/lib/features/academic/attendance_screen.dart:676` (`_updateAbsence`)
- **Otros Microservicios:** No se consume por otros microservicios.

## Flujo
- `back/infacadem/internal/handler/http/absences_handler.go` (`Update`) → `back/infacadem/internal/service/absence_service.go` (`SetJustified`, `justified` nil → 400) → `back/infacadem/internal/repository/absence_repository.go` (`UpdateJustified`).

## Tests
- `back/infacadem/internal/handler/http/attendance_handler_test.go`: `TestAbsences_FlujoCompleto`, `TestAbsences_Validaciones`.
- `back/infacadem/internal/service/attendance_service_test.go`: `TestAttendance_FormulaFrente` (toggle true→false mueve el conteo: 0.84375→0.875, restantes 3→4).
