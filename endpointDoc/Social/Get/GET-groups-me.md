# `GET /groups/me`


## Req

- `GET /groups/me` bajo JWT: `back/social/internal/handler/http/group_handler.go:25-36`; router protegido en `back/social/cmd/server/main.go:272-275`.
- Sin body, query ni path params. El usuario se obtiene del JWT.
- La ruta es del servicio Social; el base Flutter habitual termina en `/api-social`, que el gateway reescribe.

## Res

- `200`: array de objetos con group_id, name y role; si no hay grupos responde [].
- 401 por sesión inválida; errores de almacenamiento se mapean como error interno. Los errores de negocio de respondGroupError usan {"error":"mensaje"}; el middleware de auth usa el envelope común {"error":{"code":"...","message":"..."}}.
- Es una lista mínima de membresías, no el payload enriquecido de la pantalla principal.

## Invocación (Front / Otros Servicios)

- `FollowedScreen._inviteToGroup` usa `SocialService.listMyGroups` para listar los grupos del usuario y ofrecer solo los que administra (`front/lib/features/academic/followed_screen.dart`).
- No se encontró consumidor de producción desde otro microservicio.

## Flujo

1. `GroupHandler.RegisterRoutes` declara `GET /groups/me`; `main()` monta esas rutas bajo el middleware JWT (`back/social/internal/handler/http/group_handler.go:23-38`; `back/social/cmd/server/main.go:271-275`).
2. `GroupHandler.ListMy` obtiene el user ID del JWT y llama al servicio (`back/social/internal/handler/http/group_handler.go:129-140`).
3. `GroupService.ListMyGroups` consulta al repositorio (`back/social/internal/service/group_service.go:276-285`; `back/social/internal/repository/group_repository.go:80-108`).
4. El handler responde el arreglo JSON, incluso vacío (`back/social/internal/handler/http/group_handler.go:135-140`). El consumidor Flutter documentado arriba usa `/me/overview`, no esta ruta.

## Front
- Para tarjetas y sidebar, el front usa `GET /me/overview` (`SocialService.getOverview`), que devuelve además `sidebar.groups`, `stats` y `member_count` por grupo.
- Para elegir un grupo al invitar a una persona seguida, `SocialService.listMyGroups` consume este endpoint y `FollowedScreen` filtra los grupos con rol `admin`.
- Consumidores del overview: `_GroupsScreenState._load` (`front/lib/features/groups/groups_screen.dart:81`, tarjetas de grupo) y el sidebar de `MainShell` (`front/lib/core/widgets/main_shell.dart:111`, primeros 5 grupos y contador).

## Tests

- `back/social/internal/handler/http/group_handler_test.go:144`, `TestListMyGroupsContract` comprueba array vacío y forma de cada elemento.
- `back/social/internal/handler/http/groups_postgres_e2e_test.go:28` contiene el ciclo de membresía contra PostgreSQL.
