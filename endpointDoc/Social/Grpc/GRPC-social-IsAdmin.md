# gRPC Social.IsAdmin

Indica si un usuario es admin de un grupo. **Operación gRPC interna** (puerto `9083`).

## Req
- RPC: `social.v1.SocialService/IsAdmin` (`back/proto/social/v1/social.proto`).
- Auth: metadata `x-internal-key`.
- Request: `IsAdminRequest{group_id:"<uuid>", user_id:"<uuid>"}`.

## Res
- OK: `IsAdminResponse{is_admin}`; `false` también para no-miembros.
- `InvalidArgument`, `NotFound` (`group_not_found`), `Unauthenticated`, `Internal`: igual que `IsMember`.

## Flujo
- Llamador: Notes (`back/notes/internal/service/social_grpc_adapter.go:70`) para permitir que un admin retire apuntes compartidos por otros.
- `SocialServer.IsAdmin` (`internal/handler/grpc/social_server.go:37`) → `role()` → `GroupService.MembershipRole` y compara con `admin`.

## Tests
- `internal/handler/grpc/social_server_test.go`: `IsMember/IsAdmin`, `todos los RPC requieren clave interna`.
- `test/cross_service_e2e_test.go`: `TestCrossServiceMembershipLifecycle`.
