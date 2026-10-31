# GET /notifications/me

Cajita de notificaciones: lista las notificaciones in-app del usuario (Social, tarea 3_5_14).

## Req
- Método: `GET`, ruta: `/notifications/me` (protegida con JWT `Bearer`; cada usuario ve solo las suyas).
- Sin body ni query.

## Res
- `200`: `{"data":[{"id":"uuid","meeting_id":"uuid","group_id":"uuid","title":"...","scheduled_at":"RFC3339","read":false,"read_at":null,"created_at":"RFC3339"}]}` (no leídas primero; vacío = `[]`).
- `401`: `unauthorized`. `500`: `internal`.

## Flujo
- `NotificationHandler.ListMine` → `NotificationService.ListMine` (valida uuid) → `MeetingRepository.ListNotificationsByUser` (SQL directo: `JOIN social.meeting_notifications + meetings`, `ORDER BY (read_at IS NOT NULL), created_at DESC`). Las filas las crea el fan-out de `CreateMeetingWithNotifications` (todos los miembros excepto el creador).

## Tests
- `notification_handler_test.go`: `TestNotifications_ListMine_Unit` (forma completa, no-leída primero, 401).
- `notification_integration_test.go`: `TestNotifPG_FlowLectura` (miembro ve 1, creador ve 0).
