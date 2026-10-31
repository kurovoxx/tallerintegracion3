# `POST /groups/:id/members/:user_id/kick`


## Req

- `POST /groups/:id/members/:user_id/kick` con JWT: `back/social/internal/handler/http/group_handler.go:25-36`.
- :id y :user_id son UUID. Solo admin. Body opcional {"cleanup_shared_notes":true|false}; ausente o no parseable equivale a false.
- El user_id objetivo es recurso, el actor admin se extrae del JWT.

## Res

- 204 sin body tras retirar membresía.
- 400 si el target es self o admin protegido; 401 sin sesión; 403 actor no admin/no miembro o usuario baneado; 404 grupo/objetivo ausente. Los errores de dominio de respondGroupError usan {"error":"mensaje"}; auth previo usa el envelope común.
- Stream se intenta retirar best-effort como operación existente. Si cleanup_shared_notes=true se inicia retirada de shares Notes best-effort/asíncrona; el 204 no confirma ese trabajo externo.

## Invocación (Front / Otros Servicios)

- `_GroupMembersTabState._confirmModeration` ofrece la opción `cleanup_shared_notes` y llama a `_perform` (`front/lib/features/groups/group_members_tab.dart:175-226,282-300`).
- `SocialService.kickMember` envía el boolean y acepta `204` (`front/lib/core/services/social_service.dart:210-235`).
- No hay caller backend directo; Notes no llama este endpoint.

## Flujo

1. `main()` monta `GroupHandler.RegisterRoutes` bajo JWT (`back/social/cmd/server/main.go:271-275`; `back/social/internal/handler/http/group_handler.go:23-38`).
2. `GroupHandler.KickMember` toma admin, objetivo y body opcional; extrae JWT para el fallback HTTP de Notes (`back/social/internal/handler/http/group_handler.go:197-215`).
3. `GroupService.KickMember` protege la operación con `withTargetAccount`, y `GroupRepository.KickMember` verifica permisos y retira la membresía (`back/social/internal/service/group_service.go:522-539`; `back/social/internal/repository/group_repository.go:293-316`).
4. `removeFromChat` solicita la retirada Stream best-effort; si se activó `cleanup_shared_notes`, `unshareNotes` llama Notes gRPC y conserva el fallback HTTP legacy (`back/social/internal/service/group_service.go:133-151,595-619`). El handler devuelve `204` sin esperar esa limpieza. El borrado de cuenta usa el RPC durable por separado (`back/social/internal/service/account_deletion.go:76-139`).

## Tests

- `back/social/internal/handler/http/group_handler_test.go:235`, `TestKickAndBanContract` recorre no admin, actor extraño, target y body.
- `back/social/internal/handler/http/groups_postgres_e2e_test.go:28` incluye kick en la suite E2E.
- `back/social/internal/service/group_isolation_test.go:48` cubre aislamiento de ban; no se encontró prueba HTTP que convierta el 204 en confirmación de Stream/Notes.
