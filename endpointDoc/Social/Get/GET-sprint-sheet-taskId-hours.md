# GET /sprint-sheet/:taskId/hours

Lista las horas diarias de una tarea de sprint y su total (microservicio `social`).

## Req
- Método: `GET`, ruta: `/sprint-sheet/:taskId/hours` (protegida con JWT `Bearer`; solo miembros del grupo dueño de la tarea; `back/social/cmd/server/main.go:273`).
- Headers: `Authorization: Bearer <access_token>`.
- Path: `taskId` (uuid). Query opcional: `from`, `to` (`YYYY-MM-DD`, inclusivos; cualquiera puede omitirse).

## Res
- `200`: `{"data":[{"id","task_id","log_date","hours"}],"total_hours":<float>}`. `data` nunca es `null`; `total_hours` suma solo las filas devueltas (respeta el rango).
- `400` `invalid_sprint_task_id` o `invalid_body` (fecha de filtro inválida). `403` `forbidden`. `404` `sprint_task_not_found`. `401`: sin token.

## Flujo
- `HoursHandler.ListHours` (`internal/handler/http/hours_handler.go:92`) → `HoursService.ListHours` (`internal/service/hours_service.go:207`: valida fechas, resuelve grupo, exige membresía, suma en Go) → `ListDailyHoursByTask` (sqlc, filtro por rango en SQL).

## Tests
- `internal/handler/http/hours_handler_test.go`: `TestListHoursContractAndMembership` (contrato, 401/403/404/400, lista vacía, total).
- `test/social_routes_e2e_test.go`: subtest `todo_sprint_hours_and_workspace` (filtro por rango con PostgreSQL real; el store en memoria no lo implementa).
