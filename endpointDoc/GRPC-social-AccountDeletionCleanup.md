# gRPC Social.AccountDeletionCleanup

Saca al usuario de **todos** sus grupos con sucesión en cada uno. Es la parte Social del borrado de cuenta (no borra Identity ni apuntes). **Operación gRPC, no endpoint HTTP**: el antiguo `POST /groups/account-deletion` fue eliminado (`group_handler.go:37`).

## Req
- RPC: `social.v1.SocialService/AccountDeletionCleanup` (`back/proto/social/v1/social.proto`, servidor en `back/social/cmd/server/main.go:107-129`).
- Auth: metadata `x-internal-key` (la exige el interceptor `internal/interceptor`, no el handler).
- Request: `AccountDeletionCleanupRequest{user_id:"<uuid>"}` (uuid requerido).

## Res
- OK: `AccountDeletionCleanupResponse{groups}` con una entrada por grupo (`group_id`, `promoted_user_id?`, `group_deleted`); vacía si no tenía grupos (idempotente). Sin transacción global entre grupos.
- `InvalidArgument`: `user_id` inválido. `Unauthenticated`: sin clave interna.

## Flujo
- `SocialServer.AccountDeletionCleanup` (`internal/handler/grpc/social_server.go:85`: valida uuid) → `GroupService.HandleAccountDeletion` (`internal/service/group_service.go:541`: recorre grupos, registra resultado) → `GroupRepository.HandleAccountDeletion` (lista membresías y aplica `removeUserFromGroup(strict=false)` por grupo + limpia `banned_users`; `internal/repository/group_repository.go:490,527`).

## Tests
- `internal/handler/grpc/social_server_test.go`: `TestSocialServer/AccountDeletionCleanup promueve y es idempotente` (con memoria) + `TestAccountDeletionNotExposedOverHTTP` en `group_handler_test.go:314` (la ruta HTTP ya no existe).
- `internal/service/group_service_test.go`: `TestAccountDeletionPromotesOldestMember`, `TestAccountDeletionLastMemberDeletesGroup`, `TestAccountDeletionWithoutGroupsIsNoOpAndIdempotent`, `TestAccountDeletionAcrossSeveralGroups` (+ resto `TestAccountDeletion*`).
- `internal/handler/http/groups_postgres_e2e_test.go`: `TestGroupsPostgresE2E_MembershipLifecycle` (cierra por bufconn contra el mismo PostgreSQL: borrado en cascada e idempotencia por SQL).
