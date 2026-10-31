# gRPC `auth.v1.IdentityService/GetUser`


## Req

- Servicio completo: `auth.v1.IdentityService/GetUser`, declarado en `back/proto/auth/v1/identity.proto:37-40`.
- `GetUserRequest.user_id`: UUID de usuario.
- Seguridad: metadata `x-internal-key`. Auth aplica logging y NewInternalAuth a todos los RPC unary: `back/auth/cmd/server/main.go:248-252` y `back/auth/internal/interceptor/auth.go:33`.
- Servidor escucha el puerto gRPC configurado; default local/Compose 9080.

## Res

- OK devuelve user_id, email y display_name opcional; solo datos públicos.
- `INVALID_ARGUMENT`: UUID inválido; `NOT_FOUND`: user_not_found; `INTERNAL`: error de repository.
- No devuelve hashes, tokens ni datos de autenticación.

## Invocación (Front / Otros Servicios)

- No se encontró invocación de producción actual. Existe wrapper Social `GRPCClient.GetUser` en `back/social/internal/identity/grpc_client.go:130` y tests del wrapper, pero no se encontró llamada productiva a ese método.
- La ruta HTTP `GET /internal/users/:id` sigue registrada en Auth y no se considera retirada por este RPC.

## Flujo

1. `main()` configura los interceptores gRPC y registra `IdentityService` (`back/auth/cmd/server/main.go:248-252`); `NewInternalAuth` valida `x-internal-key` (`back/auth/internal/interceptor/auth.go:33-47`).
2. `IdentityServer.GetUser` valida el UUID (`back/auth/internal/handler/grpc/identity_server.go:53-74`).
3. `UsersService.Lookup` normaliza y resuelve el ID (`back/auth/internal/service/users_service.go:30-53`); `UserRepository.GetPublicByIDs` lee los campos públicos (`back/auth/internal/repository/user_repository.go:188-219`).
4. `IdentityServer.GetUser` construye `GetUserResponse` (`back/auth/internal/handler/grpc/identity_server.go:53-74`).

## Tests

- `back/auth/internal/handler/grpc/identity_server_test.go:151`, `TestIdentityServer_GetUser`.
- `back/auth/internal/handler/grpc/identity_server_extra_test.go:30`, `error` del repository.
- `back/auth/internal/interceptor/interceptor_test.go:83-100` cubre clave correcta, ausente/mala y configuración fail-closed.
