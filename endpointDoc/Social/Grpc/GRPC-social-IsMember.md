# gRPC Social.IsMember

Indica si un usuario pertenece a un grupo y con qué rol. **Operación gRPC interna** (puerto `9083`), no endpoint HTTP.

## Req
- RPC: `social.v1.SocialService/IsMember` (`back/proto/social/v1/social.proto`; servidor en `back/social/cmd/server/main.go:131`, `startGRPC`).
- Auth: metadata `x-internal-key` = `INTERNAL_API_KEY` (interceptor `internal/interceptor`).
- Request: `IsMemberRequest{group_id:"<uuid>", user_id:"<uuid>"}`.

## Res
- OK: `IsMemberResponse{is_member, role}`; `role` = `ROLE_ADMIN`, `ROLE_MEMBER` o `ROLE_UNSPECIFIED` si no es miembro.
- `InvalidArgument`: uuid inválido. `NotFound` `group_not_found`: el grupo no existe. `Unauthenticated`: sin clave. `Internal`: error de BD (sin detalles).

## Flujo
- Llamador: Notes (`back/notes/internal/service/social_grpc_adapter.go:57`) antes de compartir apuntes y al leer apuntes compartidos.
- `SocialServer.IsMember` (`internal/handler/grpc/social_server.go:29`) → `role()` (`:105`, valida uuids) → `GroupService.MembershipRole` (`internal/service/group_service.go:388`: grupo existe, rol o `""`).

## Tests
- `internal/handler/grpc/social_server_test.go`: `IsMember/IsAdmin`, `errores de entrada y grupo inexistente`, `todos los RPC requieren clave interna` (bufconn).
- `internal/handler/grpc/social_errors_test.go`: los errores de BD no se filtran.
- `test/cross_service_e2e_test.go`: `TestCrossServiceMembershipLifecycle` (Notes → Social sobre TCP real).
