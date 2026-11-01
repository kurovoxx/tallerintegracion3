# GET /groups/:id

Trae los datos de un grupo; el `invite_token` solo se muestra al admin (microservicio `social`).

## Req
- Método: `GET`, ruta: `/groups/:id` (protegida con JWT `Bearer`; solo miembros; `back/social/cmd/server/main.go:224`).
- Headers: `Authorization: Bearer <access_token>`.
- Path: `id` (uuid del grupo, requerido).

## Res
- `200`: `{"id":"<uuid>","name":"...","description":"...","owner_user_id":"<uuid>","created_at":"RFC3339","role":"admin|member"}` + `"invite_token":"<uuid>"` solo si `role` es `admin` (`GroupView`).
- `403`: no-miembro. `404`: id inválido o grupo inexistente. `401`: sin token.

## Flujo
- `GroupHandler.Get` (`internal/handler/http/group_handler.go:116`) → `GroupService.GetGroup` (`internal/service/group_service.go:170`: valida uuid, exige membresía, oculta invitación a no-admins) → `GroupRepository.GetByID` (`SELECT social.groups`, `internal/repository/group_repository.go:60`) + `GetMemberRole`.

## Front
- Cliente: `SocialService.getGroup` (`front/lib/core/services/social_service.dart:130`) → `GroupDetail.fromJson` (`front/lib/core/models/social_models.dart`).
- Pantalla grupo (cabecera): `_GroupDetailScreenState._loadHeader` (`front/lib/features/groups/group_detail_screen.dart:117`); muestra `name` y, aparte, el conteo de integrantes sacado de `GET /groups/:id/members` (no viene en este response).
- Diálogo INFO de la tarjeta de grupo: `_GroupInfoDialogState._load` (`front/lib/features/groups/groups_screen.dart:552`); muestra descripción y rol.
- Diálogo INVITAR: `_InviteDialogState._load` (`front/lib/features/groups/groups_screen.dart:797`); lee solo `invite_token` (llega únicamente si `role == admin`) para mostrarlo y copiarlo.
- Pestaña Discord: `_GroupDiscordTabState._loadRole` (`front/lib/features/groups/group_detail_screen.dart:482`); usa solo `role == 'admin'` para habilitar el botón CONFIGURAR DISCORD.
- El modelo también parsea `notes_restricted_to_staff` (bool, default `false`); el backend lo devuelve aunque no figure en Res.
- Error: cualquier fallo se muestra como mensaje genérico (`No se pudo cargar el grupo.` / `la información del grupo.` / `la invitación.`).

## Tests
- `internal/handler/http/group_handler_test.go`: `TestGetGroupHidesInviteTokenFromNonAdmins` (miembro no recibe invitación).
- `internal/service/group_service_test.go`: `TestGetGroup`.
- `internal/handler/http/groups_postgres_e2e_test.go`: `TestGroupsPostgresE2E_MembershipLifecycle` (401 sin token, 403 ajeno, invitación solo admin).
