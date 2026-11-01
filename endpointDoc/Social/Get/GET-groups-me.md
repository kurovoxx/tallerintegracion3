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

## Front
- **El front no llama este endpoint.** Para listar los grupos del usuario usa `GET /me/overview` (`SocialService.getOverview`, `front/lib/core/services/social_service.dart:84`), que devuelve además `sidebar.groups`, `stats` y `member_count` por grupo.
- Consumidores del overview: `_GroupsScreenState._load` (`front/lib/features/groups/groups_screen.dart:81`, tarjetas de grupo) y el sidebar de `MainShell` (`front/lib/core/widgets/main_shell.dart:111`, primeros 5 grupos y contador).

## Tests
- `internal/handler/http/group_handler_test.go`: `TestListMyGroupsContract`.
- `internal/service/group_service_test.go`: `TestListMyGroups`.
- `internal/handler/http/groups_postgres_e2e_test.go`: `TestGroupsPostgresE2E_MembershipLifecycle` (el miembro lista exactamente su grupo).
