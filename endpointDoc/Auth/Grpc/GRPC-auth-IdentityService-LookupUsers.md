# gRPC `auth.v1.IdentityService/LookupUsers`


## Req

- `auth.v1.IdentityService/LookupUsers`: `back/proto/auth/v1/identity.proto:42-45`.
- Request: repeated user_ids; límite 200. Sin IDs o IDs vacíos da lista vacía; un ID no vacío malformado da `INVALID_ARGUMENT`.
- Metadata `x-internal-key` obligatoria, ver `back/auth/cmd/server/main.go:248-252`. No es una llamada Flutter directa.

## Res

- OK: users repetidos, vacíos cuando corresponde; cada User incluye user_id, email y display_name opcional. Usuarios inexistentes se omiten; no hay orden garantizado.
- `INVALID_ARGUMENT` por UUID inválido; `INTERNAL` por error de lectura.
- No devuelve secretos ni hashes.

## Invocación (Front / Otros Servicios)

- Producción: Social usa Identity gRPC para enriquecer `GET /groups/:id/members.` Configura GRPCUserDirectory en `back/social/cmd/server/main.go:104`; consulta en `back/social/internal/identity/grpc_directory.go:28`.
- El servicio Social entrega esos campos opcionales a sus pantallas de grupos; no hay cliente Flutter directo al RPC.
- La ruta HTTP `GET /internal/users/lookup` sigue registrada, pero el flujo productivo citado usa gRPC.

## Flujo

1. `main()` instala los interceptores y registra `IdentityService` (`back/auth/cmd/server/main.go:248-252`); `NewInternalAuth` comprueba `x-internal-key` (`back/auth/internal/interceptor/auth.go:33-47`).
2. `IdentityServer.LookupUsers` elimina IDs vacíos, valida UUIDs y aplica el límite configurado (`back/auth/internal/handler/grpc/identity_server.go:76-99`).
3. `UsersService.Lookup` normaliza, limita y consulta los datos públicos (`back/auth/internal/service/users_service.go:30-53`); `UserRepository.GetPublicByIDs` realiza la consulta conjunta de usuarios y perfiles (`back/auth/internal/repository/user_repository.go:188-219`).
4. `IdentityServer.LookupUsers` arma `LookupUsersResponse` (`back/auth/internal/handler/grpc/identity_server.go:76-99`). `GRPCUserDirectory.LookupUsers` en Social asocia por ID y, si Identity falla, `GroupService.enrichMembers` conserva la lista base (`back/social/internal/identity/grpc_directory.go:28-45`; `back/social/internal/service/group_service.go:468-484`).

## Tests

- `back/auth/internal/handler/grpc/identity_server_test.go:172`, `TestIdentityServer_LookupUsers`.
- `back/auth/internal/handler/grpc/identity_server_extra_test.go:50`, `TestIdentityServer_LookupLimit`.
- `back/social/internal/identity/grpc_directory_test.go:13`, `TestGRPCDirectory_LookupUsers`; :34 prueba la falta de cliente.
