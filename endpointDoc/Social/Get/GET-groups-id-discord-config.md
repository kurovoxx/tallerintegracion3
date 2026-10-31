# GET /groups/:id/discord-config

Devuelve la integración de Discord del grupo (nombre del servidor e invitación) (microservicio `social`).

## Req
- Método: `GET`, ruta: `/groups/:id/discord-config` (protegida con JWT `Bearer`; solo miembros; `back/social/internal/handler/http/discord_handler.go:27`).
- Headers: `Authorization: Bearer <access_token>`.
- Path: `id` (uuid del grupo).

## Res
- `200`: `{"server_name":"...","invite_url":"...","webhook_url":"https://discord.com/api/webhooks/..."|null}`.
  - `webhook_url` solo llega a admins. Es un secreto: quien lo tenga puede publicar en el canal. Para miembros siempre es `null`.
- `404` `discord_not_configured`: el grupo nunca se configuró. `404` `group_not_found`. `403` `forbidden`: no-miembro. `400` `invalid_group_id`. `401`: sin token.

## Flujo
- `DiscordHandler.GetConfig` (`internal/handler/http/discord_handler.go:34`) → `DiscordService.GetConfig` (`internal/service/discord_service.go:156`: grupo existe, rol del usuario, lee la config y borra el webhook si el rol no es `admin`) → `DiscordRepository.GetConfigByGroup` (tabla `social.discord_integrations`).

## Tests
- `internal/handler/http/discord_handler_test.go`: `TestGetDiscordConfig_ContractAndMembership` (401/403/400/404, admin ve el webhook, miembro recibe `null`).
- `internal/service/discord_service_test.go`: `TestDiscordGetConfig_MiembroLeeYExtranoNo`.
- `test/social_routes_e2e_test.go`: subtest `meetings_notifications_discord_and_stream`.
