# gRPC Social.GetMembers

Lista los miembros de un grupo con rol, fecha de ingreso y, si Identity responde, nombre y correo. **Operación gRPC interna** (puerto `9083`). No exige que quien llama sea miembro: la confianza viene de la clave interna.

## Req
- RPC: `social.v1.SocialService/GetMembers` (`back/proto/social/v1/social.proto`).
- Auth: metadata `x-internal-key`.
- Request: `GetMembersRequest{group_id:"<uuid>"}`.

## Res
- OK: `GetMembersResponse{members:[Member{user_id, role, joined_at, email, display_name}]}`. `email`/`display_name` vacíos si Identity falla (best-effort).
- `InvalidArgument`: uuid inválido. `NotFound`: grupo inexistente. `Unauthenticated`. `Internal`.

## Flujo
- Llamador: Notes `ListMemberUserIDs` (`back/notes/internal/service/social_grpc_adapter.go:92`) para resolver las cuentas Google Drive de los miembros.
- `SocialServer.GetMembers` (`internal/handler/grpc/social_server.go:45`) → `GroupService.GroupMembers` (`internal/service/group_service.go:378`: `ListMembersInternal` + `enrichMembers` vía Identity gRPC).

## Tests
- `internal/handler/grpc/social_server_test.go`: `GetMembers`.
- `internal/service/group_members_enriched_test.go` (enriquecimiento y fallo de Identity).
