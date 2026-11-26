# gRPC `auth.v1.IdentityService/GetOAuthToken`


## Req

- `auth.v1.IdentityService/GetOAuthToken`: `back/proto/auth/v1/identity.proto:51-56`.
- Request: user_id UUID y provider enum PROVIDER_GOOGLE_DRIVE o PROVIDER_GOOGLE_CALENDAR.
- Metadata `x-internal-key` obligatoria. El request no recibe un refresh token.
- Auth puede refrescar la credencial Google vencida usando su conexión OAuth guardada; nunca devuelve el refresh token Google.

## Res

- OK: access_token; expires_at y external_email opcionales.
- `INVALID_ARGUMENT` user_id/provider inválido; `NOT_FOUND` sin vínculo; `UNAUTHENTICATED` conexión OAuth marcada inválida/revocada; `UNAVAILABLE` Google caído; `INTERNAL` para otros errores.
- El metadato de vencimiento/correo se obtiene de la conexión guardada de forma best-effort; el access token validado puede devolverse sin esos metadatos.

## Invocación (Front / Otros Servicios)

- Notes: `back/notes/internal/drive/oauth_grpc.go:62`, `GRPCOAuthTokenProvider` solicita token Drive para leer/escribir contenido privado.
- Social Calendar real: `back/social/internal/service/calendar_grpc_gateway.go:45`, `GRPCalendarGateway`.GetToken obtiene el access token para crear/actualizar reuniones.
- No hay invocación Flutter directa; la app solicita las acciones a Notes/Social.

## Flujo

1. `main()` crea el servidor gRPC, instala logging y `NewInternalAuth`, y registra `IdentityService` (`back/auth/cmd/server/main.go:248-252`; `back/auth/internal/interceptor/auth.go:33-47`).
2. `IdentityServer.GetOAuthToken` valida `user_id` y provider (`back/auth/internal/handler/grpc/identity_server.go:118-151`).
3. Según el provider llama `DriveOAuthService.GetValidAccessToken` o `CalendarOAuthService.GetValidAccessToken`; ambos leen la conexión, renuevan si corresponde y persisten la rotación (`back/auth/internal/service/drive_oauth_service.go:391-450`; `back/auth/internal/service/calendar_oauth_service.go:225-281`). `OAuthRepository.GetByUserIDAndProvider`, `UpdateGoogleDriveAccessToken` y `UpdateGoogleCalendarAccessToken` acceden a esa persistencia (`back/auth/internal/repository/oauth_repository.go:33,130-176`).
4. `IdentityServer.GetOAuthToken` construye la respuesta protobuf con el token y metadatos de conexión (`back/auth/internal/handler/grpc/identity_server.go:118-151`). El interceptor creado por `NewInternalAuth` valida la clave antes del handler (`back/auth/internal/interceptor/auth.go:33-47`).

## Tests

- `back/auth/internal/handler/grpc/identity_server_test.go:227`, `TestIdentityServer_GetOAuthToken`.
- `back/auth/internal/handler/grpc/identity_server_pg_test.go:107`, `TestIdentityServerPG` verifica refresh y persistencia PostgreSQL con provider simulado.
- `back/notes/internal/drive/oauth_grpc_test.go` y `back/social/internal/service/calendar_grpc_gateway_test.go` prueban clientes con servidor simulado.
