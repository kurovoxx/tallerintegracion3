# POST /groups/:id/invite/regenerate

Reemplaza el código de invitación del grupo; el anterior deja de servir (microservicio `social`).

## Req
- Método: `POST`, ruta: `/groups/:id/invite/regenerate` (protegida con JWT `Bearer`; solo admin; `back/social/cmd/server/main.go:224`).
- Headers: `Authorization: Bearer <access_token>`.
- Path: `id` (uuid del grupo). Sin body.

## Res
- `200`: `{"new_invite_token":"<uuid>"}`.
- `403`: no-admin o no-miembro. `404`: grupo inexistente. `401`: sin token.

## Flujo
- `GroupHandler.RegenerateInvite` (`internal/handler/http/group_handler.go:167`) → `GroupService.RegenerateInvite` (`internal/service/group_service.go:262`) → `GroupRepository.RegenerateInviteToken` (verifica rol `admin`, `UPDATE social.groups SET invite_token = gen_random_uuid()` en transacción; `internal/repository/group_repository.go:156`). No borra miembros.

## Tests
- `internal/handler/http/group_handler_test.go`: `TestRegenerateInviteContract` (403 miembro, token distinto, viejo inválido).
- `internal/service/group_service_test.go`: `TestRegenerateInvite`.
- `internal/handler/http/groups_postgres_e2e_test.go`: `TestGroupsPostgresE2E_MembershipLifecycle` (rotación verificada por HTTP).
