# POST /notifications/:id/read

Marca una notificación como leída (Social, tarea 3_5_14).

## Req
- Método: `POST`, ruta: `/notifications/:id/read` (protegida con JWT `Bearer`).
- Path: `id` (uuid de la notificación). Sin body.

## Res
- `200`: `{"message":"notification marked as read"}`.
- `401`: `unauthorized`. `404`: `notification_not_found` (ajena, inexistente o ya leída — sin filtrar). `500`: `internal`.

## Flujo
- `NotificationHandler.MarkRead` → `NotificationService.MarkRead` (valida uuids) → `MeetingRepository.MarkNotificationRead` (`UPDATE ... SET read_at=now() WHERE id AND user_id AND read_at IS NULL`; 0 filas → 404).

## Tests
- `notification_handler_test.go`: `TestNotifications_MarkRead_Unit` (200, releída → 404, inexistente → 404, 401).
- `notification_integration_test.go`: `TestNotifPG_FlowLectura` (`read_at` persistido, releída → 404, admin sobre ajena → 404).
