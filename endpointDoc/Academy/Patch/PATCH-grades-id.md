# PATCH /grades/{id}

## Req
- Método: `PATCH`, ruta: `/grades/:id` (protegida, `back/infacadem/cmd/server/main.go:136`).
- Headers: `Authorization: Bearer <access_token>`, `Content-Type: application/json`.
- Body parcial: `{"score":60}` (cualquiera de `name/score/weight/status/scheduled_date/topics/notify_discord/google_calendar_event_id/is_final_exam`; `null` explícito en `score/scheduled_date/topics/google_calendar_event_id` pide NULL).

## Res
- `200`: recurso actualizado.
- `400`: `{"error":{"code":"bad_request|invalid_name|invalid_score|invalid_weight|invalid_status|invalid_date|invalid_uuid","message":"..."}}` (body vacío o campo inválido).
- `401`: `{"error":{"code":"unauthorized|invalid_token|token_expired","message":"..."}}`.
- `404`: `{"error":{"code":"not_found","message":"..."}}`.
- `409`: `{"error":{"code":"conflict","message":"..."}}` (promover a final habiendo otro).

## Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/academic/academic_api.dart:454` (`patchGrade`) y `L749` (`renameGrade`)
  - Pantalla: `front/lib/features/academic/grade_calculator_screen.dart:380`
- **Otros Microservicios:** No se consume por otros microservicios.

## Flujo
- `back/infacadem/internal/handler/http/grades_handler.go` (`Update`, detecta `null` explícitos vía doble parseo) → `back/infacadem/internal/service/grade_service.go` (`Update`, lee actual para validar final único con `excludeID`) → `back/infacadem/internal/repository/grade_repository.go` (`Update`, `SET` dinámico + `Clear*`).

## Tests
- `back/infacadem/internal/handler/http/grades_handler_test.go`: `TestGrades_FlujoCompleto`, `TestGrades_Validaciones_400`.
- `back/infacadem/internal/service/grade_service_test.go`: `TestGrade_FinalUnico_409` (create y PATCH duplicado).
