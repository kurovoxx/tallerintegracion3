# `POST /groups/:id/join`


## Req

- `POST /groups/:id/join`, JWT requerido; registro en `back/social/internal/handler/http/group_handler.go:25-36`.
- :id es UUID. JSON requerido: {"invite_token":"<uuid de invitación>"}.
- El user_id del nuevo miembro sale del JWT; sin query.

## Res

- `201` Created sin body al crear la membresía.
- 400 si falta invite_token/body; 401 sin JWT; 403 si el usuario está baneado; 404 si grupo/token no existe o es inválido; otros errores internos se traducen por respondGroupError como {"error":"mensaje"}. El middleware de auth usa {"error":{"code":"...","message":"..."}}.
- La membresía se inserta en Social; no se llama a Notes en esta operación.

## Invocación (Front / Otros Servicios)

- `_GroupsScreenState._openJoinDialog` valida el ID/código ingresado, llama `joinGroup` y recarga grupos tras éxito (`front/lib/features/groups/groups_screen.dart:179-251`).
- `SocialService.joinGroup` envía el request y notifica el cambio de grupos (`front/lib/core/services/social_service.dart:850-873`).
- No se encontró consumidor backend de producción.

## Flujo

1. `main()` monta `GroupHandler.RegisterRoutes` dentro del grupo protegido por JWT (`back/social/cmd/server/main.go:271-275`; `back/social/internal/handler/http/group_handler.go:23-38`).
2. `GroupHandler.Join` obtiene el user ID del JWT, valida `invite_token` y llama al servicio (`back/social/internal/handler/http/group_handler.go:147-164`).
3. `GroupService.JoinGroup` valida IDs; `GroupRepository.JoinWithInviteToken` comprueba token y ban, luego inserta la membresía dentro de una transacción (`back/social/internal/service/group_service.go:348-361`; `back/social/internal/repository/group_repository.go:111-163`).
4. El handler responde `201` sin cuerpo. `_openJoinDialog` espera el POST y vuelve a cargar el resumen (`back/social/internal/handler/http/group_handler.go:158-164`; `front/lib/features/groups/groups_screen.dart:179-251`).

## Tests

- `back/social/internal/handler/http/group_handler_test.go:184`, `TestJoinContract` cubre creación, token inválido/malformado, body ausente y ban.
- `back/social/internal/handler/http/groups_postgres_e2e_test.go:28`, `ciclo` de membresía PostgreSQL.
