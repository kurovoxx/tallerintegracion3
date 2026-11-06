# `POST /groups`


## Req

- `POST /groups`, dentro del grupo HTTP protegido con JWT; registro: `back/social/internal/handler/http/group_handler.go:25-36` y middleware en `back/social/cmd/server/main.go:272-275`.
- `Authorization`: Bearer <JWT>; JSON: name requerido y description opcional. name se normaliza y no puede quedar vacío; máximo 200 bytes. No se acepta owner_user_id del cliente.
- Ejemplo: {"name":"Grupo demo","description":"Estudio"}.

## Res

- `201`: {"group_id":"<uuid>"}.
- 400 si el JSON/nombre es inválido; 401 sin JWT; 500 ante error interno. Los errores de Create usan el envelope común {"error":{"code":"...","message":"..."}}.
- PostgreSQL inserta grupo y membresía inicial admin en una transacción. El alta de canal Stream es best-effort y no revierte el grupo.

## Invocación (Front / Otros Servicios)

- `_GroupsScreenState._openCreateSheet` confirma el formulario, llama a `createGroup` y vuelve a cargar la lista (`front/lib/features/groups/groups_screen.dart:123-159`).
- `SocialService.createGroup` envía POST con JWT, valida `201`, extrae `group_id` y notifica el cambio (`front/lib/core/services/social_service.dart:95-125`).
- No se encontró invocación de producción desde otro microservicio.

## Flujo

1. `main()` instala `GroupHandler.RegisterRoutes` bajo JWT (`back/social/cmd/server/main.go:271-275`; `back/social/internal/handler/http/group_handler.go:23-38`).
2. `GroupHandler.Create` obtiene el user ID del JWT, valida el body y llama al servicio (`back/social/internal/handler/http/group_handler.go:85-103`).
3. `GroupService.Create` normaliza y valida los datos; `GroupRepository.CreateWithOwner` guarda el grupo y su membresía de admin en una transacción (`back/social/internal/service/group_service.go:212-238`; `back/social/internal/repository/group_repository.go:23-58`).
4. El handler responde `201` con `group_id`; la notificación de canal Stream se ejecuta best-effort (`back/social/internal/handler/http/group_handler.go:96-103`; `back/social/internal/service/group_service.go:233-235`). `_openCreateSheet` recarga la lista tras éxito (`front/lib/features/groups/groups_screen.dart:123-159`).

## Front
- Cliente: `SocialService.createGroup` (`front/lib/core/services/social_service.dart:96`); envía `name` y `description` (solo si no está vacía), ambos con `trim()`.
- Pantalla: `_GroupsScreenState._openCreateSheet` (`front/lib/features/groups/groups_screen.dart:136`), botón de nuevo grupo → hoja `_NewGroupSheet`.
- Respuesta: exige `201` y lee solo `group_id` (si falta lanza `missing_group_id`). El id no se muestra: se llama `SocialService.notifyGroupsChanged()` y la pantalla recarga con `getOverview` (lista + sidebar).
- UI: snackbar `Grupo creado.` o `No se pudo crear el grupo.`.

## Tests

- `back/social/internal/handler/http/group_handler_test.go:131`, `TestCreateGroupContract`.
- `back/social/internal/service/group_service_test.go` incluye validaciones de Create.
- `back/social/internal/handler/http/groups_postgres_e2e_test.go:28`, `TestGroupsPostgresE2E_MembershipLifecycle` verifica ruta y persistencia. Suite con base PostgreSQL configurada.
