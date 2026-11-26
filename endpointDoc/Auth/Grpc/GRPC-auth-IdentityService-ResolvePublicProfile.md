# gRPC `identity.v1.IdentityService/ResolvePublicProfile`

RPC interno (servicio a servicio) que traduce el `public_id` de un perfil al
UUID interno del usuario. Lo consume Notes para
`GET /public-users/:public_id/notes`. El UUID nunca se devuelve a clientes
HTTP.

No informa la visibilidad del perfil: por decisión del equipo, la visibilidad
del perfil no filtra los apuntes públicos (RN-PERFIL-06), así que Notes no la
necesita.

## 1. Req (Petición)
- **Contrato:** `back/proto/auth/v1/identity.proto`.
  ```proto
  message ResolvePublicProfileRequest { string public_id = 1; }
  message ResolvePublicProfileResponse { string user_id = 1; }
  ```
- **Autenticación:** metadata `x-internal-key` igual a `INTERNAL_API_KEY`
  (interceptor `NewInternalAuth`). Sin clave válida: `Unauthenticated`.
- **Servidor:** `IdentityServer.ResolvePublicProfile`
  (`back/auth/internal/handler/grpc/identity_server.go`), registrado con
  `NewIdentityServerWithPublicProfiles(..., userRepo)` en
  `back/auth/cmd/server/main.go`.

## 2. Res (Respuestas y Códigos gRPC)
- **OK:** `{user_id}`.
- **`InvalidArgument`:** `public_id` con formato inválido (tras normalizar a
  minúsculas).
- **`NotFound`:** `public_id` inexistente.
- **`Internal`:** fallo de base de datos o resolver no configurado.

## 3. Invocación (Otros Servicios)
- **Notes:** `identity.PublicProfileClient.ResolvePublicProfile`
  (`back/notes/internal/identity/client.go`), creado en `main.go`
  (`newPublicProfileResolver`) con `IDENTITY_GRPC_ADDR`, `INTERNAL_API_KEY` y
  `SOCIAL_TIMEOUT` como timeout. Es independiente de `STORAGE_MODE`. Mapea
  `NotFound`/`InvalidArgument` → 404 y cualquier otro error → 503.

## 4. Flujo Técnico
1. Normaliza y valida `public_id` (`utils.NormalizePublicID`,
   `utils.ValidatePublicID`).
2. `UserRepository.GetIDByPublicID`
   (`back/auth/internal/repository/user_repository.go`):
   `SELECT id FROM identity.users WHERE public_id = $1`.
3. Responde `user_id`.

## 5. Tests Asociados
- `back/auth/internal/handler/grpc/identity_server_test.go`:
  `TestIdentityServer_ResolvePublicProfile` (resolución, normalización,
  InvalidArgument, NotFound, Unauthenticated) y
  `TestIdentityServer_ResolvePublicProfile_Internal`.
- `back/notes/internal/identity/client_test.go`.
