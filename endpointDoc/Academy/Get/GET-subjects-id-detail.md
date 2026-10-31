# GET /subjects/{id}/detail

## Req
- Método: `GET`, ruta: `/subjects/:id/detail` (protegida, `back/infacadem/cmd/server/main.go:166`).
- Headers: `Authorization: Bearer <access_token>`. Sin body.

## Res
- `200`: `{"subject":{...,"code","professor","room"},...,"schedules":[{"modality":...}],...}` (misma forma que las rutas individuales, con los campos nuevos; listas `[]` nunca `null`; materia vacía → `average: {"average":null,"status":"pending"}` + `percentage: null`).
- `400`: `{"error":{"code":"invalid_uuid","message":"..."}}`.
- `401`: `{"error":{"code":"unauthorized|invalid_token|token_expired","message":"..."}}`.
- `404`: `{"error":{"code":"not_found","message":"..."}}` (ajena/inexistente).

## Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/academic/academic_api.dart`
  - Pantalla: `front/lib/features/academic/courses_screen.dart:150` (vista completa de la materia)
- **Otros Microservicios:** No se consume por otros microservicios.

## Flujo
- `back/infacadem/internal/handler/http/overview_handler.go` (`SubjectDetail`, DTOs de `present.go`) → `back/infacadem/internal/service/overview_service.go` (`DetailService.Get`: compone los 5 services existentes sin SQL nuevo, ~7 queries indexadas, mismo costo que las 5 llamadas individuales).

## Tests
- `back/infacadem/internal/handler/http/overview_handler_test.go`: `TestSubjectDetail_HTTP` (forma exacta, materia vacía, 404/400/401), `TestSubjectDetail_Consistencia` (agregado idéntico a las 5 rutas individuales).
- `back/infacadem/internal/service/overview_service_test.go`: `TestDetail_Vacia`, `TestDetail_Completa_Decimales` (`average_decimal_places` por materia respetado).
