# PATCH /groups/:id/members/:user_id/role

Un admin sube a `admin` o baja a `member` a otro integrante (microservicio `social`).

## Req
- Método: `PATCH`, ruta: `/groups/:id/members/:user_id/role` (protegida con JWT `Bearer`; solo admin; `back/social/cmd/server/main.go:224`).
- Headers: `Content-Type: application/json`, `Authorization: Bearer <access_token>`.
- Path: `id` (uuid del grupo), `user_id` (uuid del objetivo).
- Body (`changeRoleRequest`, `group_handler.go:236-238`): `{"role":"admin"}` o `{"role":"member"}`.

## Res
- `200`: sin body (vacío a propósito; no hacer `jsonDecode` obligatorio).
- `400`: body/rol inválido o cambio propio. `403`: solicitante no-admin (se verifica antes de validar el rol). `404`: grupo u objetivo inexistente. `401`: sin token.

## Flujo
- `GroupHandler.ChangeRole` (`internal/handler/http/group_handler.go:241`) → `GroupService.ChangeRole` (`internal/service/group_service.go:481`) → `GroupRepository.ChangeMemberRole` (exige admin primero, valida rol, prohíbe auto-cambio, `UPDATE social.group_memberships.role`; `internal/repository/group_repository.go:347`). No toca `owner_user_id`.

## Tests
- `internal/handler/http/group_handler_test.go`: `TestChangeRoleContract`.
- `internal/service/group_service_test.go`: `TestChangeRole`.
- `internal/handler/http/groups_postgres_e2e_test.go`: `TestGroupsPostgresE2E_MembershipLifecycle` (ascenso/descenso con rol verificado por SQL).
