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

## Front
- Cliente: `SocialService.listMembers` (`front/lib/core/services/social_service.dart:191`) → `List<GroupMember>` (acepta el array JSON directo; campos `id, group_id, user_id, role, joined_at, display_name?, email?`).
- Pestaña INTEGRANTES: `_GroupMembersTabState._load` (`front/lib/features/groups/group_members_tab.dart:80`); ordena admins primero y luego por nombre; la ficha muestra nombre, correo, rol, fecha `joined_at` e ID.
- Tarjetas de grupo: `_GroupsScreenState._loadInitials` (`front/lib/features/groups/groups_screen.dart:108`); toma los 4 primeros para las iniciales del avatar-stack.
- Cabecera del grupo: `_loadHeader` (`front/lib/features/groups/group_detail_screen.dart:126`); usa solo `members.length` como conteo.
- Selectores de asignado/asistentes: Kanban (`front/lib/features/workspace/kanban_screen.dart:154`, `:180`), Sprint (`front/lib/features/workspace/sprint_sheet_screen.dart:202`, `:414`), Reunión (`front/lib/features/workspace/schedule_meeting_screen.dart:138`).
- Chat: `_GroupChatScreenState._loadMemberNames` (`front/lib/features/chat/group_chat_screen.dart:180`); mapa `user_id → displayLabel` para nombrar mensajes.
- `displayLabel` = `display_name` → `email` → id corto; nunca muestra un UUID completo.

## Tests
- `internal/handler/http/group_handler_test.go`: `TestMembersContract`.
- `internal/service/group_service_test.go`: `TestListMembers` (+ `group_members_enriched_test.go` con/sin Identity).
- `internal/handler/http/groups_postgres_e2e_test.go`: `TestGroupsPostgresE2E_MembershipLifecycle` (2 miembros por HTTP).
