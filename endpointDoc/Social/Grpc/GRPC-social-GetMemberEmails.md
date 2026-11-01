# gRPC Social.GetMemberEmails

Devuelve los correos de todos los miembros de un grupo para dar permisos `restricted` en Google Drive. **Operación gRPC interna** (puerto `9083`). Fail-closed: nunca devuelve una lista parcial.

## Req
- RPC: `social.v1.SocialService/GetMemberEmails` (`back/proto/social/v1/social.proto`).
- Auth: metadata `x-internal-key`.
- Request: `GetMemberEmailsRequest{group_id:"<uuid>"}`.

## Res
- OK: `GetMemberEmailsResponse{emails:[...]}` (vacío si el grupo no tiene miembros resolubles).
- `Unavailable` `identity_unavailable`: Identity cayó o no hay directorio configurado. Notes no debe calcular permisos en ese caso.
- `InvalidArgument`, `NotFound`, `Unauthenticated`.

## Flujo
- Llamador: Notes `ListMemberEmails` (`back/notes/internal/service/social_grpc_adapter.go:111`) al compartir un apunte privado con el grupo.
- `SocialServer.GetMemberEmails` (`internal/handler/grpc/social_server.go:66`) → `GroupService.ListMemberEmailsInternal` (`internal/service/group_service.go:347`) → `users.LookupEmailsStrict` (Identity gRPC; falla si falta cualquier correo).
- Equivalente HTTP legado: `GET /internal/groups/:id/member-emails` (ver `GET-internal-groups-id-member-emails.md`).

## Tests
- `internal/handler/grpc/social_server_test.go`: `GetMemberEmails fail-closed`.
- `internal/service/internal_members_test.go`.
- `test/cross_service_e2e_test.go`: `TestCrossServiceMembershipLifecycle` (share restricted real entre procesos).
