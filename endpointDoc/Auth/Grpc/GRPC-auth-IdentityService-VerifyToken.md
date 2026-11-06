# gRPC `auth.v1.IdentityService/VerifyToken`


## Req

- `auth.v1.IdentityService/VerifyToken`: `back/proto/auth/v1/identity.proto:47-49`.
- Request: `VerifyTokenRequest.token` contiene el JWT de acceso de Auth.
- Metadata `x-internal-key` obligatoria incluso para VerifyToken; el JWT del request se valida como dato, no sustituye la clave de servicio.
- El token no se escribe en logs.

## Res

- OK: {"valid":true,"user_id":"<uuid>"}; false no se devuelve en OK.
- `INVALID_ARGUMENT` si token vacío; `UNAUTHENTICATED` si inválido o expirado; `INTERNAL` si falta/configuración de clave por interceptor.
- Respuesta no contiene claims adicionales.

## Invocación (Front / Otros Servicios)

- No se encontró una invocación de producción del RPC. Hay cliente Social de compatibilidad en `back/social/internal/identity/grpc_client.go:110`, `pero` no se encontró uso en main.
- HTTP `POST /internal/auth/verify` permanece registrado en Auth; no fue retirado por este RPC.

## Flujo

1. `main()` configura interceptores y registra `IdentityService` (`back/auth/cmd/server/main.go:248-252`); `NewInternalAuth` valida `x-internal-key` antes del método (`back/auth/internal/interceptor/auth.go:33-47`).
2. `IdentityServer.VerifyToken` recorta y valida la entrada (`back/auth/internal/handler/grpc/identity_server.go:102-108`).
3. `JWTService.ValidarAccessToken` verifica firma HS256, issuer, audience, expiración y user ID (`back/auth/internal/service/jwt_service.go:105-141`).
4. `IdentityServer.VerifyToken` traduce token vencido/inválido a `UNAUTHENTICATED`; solo devuelve `valid: true` y `user_id` si la validación pasó (`back/auth/internal/handler/grpc/identity_server.go:108-115`).

## Tests

- `back/auth/internal/handler/grpc/identity_server_test.go:192`, `TestIdentityServer_VerifyToken`.
- `back/auth/internal/handler/grpc/identity_server_extra_test.go:119`, `TestIdentityServer_VerifyTokenVariantes`; :139 comprueba que todos los métodos exigen auth interna.
