# `GET /groups/:id`


## Req

- `GET /groups/:id` con JWT; registro compartido: `back/social/internal/handler/http/group_handler.go:25-36`.
- id debe ser UUID. Sin query ni body. Solo miembros pueden leer.
- Prefijo `/api-social`, cuando aplica, pertenece al gateway; la ruta del servidor comienza `/groups.`

## Res

- `200`: GroupView con id, name, description?, owner_user_id, notes_restricted_to_staff, created_at y role. invite_token solo aparece para admin.
- 401 sesión no válida; 403 no miembro; 404 grupo ausente o UUID inválido; 500 error interno. respondGroupError devuelve {"error":"mensaje"}; el middleware de auth usa {"error":{"code":"...","message":"..."}}.
- No cambia datos.

## Invocación (Front / Otros Servicios)

- `SocialService.getGroup` envía el GET (`front/lib/core/services/social_service.dart:130-145`). Lo llaman `_GroupDetailScreenState._loadHeader` (`front/lib/features/groups/group_detail_screen.dart:108-123`), `_GroupInfoDialogState._load` (`front/lib/features/groups/groups_screen.dart:545-565`) y `_InviteDialogState._load` (`front/lib/features/groups/groups_screen.dart:790-810`).
- No se encontró consumidor de producción desde otro microservicio.

## Flujo

1. `main()` registra `GET /groups/:id` (`back/social/cmd/server/main.go:274`).
2. `GroupHandler.Get` lee el actor y el ID del grupo (`back/social/internal/handler/http/group_handler.go:116-128`).
3. `GroupService.GetGroup` valida UUID y membresía, consulta `GroupRepository.GetByID` y `GroupRepository.GetMemberRole` y construye `GroupView` (`back/social/internal/service/group_service.go:242-274`; `back/social/internal/repository/group_repository.go:60-78,423-434`). Solo agrega el token de invitación al rol admin.
4. `GroupHandler.Get` serializa el detalle con `200` (`back/social/internal/handler/http/group_handler.go:116-128`); las pantallas citadas en Invocación lo cargan para mostrar el grupo o administrar su invitación.

## Front
- Cliente: `SocialService.getGroup` (`front/lib/core/services/social_service.dart:130`) → `GroupDetail.fromJson` (`front/lib/core/models/social_models.dart`).
- Pantalla grupo (cabecera): `_GroupDetailScreenState._loadHeader` (`front/lib/features/groups/group_detail_screen.dart:117`); muestra `name` y, aparte, el conteo de integrantes sacado de `GET /groups/:id/members` (no viene en este response).
- Diálogo INFO de la tarjeta de grupo: `_GroupInfoDialogState._load` (`front/lib/features/groups/groups_screen.dart:552`); muestra descripción y rol.
- Diálogo INVITAR: `_InviteDialogState._load` (`front/lib/features/groups/groups_screen.dart:797`); lee solo `invite_token` (llega únicamente si `role == admin`) para mostrarlo y copiarlo.
- Pestaña Discord: `_GroupDiscordTabState._loadRole` (`front/lib/features/groups/group_detail_screen.dart:482`); usa solo `role == 'admin'` para habilitar el botón CONFIGURAR DISCORD.
- El modelo también parsea `notes_restricted_to_staff` (bool, default `false`); el backend lo devuelve aunque no figure en Res.
- Error: cualquier fallo se muestra como mensaje genérico (`No se pudo cargar el grupo.` / `la información del grupo.` / `la invitación.`).

## Tests

- `back/social/internal/handler/http/group_handler_test.go:161`, `TestGetGroupHidesInviteTokenFromNonAdmins` cubre admin, miembro, no miembro y grupo inexistente.
- `back/social/internal/handler/http/groups_postgres_e2e_test.go:28` incluye la lectura en el ciclo real.
