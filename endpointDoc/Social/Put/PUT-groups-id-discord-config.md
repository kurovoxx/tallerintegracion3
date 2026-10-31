# PUT /groups/:id/discord-config

Crea o reemplaza la integración de Discord del grupo (microservicio `social`). Con `webhook_url`, las reuniones nuevas se avisan en el canal.

## Req
- Método: `PUT`, ruta: `/groups/:id/discord-config` (protegida con JWT `Bearer`; solo admin; `back/social/internal/handler/http/discord_handler.go:28`).
- Headers: `Content-Type: application/json`, `Authorization: Bearer <access_token>`.
- Path: `id` (uuid del grupo).
- Body (`PutDiscordConfigRequest`, `discord_handler.go:81-85`): `{"server_name":"Mi server","invite_url":"https://discord.gg/abc","webhook_url":"https://discord.com/api/webhooks/<id>/<token>"}`.
  - `server_name` (máx. 200) e `invite_url` (máx. 500) obligatorios y no vacíos.
  - `webhook_url` opcional; vacío = sin notificaciones (`NULL`). Si viene debe ser `https`, host de Discord y ruta `/api/webhooks/...`.

## Res
- `200`: `{"message":"discord config saved successfully","server_name","invite_url","webhook_url"|null}`.
- `400` `invalid_body` (campos vacíos, largos o webhook no-Discord) o `invalid_group_id`. `403` `forbidden`: no-admin o no-miembro. `404` `group_not_found`. `401`: sin token.

## Flujo
- `DiscordHandler.PutConfig` (`internal/handler/http/discord_handler.go:87`) → `DiscordService.PutConfig` (`internal/service/discord_service.go:105`: valida campos, exige rol `admin`) → `UpsertDiscordConfig` (sqlc; un registro por grupo, el segundo PUT pisa).
- Consumidor: `DiscordMeetingNotifier` lee el webhook al crear una reunión (`internal/service/meeting_discord_notifier.go`).

## Tests
- `internal/handler/http/discord_handler_test.go`: `TestPutDiscordConfig_Contrato`.
- `internal/service/discord_service_test.go`: `TestDiscordPutConfig_AdminGuarda`, `TestDiscordPutConfig_Permisos`, `TestDiscordPutConfig_Validacion`.
- `test/social_routes_e2e_test.go`: subtest `meetings_notifications_discord_and_stream` (403 miembro, 200 admin).
