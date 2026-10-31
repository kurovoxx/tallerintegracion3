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

## Tests
- `internal/handler/http/group_handler_test.go`: `TestGetGroupHidesInviteTokenFromNonAdmins` (miembro no recibe invitación).
- `internal/service/group_service_test.go`: `TestGetGroup`.
- `internal/handler/http/groups_postgres_e2e_test.go`: `TestGroupsPostgresE2E_MembershipLifecycle` (401 sin token, 403 ajeno, invitación solo admin).
