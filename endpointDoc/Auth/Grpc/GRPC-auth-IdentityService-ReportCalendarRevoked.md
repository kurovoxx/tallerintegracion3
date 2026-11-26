# gRPC `auth.v1.IdentityService/ReportCalendarRevoked`


## Req

- `auth.v1.IdentityService/ReportCalendarRevoked`: `back/proto/auth/v1/identity.proto:58-60`.
- Request: user_id UUID del propietario de la conexión Calendar que Google rechazó.
- Metadata `x-internal-key` obligatoria. Sin cuerpo adicional ni JWT de usuario.

## Res

- OK con respuesta vacía para user_id válido. Es idempotente y evita filtrar si existe una conexión.
- `INVALID_ARGUMENT` si UUID inválido; error de autenticación interna si falta/mala clave.
- El handler ignora intencionalmente el resultado de marcar revoked_at; por ello OK significa que se procesó el aviso, no una confirmación de escritura persistida.

## Invocación (Front / Otros Servicios)

- Social, en Calendar real: `back/social/internal/service/calendar_grpc_gateway.go:77`, `ReportRevoked` se llama cuando Google devuelve rechazo de permisos.
- La pantalla Flutter no llama al RPC: usa Social para reuniones.
- Auth conserva también `POST /internal/oauth/calendar-revoked`, ruta HTTP histórica aún registrada.

## Flujo

1. Social llama `GRPCCalendarGateway.ReportRevoked`, que agrega `x-internal-key` y envía el RPC (`back/social/internal/service/calendar_grpc_gateway.go:77-89`).
2. Auth crea el servidor con `NewInternalAuth` y registra `IdentityService` (`back/auth/cmd/server/main.go:248-252`); el interceptor comprueba la clave (`back/auth/internal/interceptor/auth.go:33-47`).
3. `IdentityServer.ReportCalendarRevoked` valida UUID y llama al servicio (`back/auth/internal/handler/grpc/identity_server.go:153-162`).
4. `CalendarOAuthService.ReportCalendarPermissionDenied` marca la conexión como revocada (`back/auth/internal/service/calendar_oauth_service.go:307-316`); `OAuthRepository.MarkGoogleCalendarConnectionRevoked` persiste `revoked_at` (`back/auth/internal/repository/oauth_repository.go:38,109-127`).
5. `IdentityServer.ReportCalendarRevoked` devuelve una respuesta protobuf vacía (`back/auth/internal/handler/grpc/identity_server.go:153-162`).

## Tests

- `back/auth/internal/handler/grpc/identity_server_test.go:252`, `TestIdentityServer_ReportCalendarRevoked`.
- `back/social/internal/service/calendar_grpc_gateway_test.go` comprueba la notificación de rechazo con servidor gRPC simulado.
