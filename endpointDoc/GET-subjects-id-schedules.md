# GET /subjects/{id}/schedules

## Req
- Método: `GET`, ruta: `/subjects/:id/schedules` (protegida, `back/infacadem/cmd/server/main.go:123`).
- Headers: `Authorization: Bearer <access_token>`. Sin body.

## Res
- `200`: `{"items":[{"id":"...","subject_id":"...","day_of_week":2,"start_time":"10:00:00","end_time":"11:30:00","modality":"Cátedra"}]}` (horas serializadas `"15:04:05"`; `modality` null si ausente; `[]` nunca `null`).
- `400`: `{"error":{"code":"invalid_uuid","message":"..."}}`.
- `401`: `{"error":{"code":"unauthorized|invalid_token|token_expired","message":"..."}}`.
- `404`: `{"error":{"code":"not_found","message":"..."}}` (materia no existe o es ajena).

## Flujo
- `back/infacadem/internal/handler/http/schedules_handler.go` (`ListBySubject`, mapea `time.Time` a `"15:04:05"`) → `back/infacadem/internal/service/schedule_service.go` (`List`) → `back/infacadem/internal/repository/schedule_repository.go` (`ListBySubject`, check `EXISTS` + `ORDER BY day_of_week, start_time, id`).

## Tests
- `back/infacadem/internal/handler/http/academic_handler_test.go`: `TestHandler_SinToken_401`, `TestHandler_FlujoCompleto`, `TestHandler_CrossUser_404`.
- `back/infacadem/internal/service/academic_service_test.go`: `TestSchedule_CRUD_Validaciones`.
