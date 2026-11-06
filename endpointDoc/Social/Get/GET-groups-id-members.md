# `GET /groups/:id/members`


## Req

- `GET /groups/:id/members`, JWT requerido: `back/social/internal/handler/http/group_handler.go:25-36`.
- :id UUID. Sin body/query. Solo miembros del grupo pueden listar.
- El prefijo externo `/api-social` no forma parte de la ruta Gin.

## Res

- `200`: array de miembros con id, group_id, user_id, role, joined_at; display_name y email aparecen cuando Identity los resuelve.
- El enriquecimiento falla best-effort: devuelve la lista base sin esos campos, no falla la lectura del grupo.
- 401 sin JWT; 403 no miembro; 404 grupo/UUID inexistente; errores de almacenamiento se mapean como interno. respondGroupError devuelve {"error":"mensaje"}; el middleware de auth usa {"error":{"code":"...","message":"..."}}.

## Invocación (Front / Otros Servicios)

- `front/lib/core/services/social_service.dart:191`, `listMembers` analiza la lista.
- Consumidores de interfaz comprobados: `_GroupMembersTabState._load` (`front/lib/features/groups/group_members_tab.dart:65-91`), `_GroupDetailScreenState._loadHeader` al contar miembros (`front/lib/features/groups/group_detail_screen.dart:108-129`), `_GroupsScreenState._loadInitials` (`front/lib/features/groups/groups_screen.dart:104-120`), `_ScheduleMeetingScreenState._loadMembers` (`front/lib/features/workspace/schedule_meeting_screen.dart:135-146`), `_KanbanScreenState._loadReal` y `_loadMembers` (`front/lib/features/workspace/kanban_screen.dart:108-173,177-186`) y `_SprintSheetScreenState._loadRealSupport` y `_loadMembers` (`front/lib/features/workspace/sprint_sheet_screen.dart:198-205,411-423`).
- Social invoca Identity LookupUsers vía GRPCUserDirectory; configuración en `back/social/cmd/server/main.go:104` y llamada en `back/social/internal/identity/grpc_directory.go:28`.

## Flujo

1. `main()` registra `GET /groups/:id/members` bajo JWT (`back/social/cmd/server/main.go:274`).
2. `GroupHandler.ListMembers` lee el actor y grupo, valida la respuesta y serializa JSON (`back/social/internal/handler/http/group_handler.go:184-198`).
3. `GroupService.ListMembersEnriched` comprueba membresía y obtiene la lista (`back/social/internal/service/group_service.go:427-435`); `GroupRepository.ListMembers` lee las filas de membresía (`back/social/internal/repository/group_repository.go:211-231`).
4. `GroupService.enrichMembers` consulta datos públicos con `GRPCUserDirectory.LookupUsers` (`back/social/internal/service/group_service.go:468-484`; `back/social/internal/identity/grpc_directory.go:28-45`). Auth resuelve el RPC en `IdentityServer.LookupUsers` y `UsersService.Lookup` (`back/auth/internal/handler/grpc/identity_server.go:76-99`; `back/auth/internal/service/users_service.go:30-53`). Si falla esa consulta, Social conserva la lista base.
5. El handler responde `200`; `SocialService.listMembers` convierte el array a modelos Flutter (`front/lib/core/services/social_service.dart:191-207`) y las pantallas citadas arriba lo usan para miembros, responsables o conteos.

## Front
- Cliente: `SocialService.listMembers` (`front/lib/core/services/social_service.dart:191`) → `List<GroupMember>` (acepta el array JSON directo; campos `id, group_id, user_id, role, joined_at, display_name?, email?`).
- Pestaña INTEGRANTES: `_GroupMembersTabState._load` (`front/lib/features/groups/group_members_tab.dart:80`); ordena admins primero y luego por nombre; la ficha muestra nombre, correo, rol, fecha `joined_at` e ID.
- Tarjetas de grupo: `_GroupsScreenState._loadInitials` (`front/lib/features/groups/groups_screen.dart:108`); toma los 4 primeros para las iniciales del avatar-stack.
- Cabecera del grupo: `_loadHeader` (`front/lib/features/groups/group_detail_screen.dart:126`); usa solo `members.length` como conteo.
- Selectores de asignado/asistentes: Kanban (`front/lib/features/workspace/kanban_screen.dart:154`, `:180`), Sprint (`front/lib/features/workspace/sprint_sheet_screen.dart:202`, `:414`), Reunión (`front/lib/features/workspace/schedule_meeting_screen.dart:138`).
- Chat: `_GroupChatScreenState._loadMemberNames` (`front/lib/features/chat/group_chat_screen.dart:180`); mapa `user_id → displayLabel` para nombrar mensajes.
- `displayLabel` = `display_name` → `email` → id corto; nunca muestra un UUID completo.

## Tests

- `back/social/internal/handler/http/group_handler_test.go:219`, `TestMembersContract` valida acceso y forma base.
- `back/social/internal/service/group_members_enriched_test.go:40`, `nombres` disponibles; :73, Identity caído degrada; :87, sin directorio conserva compatibilidad.
- `back/social/internal/handler/http/groups_postgres_e2e_test.go:28` cubre autorización/ruta con PostgreSQL.
