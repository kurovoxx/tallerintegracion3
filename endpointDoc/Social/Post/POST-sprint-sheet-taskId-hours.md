# POST /sprint-sheet/:taskId/hours

Registra (o reemplaza) las horas de un día para una tarea de sprint (microservicio `social`). Un registro por tarea y fecha: repetir la fecha actualiza, no suma.

## Req
- Método: `POST`, ruta: `/sprint-sheet/:taskId/hours` (protegida con JWT `Bearer`; solo miembros del grupo dueño de la tarea; `back/social/cmd/server/main.go:272`).
- Headers: `Content-Type: application/json`, `Authorization: Bearer <access_token>`.
- Path: `taskId` (uuid de la tarea de sprint).
- Body (`LogHoursRequest`, `hours_handler.go:25-28`): `{"log_date":"2026-10-22","hours":2.5}`. `log_date` obligatorio (`YYYY-MM-DD`); `hours` >= 0, se guarda con 2 decimales.

## Res
- `201`: `{"message":"daily hours registered successfully","data":{"id","task_id","log_date","hours"}}` (fecha nueva).
- `200`: mismo formato con `"daily hours updated successfully"` (la fecha ya existía y se reemplazó).
- `400` `invalid_sprint_task_id` (uuid malo) o `invalid_body` (fecha/horas inválidas). `403` `forbidden`: no-miembro. `404` `sprint_task_not_found`. `401`: sin token. `500` `internal`.
- Formato de error propio de este handler: `{"error":"<texto>","code":"<code>"}` (no el `{"error":{code,message}}` de sprint-sheets).

## Flujo
- `HoursHandler.LogHours` (`internal/handler/http/hours_handler.go:30`: valida uuid y body) → `HoursService.LogHours` (`internal/service/hours_service.go:129`: tarea → hoja → grupo, exige membresía, busca la fila `(task_id, log_date)` y hace update o insert; si dos inserts compiten, el `23505` se resuelve con update) → `HoursRepository` (sqlc, tabla `social.sprint_sheet_daily_hours`, `UNIQUE (task_id, log_date)`).
- Las horas son por tarea, no por usuario: cualquier miembro pisa el valor del día.

## Notas
- La columna es `numeric(4,2)`: el máximo almacenable es `99.99`. Un valor mayor no se valida en Go y termina en `500` (overflow de PostgreSQL). No hay tope de 24 h por día.

## Tests
- `internal/handler/http/tasks_routes_test.go`: `TestSprintTaskIdRoutes_Contrato` (201 miembro, 403 ajeno).
- `internal/service/sprint_membership_test.go` (membresía y alta nueva vs. actualización).
- `test/social_routes_e2e_test.go`: subtest `todo_sprint_hours_and_workspace` (alta, actualización por fecha y total).
- `test/cluster_smoke_test.go`: upsert sin duplicar filas en el clúster.
