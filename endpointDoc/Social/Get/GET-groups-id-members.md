# GET /groups/:id/members

Lista los integrantes con rol y fecha de ingreso; completa nombres si Identity responde (microservicio `social`).

## Req
- Método: `GET`, ruta: `/groups/:id/members` (protegida con JWT `Bearer`; solo miembros; `back/social/cmd/server/main.go:224`).
- Headers: `Authorization: Bearer <access_token>`.
- Path: `id` (uuid del grupo).

## Res
- `200`: `[{"user_id":"<uuid>","role":"admin|member","joined_at":"RFC3339","display_name":"...","email":"..."}]` (nombres opcionales; sin Identity igual devuelve ids y roles).
- `403`: ajeno. `404`: grupo inválido. `401`: sin token.

## Flujo
- `GroupHandler.ListMembers` (`internal/handler/http/group_handler.go:184`) → `GroupService.ListMembersEnriched` (`internal/service/group_service.go:355`: exige membresía, completa vía `LookupUsers` a `/internal/users/lookup` best-effort) → `GroupRepository.ListMembers` (`social.group_memberships` ordenado por `joined_at`, `internal/repository/group_repository.go:210`).

## Tests
- `internal/handler/http/group_handler_test.go`: `TestMembersContract`.
- `internal/service/group_service_test.go`: `TestListMembers` (+ `group_members_enriched_test.go` con/sin Identity).
- `internal/handler/http/groups_postgres_e2e_test.go`: `TestGroupsPostgresE2E_MembershipLifecycle` (2 miembros por HTTP).
