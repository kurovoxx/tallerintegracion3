# GET /subjects/{id}/average

## Req
- Método: `GET`, ruta: `/subjects/:id/average` (protegida, `back/infacadem/cmd/server/main.go:139`).
- Headers: `Authorization: Bearer <access_token>`. Sin body. Solo lectura, no persiste nada.

## Res
- `200`: `{"average":5.6,"status":"approved"}` (`average` escala 1.0–7.0 redondeado con `RoundHalfUp` a `average_decimal_places` del subject; `status` en `approved|failed|exam_required|pending`).
- `200`: `{"average":null,"status":"pending"}` (nada califica: sin notas con score o Σ(pesos)==0).
- `400/401/404`: `invalid_uuid` / auth / materia ajena.
- Orden de requisitos: `force_fail` > `force_exam` > `force_pass`; sin disparos, aprobado si `average >= 4.0`. Sin redistribución de pesos (Σ≠1 permitida, igual que el front).

## Flujo
- `back/infacadem/internal/handler/http/grades_handler.go` (`Average`) → `back/infacadem/internal/service/average_service.go` (`Average`: valores efectivos recursivos + requisitos) → `repository` (`SubjectRepository.GetByID`, `GradeRepository.ListBySubject`, `RequirementRepository.ListBySubject`).

## Tests
- `back/infacadem/internal/handler/http/grades_handler_test.go`: `TestGrades_Average_Requisitos_HTTP`, `TestGrades_CrossUser_404`.
- `back/infacadem/internal/service/grade_service_test.go`: `TestAverage_Calculo_Requisitos` (redondeo 5.55→5.6, orden force_fail>force_exam), `TestRequirement_FuentePendiente_NoDispara`.
