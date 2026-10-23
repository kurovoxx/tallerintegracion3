# POST /groups/:id/members/:user_id/ban

Un admin expulsa y prohíbe el reingreso (kick + registro en `banned_users`) (microservicio `social`).

## Req
- Método: `POST`, ruta: `/groups/:id/members/:user_id/ban` (protegida con JWT `Bearer`; solo admin; `back/social/cmd/server/main.go:224`).
- Headers: `Content-Type: application/json`, `Authorization: Bearer <access_token>`.
- Path: `id` (uuid del grupo), `user_id` (uuid del objetivo).
- Body opcional: `{"cleanup_shared_notes":true}` (igual que kick).

## Res
- `204`: sin body (el join posterior del baneado da `403`). No hay ruta unban en esta base.
- `400`: auto-baneo u objetivo admin. `403`: solicitante no-admin. `404`: grupo u objetivo inexistente. `401`: sin token.

## Flujo
- `GroupHandler.BanMember` (`internal/handler/http/group_handler.go:219`) → `GroupService.BanMember` (`internal/service/group_service.go:468`) → `GroupRepository.BanMember` (mismas reglas y lock que kick + `INSERT INTO social.banned_users (group_id, user_id, banned_by_user_id)` con `ON CONFLICT DO NOTHING`; `internal/repository/group_repository.go:315`).
- Después de confirmar la salida, `GroupService.removeFromChat` quita al usuario del canal Stream `group-<id>` (best-effort, en segundo plano; un fallo de Stream solo se registra y no cambia el status).

## Tests
- `internal/handler/http/group_handler_test.go`: `TestKickAndBanContract`, `TestBannedUserIsolationEndToEnd`.
- `internal/service/group_service_test.go`: `TestBan`.
- `internal/handler/http/groups_postgres_e2e_test.go`: `TestGroupsPostgresE2E_MembershipLifecycle` (`banned_by_user_id` verificado por SQL, join posterior 403).
