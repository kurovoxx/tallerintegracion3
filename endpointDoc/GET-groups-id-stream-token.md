# GET /groups/:id/stream-token

Emite el token de usuario de Stream Chat y el canal del grupo para que el cliente se conecte directo a Stream (microservicio `social`).

## Req
- Método: `GET`, ruta: `/groups/:id/stream-token` (protegida con JWT `Bearer`; solo miembros; `back/social/cmd/server/main.go:278`).
- Headers: `Authorization: Bearer <access_token>`.
- Path: `id` (uuid del grupo).

## Res
- `200`: `{"token":"<jwt Stream>","channel_id":"<canal>","api_key":"<api key pública>"}`. `api_key` puede venir vacía si el servidor no la tiene. El secret de Stream nunca sale del backend.
- `503` `stream_not_configured` (sin `STREAM_SECRET`) o `stream_sync_failed` (Stream rechazó crear el canal o agregar al miembro). `403` `forbidden`: no-miembro (no se llama a Stream). `404` `group_not_found`. `400` `invalid_group_id`. `401`: sin token.

## Flujo
- `StreamHandler.Token` (`internal/handler/http/stream_handler.go:26`) → `StreamTokenService.IssueTokenWithKey` (`internal/service/stream_token_service.go:68`):
  1. Grupo existe y el usuario es miembro (fuente de verdad: PostgreSQL de Social).
  2. Canal: el guardado en `social.stream_channels` o `ChannelIDForGroup(groupID)`.
  3. `ensureChannelMember`: crea el canal si no existe y agrega al usuario (idempotente; `STREAM_MODE=mock` no usa red).
  4. `stream.SignUserToken` firma `{"user_id"}` con el secret.

## Notas
- El token no tiene `exp`. Lo que corta el acceso es la membresía del canal: al salir, ser expulsado, baneado o borrar la cuenta, Social quita al usuario del canal en Stream (best-effort). Si eso falla, queda registrado en el log `stream sync: remove-member` y el exmiembro conserva acceso: no hay reintento automático.

## Tests
- `internal/handler/http/stream_handler_test.go`: `TestStreamToken_Contrato`, `TestStreamToken_SinSecret_503`, `TestStreamToken_SyncFalla_503`.
- `internal/service/stream_token_provision_test.go`: `TestProvision_*` (canal existente, huérfano, idempotencia, ajeno sin llamadas a Stream).
- `test/social_routes_e2e_test.go`: subtest `meetings_notifications_discord_and_stream`.
