# `POST /groups/:id/leave`


## Req

- `POST /groups/:id/leave` con JWT: `back/social/internal/handler/http/group_handler.go:25-36`.
- :id UUID. Body opcional {"cleanup_shared_notes":true|false}; vacío o inválido deja false. Sin query.
- Identidad solo del JWT.

## Res

- 204 sin body después de quitar la membresía.
- Si el usuario era el único admin, el siguiente miembro más antiguo puede ser promovido; si el grupo queda vacío, se elimina. La actualización de owner_user_id acompaña la salida según la regla del repositorio.
- 401 sesión inválida; 404 no miembro/grupo; 500 interno. respondGroupError usa {"error":"mensaje"}; auth previo usa {"error":{"code":"...","message":"..."}}.
- Stream se retira best-effort y cleanup_shared_notes=true pide a Notes retirar shares en segundo plano; 204 no confirma esos proveedores.

## Invocación (Front / Otros Servicios)

- `_GroupInfoDialogState._leave` solicita confirmación, llama `leaveGroup` y cierra el diálogo tras éxito (`front/lib/features/groups/groups_screen.dart:567-624`).
- `front/lib/core/services/social_service.dart:824` envía POST con el boolean de cleanup y refresca la vista.
- `GroupService.unshareNotes` intenta Notes gRPC y, si falla, conserva el fallback HTTP legacy (`back/social/internal/service/group_service.go:595-619`). Stream se solicita mediante `removeFromChat` (`back/social/internal/service/group_service.go:133-151`).

## Flujo

1. `main()` monta `GroupHandler.RegisterRoutes` dentro del grupo JWT (`back/social/cmd/server/main.go:271-275`; `back/social/internal/handler/http/group_handler.go:23-38`).
2. `GroupHandler.LeaveGroup` obtiene el usuario del JWT, acepta el body opcional y llama al servicio (`back/social/internal/handler/http/group_handler.go:284-304`).
3. `GroupService.LeaveGroup` ejecuta la salida; `GroupRepository.LeaveGroup` delega en `removeUserFromGroup`, que en transacción aplica sucesión o elimina el grupo si era el último miembro (`back/social/internal/service/group_service.go:577-593`; `back/social/internal/repository/group_repository.go:533-618`).
4. Después de confirmar la membresía, `removeFromChat` y `unshareNotes` se ejecutan best-effort/asíncronos; el fallback HTTP de Notes solo ocurre si falla el RPC (`back/social/internal/service/group_service.go:133-151,577-619`). El handler devuelve `204`; `_leave` cierra el detalle tras éxito (`back/social/internal/handler/http/group_handler.go:299-304`; `front/lib/features/groups/groups_screen.dart:567-624`).

## Tests

- `back/social/internal/handler/http/group_handler_test.go:297`, `TestLeaveContract`.
- `back/social/internal/service/group_service_test.go:872`, `TestLeaveSuccessionOrderAndOwner`.
- `back/social/internal/repository/group_repository_integration_test.go` prueba sucesión/concurrencia con PostgreSQL; TestGroupsPostgresE2E_MembershipLifecycle cubre HTTP.
