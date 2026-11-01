# GET /groups/:id/meetings

Ventana "próximas reuniones" del grupo (Social, tarea 3_5_14).

## Req
- Método: `GET`, ruta: `/groups/:id/meetings` (protegida con JWT `Bearer`; solo miembros).
- Path: `id` (uuid del grupo).

## Res
- `200`: `{"data":[{"id":"uuid","group_id":"uuid","title":"...","description":null,"scheduled_at":"RFC3339","created_by":"uuid"}]}` (`scheduled_at >= now`, orden cronológico).
- `400`: `invalid_group_id`. `401`: `unauthorized`. `403`: `forbidden` (no-miembro). `404`: `group_not_found`. `500`: `internal`.

## Flujo
- `MeetingHandler.ListMeetings` → `MeetingService.ListUpcomingMeetings` (existente: `requireGroup` + `requireMembership`, filtra `scheduled_at >= now`) → `ListMeetingsByGroup`.

## Tests
- `notification_handler_test.go`: `TestMeetings_List_Unit` (solo futura, forma, 403/401/404).
- `notification_integration_test.go`: `TestNotifPG_FlowLectura` (1 próxima real).
