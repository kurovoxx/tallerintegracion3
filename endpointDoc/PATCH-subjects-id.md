# PATCH /subjects/{id}

## Req
- Método: `PATCH`, ruta: `/subjects/:id` (protegida, `back/infacadem/cmd/server/main.go:119`).
- Headers: `Authorization: Bearer <access_token>`, `Content-Type: application/json`.
- Body parcial: `{"credits":10}` (cualquiera de `name`, `semester`, `credits`, `total_classes`, `min_attendance_pct`, `average_decimal_places`, `status`; al menos un campo).

## Res
- `200`: recurso actualizado.
- `400`: `{"error":{"code":"bad_request|invalid_name|invalid_decimal_places|invalid_status|invalid_uuid","message":"..."}}` (body vacío o campo inválido).
- `401`: `{"error":{"code":"unauthorized|invalid_token|token_expired","message":"..."}}`.
- `404`: `{"error":{"code":"not_found","message":"..."}}` (no existe o su malla es ajena).

## Flujo
- `back/infacadem/internal/handler/http/subjects_handler.go` (`Update`) → `back/infacadem/internal/service/subject_service.go` (`Update`, PATCH parcial con `SubjectPatch`) → `back/infacadem/internal/repository/subject_repository.go` (`Update`, `SET` dinámico con placeholders numerados + check de propiedad en `WHERE EXISTS`).

## Tests
- `back/infacadem/internal/handler/http/academic_handler_test.go`: `TestHandler_SinToken_401`, `TestHandler_FlujoCompleto`, `TestHandler_CrossUser_404`, `TestHandler_Validaciones_400` (patch vacío).
- `back/infacadem/internal/service/academic_service_test.go`: `TestSubject_CRUD_Validaciones`.
