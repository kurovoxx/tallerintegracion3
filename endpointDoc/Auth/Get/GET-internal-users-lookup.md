# `GET /internal/users/lookup`


## Req

- `GET /internal/users/lookup`?ids=<uuid>,<uuid>, registrado en el grupo RequireInternalKey: `back/auth/cmd/server/main.go:135-138`.
- Requiere header X-Internal-Key válido. No usa JWT de usuario. ids es query opcional separado por comas; vacío produce lista vacía. El servicio limita el lote a 200.
- No tiene body. No es ruta pública para Flutter; nginx bloquea `/api`-auth/internal/ en `front/nginx.conf:10-11`.

## Res

- `200`: array de objetos {user_id,email,display_name?}; siempre array, nunca null. IDs inexistentes se omiten.
- 500 ante error interno.
- No devuelve hashes ni tokens. El handler existente sigue registrado en esta versión.
- El contrato gRPC equivalente de lectura masiva es `auth.v1.IdentityService/LookupUsers`. Social utiliza gRPC en producción; no se encontró consumidor productivo del endpoint HTTP Lookup actualmente.

## Invocación (Front / Otros Servicios)

- No se encontró consumidor de producción de esta ruta HTTP en `back/social` ni en Flutter. Social usa el RPC `LookupUsers` mediante `GRPCUserDirectory.LookupUsers` (`back/social/internal/identity/grpc_directory.go:28-45`); la similitud de datos no convierte ese RPC en consumidor de este endpoint.
- Social resuelve datos de miembros mediante LookupUsers gRPC: `back/social/internal/identity/grpc_directory.go:28` y `main.go` configura el directorio en `back/social/cmd/server/main.go:104`.

## Flujo

1. `main()` registra `GET /internal/users/lookup` bajo `RequireInternalKey` (`back/auth/cmd/server/main.go:137`).
2. `InternalUsersHandler.Lookup` valida y parsea la lista de IDs (`back/auth/internal/handler/http/internal_users_handler.go:26-58`).
3. `UsersService.Lookup` limpia IDs, aplica el límite y delega en `UserRepository.GetPublicByIDs` (`back/auth/internal/service/users_service.go:30-53`; `back/auth/internal/repository/user_repository.go:188-219`).
4. `InternalUsersHandler.Lookup` serializa el array de usuarios públicos (`back/auth/internal/handler/http/internal_users_handler.go:26-58`).

## Tests

- `back/auth/internal/handler/http/internal_users_handler_test.go:24`, `TestInternalUsers_Lookup_200`; :46, sin IDs devuelve vacío.
- El consumo productivo migrado se prueba por separado en `back/auth/internal/handler/grpc/identity_server_test.go:172`, `TestIdentityServer_LookupUsers` y `back/social/internal/identity/grpc_directory_test.go:13`.
