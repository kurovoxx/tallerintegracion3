# `GET /groups/:id/members`


## Req

- `GET /groups/:id/members?limit=N`, JWT requerido: `back/social/internal/handler/http/group_handler.go`.
- :id UUID. `limit` es opcional (1..200): la vista previa usa 3; sin `limit` devuelve todos. Solo miembros del grupo pueden listar.
- El prefijo externo `/api-social` no forma parte de la ruta Gin.

## Res

- `200`: array de miembros con id, group_id, user_id, role, joined_at; display_name, email y photo_thumb_url aparecen cuando Identity los resuelve.
- `photo_thumb_url` es la miniatura 96x96 generada al actualizar la foto; evita enviar la foto completa en listados. Si un perfil antiguo solo tiene URL remota corta, se usa esa URL.
- Con `limit=N` el tope se aplica en SQL y antes del lookup a Identity; los campos públicos se resuelven solo para esos N.
- El enriquecimiento falla best-effort: devuelve la lista base sin esos campos, no falla la lectura del grupo.
- 400 `limit` inválido; 401 sin JWT; 403 no miembro; 404 grupo/UUID inexistente; errores de almacenamiento se mapean como interno. respondGroupError devuelve {"error":"mensaje"}; el middleware de auth usa {"error":{"code":"...","message":"..."}}.

## Invocación (Front / Otros Servicios)

- `front/lib/core/services/social_service.dart`, `listMembers(groupId, {limit})` analiza la lista y envía `?limit=` cuando corresponde.
- Consumidores de interfaz: `_GroupMembersTabState._load` (sin `limit`, lista completa), `_GroupsScreenState._loadPreviewMembers` (con `limit: 3`), y `SocialService.listMembers` en Kanban, Sprint, Reunión y Chat.
- Social invoca Identity LookupUsers vía GRPCUserDirectory; configuración en `back/social/cmd/server/main.go:104` y llamada en `back/social/internal/identity/grpc_directory.go:28`.

## Flujo

1. `main()` registra `GET /groups/:id/members` bajo JWT (`back/social/cmd/server/main.go:274`).
2. `GroupHandler.ListMembers` lee el actor y grupo, valida la respuesta y serializa JSON (`back/social/internal/handler/http/group_handler.go:184-198`).
3. `GroupService.ListMembersEnrichedLimit` comprueba membresía, aplica el tope y obtiene la lista; `GroupRepository.ListMembersLimit` usa `LIMIT` cuando corresponde.
4. `GroupService.enrichMembers` consulta datos públicos con `GRPCUserDirectory.LookupUsers` (`back/social/internal/service/group_service.go:468-484`; `back/social/internal/identity/grpc_directory.go:28-45`). Auth resuelve el RPC en `IdentityServer.LookupUsers` y `UsersService.Lookup` (`back/auth/internal/handler/grpc/identity_server.go:76-99`; `back/auth/internal/service/users_service.go:30-53`). Si falla esa consulta, Social conserva la lista base.
5. El handler responde `200`; `SocialService.listMembers` convierte el array a modelos Flutter (`front/lib/core/services/social_service.dart:191-207`) y las pantallas citadas arriba lo usan para miembros, responsables o conteos.

## Front
- Cliente: `SocialService.listMembers` → `List<GroupMember>` (array JSON directo; campos `id, group_id, user_id, role, joined_at, display_name?, email?, photo_thumb_url?`).
- Pestaña INTEGRANTES: `_GroupMembersTabState._load` (`front/lib/features/groups/group_members_tab.dart:80`); ordena admins primero y luego por nombre; la ficha muestra nombre, correo, rol, fecha `joined_at` e ID.
- Tarjetas de grupo: `_GroupsScreenState._loadPreviewMembers` pide `limit: 3` y muestra miniatura o iniciales con `UserAvatar`; el resto se carga al abrir MIEMBROS.
- Cabecera del grupo: `_loadHeader` usa `member_count` de `GET /groups/:id`, sin traer la lista completa.
- Pestaña MIEMBROS: `UserAvatar` usa `photo_thumb_url` con fallback a iniciales.
- Selectores de asignado/asistentes: Kanban (`front/lib/features/workspace/kanban_screen.dart:154`, `:180`), Sprint (`front/lib/features/workspace/sprint_sheet_screen.dart:202`, `:414`), Reunión (`front/lib/features/workspace/schedule_meeting_screen.dart:138`).
- Chat: `_GroupChatScreenState._loadMemberNames` (`front/lib/features/chat/group_chat_screen.dart:180`); mapa `user_id → displayLabel` para nombrar mensajes.
- `displayLabel` = `display_name` → `email` → id corto; nunca muestra un UUID completo.

## Tests

- `back/social/internal/handler/http/group_handler_test.go`, `TestMembersContract` valida acceso, forma base, `limit=1` y límites inválidos.
- `front/test/group_tasks_stream_test.dart` valida `?limit=3` y `photo_thumb_url`.
- `back/social/internal/service/group_members_enriched_test.go:40`, `nombres` disponibles; :73, Identity caído degrada; :87, sin directorio conserva compatibilidad.
- `back/social/internal/handler/http/groups_postgres_e2e_test.go:28` cubre autorización/ruta con PostgreSQL.
