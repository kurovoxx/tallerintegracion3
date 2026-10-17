# POST /groups/:id/leave

Saca al usuario del grupo con sucesión automática (si era el único admin, asume el miembro más antiguo; si era el último, el grupo se borra) (microservicio `social`).

## Req
- Método: `POST`, ruta: `/groups/:id/leave` (protegida con JWT `Bearer`; solo miembros; `back/social/cmd/server/main.go:224`).
- Headers: `Content-Type: application/json`, `Authorization: Bearer <access_token>`.
- Path: `id` (uuid del grupo). Body opcional (`leaveGroupRequest`, `group_handler.go:280-282`): `{"cleanup_shared_notes":true}` (defecto `false`; el binding se ignora a propósito).

## Res
- `204`: sin body (la limpieza de Notes es asíncrona y no se propaga: un fallo allá no cambia este status).
- `403`: no-miembro. `404`: grupo inválido. `401`: sin token.

## Flujo
- `GroupHandler.LeaveGroup` (`internal/handler/http/group_handler.go:285`: flag + token crudo para Notes) → `GroupService.LeaveGroup` (`internal/service/group_service.go:496`: salida atómica y luego `unshareNotes` en goroutine si `cleanup=true`) → `GroupRepository.LeaveGroup` → `removeUserFromGroup` (lock, borra membresía, promueve por `joined_at, user_id` o borra el grupo, reasigna `owner_user_id` si salió el dueño; `internal/repository/group_repository.go:527,610`).

## Tests
- `internal/handler/http/group_handler_test.go`: `TestLeaveContract` (sucesión, último elimina, 403 extraño).
- `internal/service/group_service_test.go`: `TestLeave`, `TestLeaveCleansSharedNotes`, `TestLeaveWithoutCleanupDoesNotCallNotes`, `TestLeaveSucceedsWhenNotesIsDown`, `TestLeaveSuccessionOrderAndOwner`.
- `internal/handler/http/groups_postgres_e2e_test.go`: `TestGroupsPostgresE2E_MembershipLifecycle` (sucesión verificada por SQL).
