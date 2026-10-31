# GET /subjects/{id}/grades

## Req
- Método: `GET`, ruta: `/subjects/:id/grades` (protegida, `back/infacadem/cmd/server/main.go:134`).
- Headers: `Authorization: Bearer <access_token>`. Sin body.

## Res
- `200`: `{"items":[{"grade":{...},"children":[{...}]}]}` (notas tope con hijas anidadas; `children: []` si no tiene; `items: []` si vacía).
- `400`: `{"error":{"code":"invalid_uuid","message":"..."}}`.
- `401`: `{"error":{"code":"unauthorized|invalid_token|token_expired","message":"..."}}`.
- `404`: `{"error":{"code":"not_found","message":"..."}}` (materia ajena/inexistente).

## Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/academic/academic_api.dart:383` (`listGrades`)
  - Pantalla: `front/lib/features/academic/grade_calculator_screen.dart:230` (`_loadGrades`)
- **Otros Microservicios:** No se consume por otros microservicios.

## Flujo
- `back/infacadem/internal/handler/http/grades_handler.go` (`List`) → `back/infacadem/internal/service/grade_service.go` (`List`, agrupa por `parent_grade_id`) → `back/infacadem/internal/repository/grade_repository.go` (`ListBySubject`, plano + check de propiedad).

## Tests
- `back/infacadem/internal/handler/http/grades_handler_test.go`: `TestGrades_FlujoCompleto`, `TestGrades_CrossUser_404`.
- `back/infacadem/internal/service/grade_service_test.go`: `TestGrade_Hijos_Efectivo`.
