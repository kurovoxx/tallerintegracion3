# `PATCH /groups/:id/members/:user_id/role`


## Req

- `PATCH /groups/:id/members/:user_id/role` con JWT: `back/social/internal/handler/http/group_handler.go:25-36`.
- :id y :user_id son UUID. Body requerido {"role":"admin"} o {"role":"member"}. Solo un admin puede cambiar el rol; no puede modificarse a sí mismo.
- No body/query alternativos.

## Res

- 200 sin body.
- 400 rol inválido o intento de modificarse a sí mismo; 401 sesión no válida; 403 no admin/target protegido; 404 grupo o miembro no encontrado; 500 para error interno. respondGroupError usa {"error":"mensaje"}; auth previo usa el envelope común.
- Actualiza rol de la membresía; no transfiere owner_user_id ni es el endpoint de transferir administración.

## Invocación (Front / Otros Servicios)

- `_GroupMembersTabState._confirmRole` confirma promoción/degradación y prepara la llamada (`front/lib/features/groups/group_members_tab.dart:230-280`).
- `SocialService.changeMemberRole` envía PATCH y valida `200` (`front/lib/core/services/social_service.dart:265-289`).
- No se encontró consumidor backend.

## Flujo

1. `main()` monta `GroupHandler.RegisterRoutes` bajo JWT (`back/social/cmd/server/main.go:271-275`; `back/social/internal/handler/http/group_handler.go:23-38`).
2. `GroupHandler.ChangeRole` valida el body y toma el actor del JWT (`back/social/internal/handler/http/group_handler.go:240-256`).
3. `GroupService.ChangeRole` comprueba la acción y bloquea cambios al objetivo que esté en borrado; `GroupRepository.ChangeMemberRole` persiste el nuevo rol (`back/social/internal/service/group_service.go:559-566`; `back/social/internal/repository/group_repository.go:352-380`).
4. El handler responde `200` sin body; `SocialService.changeMemberRole` acepta ese status y `_confirmRole` recarga los miembros (`back/social/internal/handler/http/group_handler.go:251-256`; `front/lib/core/services/social_service.dart:265-289`; `front/lib/features/groups/group_members_tab.dart:230-280`).

## Front
- Cliente: `SocialService.changeMemberRole` (`front/lib/core/services/social_service.dart:265`); valida `role` ∈ `admin|member` antes de enviar; espera `200` y no lee el body.
- Pantalla: `_GroupMembersTabState._confirmRole` (`front/lib/features/groups/group_members_tab.dart:237`) HACER ADMIN (`role: admin`) y `:247` QUITAR ADMIN (`role: member`).
- Efecto: snackbar `<nombre> ahora es admin|miembro.` y recarga de integrantes.

## Tests

- `back/social/internal/handler/http/group_handler_test.go:266`, `TestChangeRoleContract` cubre admin, no admin, target y rol.
- `back/social/internal/handler/http/groups_postgres_e2e_test.go:28` cubre ruta de grupo con PostgreSQL.
