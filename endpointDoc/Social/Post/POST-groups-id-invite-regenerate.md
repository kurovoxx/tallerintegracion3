# `POST /groups/:id/invite/regenerate`


## Req

- `POST /groups/:id/invite/regenerate`, JWT requerido: `back/social/internal/handler/http/group_handler.go:25-36`.
- :id UUID. Sin body ni query. Solo el administrador vigente puede regenerar.
- No usar prefijo externo `/api-social` en la ruta Gin.

## Res

- `200`: {"new_invite_token":"<uuid nuevo>"}.
- El token anterior se invalida al guardar el nuevo.
- 401 sin JWT; 403 si no es admin; 404 grupo no encontrado; 500 persistencia/otro error. respondGroupError devuelve {"error":"mensaje"}; auth previo usa {"error":{"code":"...","message":"..."}}.

## Invocación (Front / Otros Servicios)

- `front/lib/core/services/social_service.dart:147`, `regenerateInvite` envía POST, valida la respuesta y retorna el token nuevo.
- `_InviteDialogState._regenerate` inicia la acción desde los controles de admin y reemplaza el token mostrado (`front/lib/features/groups/groups_screen.dart:812-836`).
- Sin invocación de producción desde otro microservicio.

## Flujo

1. `main()` monta `GroupHandler.RegisterRoutes` bajo JWT (`back/social/cmd/server/main.go:271-275`; `back/social/internal/handler/http/group_handler.go:23-38`).
2. `GroupHandler.RegenerateInvite` obtiene el actor del JWT y delega el ID de grupo (`back/social/internal/handler/http/group_handler.go:166-179`).
3. `GroupService.RegenerateInvite` valida el UUID y `GroupRepository.RegenerateInviteToken` comprueba el rol admin y guarda el nuevo token (`back/social/internal/service/group_service.go:334-346`; `back/social/internal/repository/group_repository.go:165-198`).
4. El handler devuelve `200` con `new_invite_token`; `_InviteDialogState._regenerate` actualiza el valor mostrado (`back/social/internal/handler/http/group_handler.go:172-179`; `front/lib/features/groups/groups_screen.dart:812-836`).

## Tests

- `back/social/internal/handler/http/group_handler_test.go:202`, `TestRegenerateInviteContract` verifica que no admin no puede, el token cambia y el viejo deja de admitir un ingreso.
- `back/social/internal/handler/http/groups_postgres_e2e_test.go:28` cubre rutas de grupo en PostgreSQL.
