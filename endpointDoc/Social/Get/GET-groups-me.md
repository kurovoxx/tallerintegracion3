# GET /groups/me

Devuelve la lista mínima de grupos del usuario (id, nombre y rol) (microservicio `social`).

## Req
- Método: `GET`, ruta: `/groups/me` (protegida con JWT `Bearer`; `back/social/cmd/server/main.go:224`).
- Headers: `Authorization: Bearer <access_token>`. Sin body, query ni path.

## Res
- `200`: `[{"group_id":"<uuid>","name":"...","role":"admin|member"}]` (vacío `[]` si no tiene grupos).
- `401`: sin token o token inválido. `500`: `internal`.

## Flujo
- `GroupHandler.ListMy` (`internal/handler/http/group_handler.go:130`) → `GroupService.ListMyGroups` (`internal/service/group_service.go:204`, `nil` → `[]`) → `GroupRepository.ListMyGroups` (`JOIN social.groups + social.group_memberships`, orden `created_at DESC`, `internal/repository/group_repository.go:80`).

## Tests
- `internal/handler/http/group_handler_test.go`: `TestListMyGroupsContract`.
- `internal/service/group_service_test.go`: `TestListMyGroups`.
- `internal/handler/http/groups_postgres_e2e_test.go`: `TestGroupsPostgresE2E_MembershipLifecycle` (el miembro lista exactamente su grupo).
