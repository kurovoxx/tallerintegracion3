# POST /groups/:id/members/:user_id/kick

Un admin expulsa a un miembro (no admin); puede volver con una invitación vigente (microservicio `social`).

## Req
- Método: `POST`, ruta: `/groups/:id/members/:user_id/kick` (protegida con JWT `Bearer`; solo admin; `back/social/cmd/server/main.go:224`).
- Headers: `Content-Type: application/json`, `Authorization: Bearer <access_token>`.
- Path: `id` (uuid del grupo), `user_id` (uuid del objetivo).
- Body opcional: `{"cleanup_shared_notes":true}` (defecto `false`; con `true` retira en Notes los apuntes del expulsado, best-effort con el JWT del admin).

## Res
- `204`: sin body.
- `400`: autoexpulsión u objetivo admin. `403`: solicitante no-admin. `404`: grupo u objetivo inexistente. `401`: sin token.

## Flujo
- `GroupHandler.KickMember` (`internal/handler/http/group_handler.go:200`: flag opcional + token crudo) → `GroupService.KickMember` (`internal/service/group_service.go:453`: valida ids, borra y dispara limpieza en goroutine si se pidió) → `GroupRepository.KickMember` (lock `FOR UPDATE`, exige admin, prohíbe auto-kick y kick a admins, `DELETE social.group_memberships`; `internal/repository/group_repository.go:289`). No toca `banned_users`.

## Tests
- `internal/handler/http/group_handler_test.go`: `TestKickAndBanContract` (permisos, self, inexistente).
- `internal/service/group_service_test.go`: `TestKick`.
- `internal/handler/http/groups_postgres_e2e_test.go`: `TestGroupsPostgresE2E_MembershipLifecycle` (403 miembro, 204 admin, reingreso permitido).
