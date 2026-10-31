# POST /groups/:id/meetings

Agenda una reunión del grupo y dispara notificaciones best-effort (microservicio `social`).

## Req
- Método: `POST`, ruta: `/groups/:id/meetings` (protegida con JWT `Bearer`; exige membresía; `back/social/cmd/server/main.go:211`).
- Headers: `Content-Type: application/json`, `Authorization: Bearer <access_token>`.
- Path: `id` (uuid del grupo).
- Body (`CreateMeetingRequest`, `internal/handler/http/meeting_handler.go:25-31`): `{"title":"Daily","description":"opcional","scheduled_at":"2026-10-20T15:00:00Z","notify_discord":true,"attendees":["a@mail.com"]}`.
- `title` requerido max 300. `scheduled_at` requerido RFC3339 (también acepta `2006-01-02T15:04:05`, `2006-01-02 15:04:05`, `2006-01-02`). `description` opcional (vacío → NULL). `notify_discord` opcional, default `true`. `attendees` opcional: emails normalizados (trim+lowercase, dedupe), max 50, ≤255 con formato válido.

## Res
- `201`: `{"meeting_id":"uuid"}` (contrato `agentApiContract.md §5`).
- `400`: `invalid_group_id|invalid_body` (título, `scheduled_at is required`, fecha inválida, email inválido, max 50) `|invalid_user_id`. `401`: `unauthorized`. `403`: `forbidden`. `404`: `group_not_found`. `500`: `internal`.

## Flujo
1. `MeetingHandler.CreateMeeting` (`meeting_handler.go:33-95`) valida `id` + JSON + `user_id` del JWT.
2. `MeetingService.CreateMeeting` (`internal/service/meeting_service.go:166-217`): valida título/fecha/invitados, `requireMeetingGroup` + `requireMembership`.
3. `MeetingRepository.CreateMeetingWithNotifications` → transacción: `INSERT INTO social.meetings` + `INSERT INTO social.meeting_attendees` + fan-out `INSERT INTO social.meeting_notifications` a miembros excepto creador (`internal/repository/sqlc/meetings.sql.go:10,109,206`).
4. Post-commit best-effort en background (`GoBestEffort`, `meeting_service.go:212-214`): `MultiMeetingNotifier` (`cmd/server/main.go:88-93`) → Calendar (mock/real), Discord webhook, mensaje Stream. Un fallo no revierte la reunión ni cambia el `201`.

## Tests
- `internal/handler/http/martin_unit_test.go`: `TestMartin_MeetingCreate_Unit` (201 + fecha/invitados inválidos, 401/403/404).
- `internal/handler/http/martin_integration_test.go`: `TestMartinIntegration_MeetingCreate_PG` (filas en `meetings` + `attendees` + 1 notificación al miembro).
- `internal/handler/http/martin_e2e_test.go`: `TestMartinE2E_MeetingJourney` (HTTP real + verificación en DB).
- `internal/service/meeting_service_seam_test.go`: `TestMeetingServiceSeam_CreateListNotifications`, `TestMeetingServiceSeam_Errores`.
