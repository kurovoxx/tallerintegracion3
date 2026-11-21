# `POST /groups/join`


## Req

- `POST /groups/join`, JWT requerido; registro en `back/social/internal/handler/http/group_handler.go` (`RegisterRoutes`, `JoinByCode`).
- JSON requerido: `{"invite_token":"<código de invitación>"}`. Solo el código: **no** se envía el id del grupo.
- El código es el `invite_token` del grupo (UUID v4 aleatorio, `UNIQUE` en `social.groups`), por eso identifica al grupo por sí solo. Se aceptan espacios alrededor y mayúsculas.
- El `user_id` del nuevo miembro sale del JWT.

## Res

- `201` `{"group_id","name","already_member":false}` al crear la membresía (rol `member`).
- `200` `{"group_id","name","already_member":true}` si el usuario ya pertenecía al grupo (idempotente: no duplica la membresía ni cambia el rol).
- `400` sin body o con código vacío; `401` sin JWT; `403` si el usuario está baneado del grupo; `404` si el código no existe, está mal formado o fue reemplazado con `POST /groups/:id/invite/regenerate`. Formato de error `{"error":"mensaje"}` (`respondGroupError`).

## Flujo

1. `GroupHandler.JoinByCode` valida el body y llama a `GroupService.JoinByInviteCode`.
2. El servicio normaliza el código (UUID canónico) y busca el grupo con `FindByInviteToken` (`SELECT … WHERE invite_token = $1`).
3. Delega en `JoinWithInviteToken(groupID, token, user)`, el mismo camino que `POST /groups/:id/join`: bloquea la fila del grupo (`FOR UPDATE`), revisa el baneo, **vuelve a validar el token bajo el lock** (si un admin lo regeneró entretanto, responde 404) e inserta con `ON CONFLICT DO NOTHING`.

## Compatibilidad

- `POST /groups/:id/join` (id + código) **se mantiene** sin cambios para clientes anteriores, scripts y pruebas.
- Los códigos ya generados siguen sirviendo: no hubo migración ni cambio de formato.
- El código no vence; un admin puede invalidarlo generando uno nuevo.

## Front

- `SocialService.joinGroupByCode` (`front/lib/core/services/social_service.dart`) envía el código y devuelve `JoinGroupResult`.
- `_GroupsScreenState._openJoinDialog` (`front/lib/features/groups/groups_screen.dart`): diálogo «Unirse a grupo» con **un solo campo**. `extractInviteCode` (`social_models.dart`) toma el UUID del texto pegado; si hay dos (formato antiguo «id código» del botón «Copiar datos de invitación»), usa el último.
- `_InviteDialog` (solo admin) muestra únicamente el código, con «Copiar código» y «Generar nuevo código».

## Tests

- `back/social/internal/handler/http/group_handler_test.go` `TestJoinByCodeContract`.
- `back/social/internal/service/group_service_test.go` `TestJoinByInviteCode`.
- `back/social/internal/repository/group_repository_integration_test.go` `TestIntegrationFindByInviteToken` (requiere `SOCIAL_TEST_DATABASE_URL`).
- `front/test/invite_code_test.dart`; `front/test/iteracion_vistas_reales_test.dart` (grupo «CHECKPOINT — Invite/Info/Join»).
