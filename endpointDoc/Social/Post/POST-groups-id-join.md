# POST /groups/:id/join

Incorpora al usuario al grupo con el código de invitación, si no está baneado (microservicio `social`).

## Req
- Método: `POST`, ruta: `/groups/:id/join` (protegida con JWT `Bearer`; `back/social/cmd/server/main.go:224`).
- Headers: `Content-Type: application/json`, `Authorization: Bearer <access_token>`.
- Path: `id` (uuid del grupo). Body (`joinGroupRequest`, `group_handler.go:143-145`): `{"invite_token":"<uuid>"}` (requerido).

## Res
- `201`: sin body (idempotente: si ya era miembro, también `201` por `ON CONFLICT DO NOTHING`).
- `400`: body inválido. `403`: baneado. `404`: token incorrecto o grupo inválido. `401`: sin token.

## Flujo
- `GroupHandler.Join` (`internal/handler/http/group_handler.go:148`) → `GroupService.JoinGroup` (`internal/service/group_service.go:276`: valida ambos uuid antes de SQL) → `GroupRepository.JoinWithInviteToken` (transacción: revisa `social.banned_users`, compara `groups.invite_token`, inserta membresía `member`; `internal/repository/group_repository.go:111`).

## Front
- Cliente: `SocialService.joinGroup` (`front/lib/core/services/social_service.dart:850`); envía `{invite_token}`; acepta `200` o `201` y no lee el body.
- Pantalla: `_GroupsScreenState._openJoinDialog` (`front/lib/features/groups/groups_screen.dart:233`), diálogo UNIRSE A GRUPO con dos campos: identificador del grupo (`:id` de la ruta) y código de invitación (`invite_token`). No hay flujo de solo código.
- Efecto: `notifyGroupsChanged()` → recarga lista y sidebar. Snackbar `Te uniste al grupo.`; en error muestra el mensaje del backend.

## Tests
- `internal/handler/http/group_handler_test.go`: `TestJoinContract`, `TestBannedUserIsolationEndToEnd` (baneado no reingresa).
- `internal/service/group_service_test.go`: `TestJoin`.
- `internal/handler/http/groups_postgres_e2e_test.go`: `TestGroupsPostgresE2E_MembershipLifecycle` (token viejo 404, vigente 201, baneado 403).
