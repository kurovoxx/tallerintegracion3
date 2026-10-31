# `GET /auth/google-drive/status`


## Req

- `GET /auth/google-drive/status`; ruta protegida con JWT: `back/auth/cmd/server/main.go:125-126`.
- Sin body, path ni query; `Authorization`: Bearer <access JWT>.
- Ruta Auth sin prefijo. En la web el base habitual `/api-auth` se reescribe a / antes de llegar al servicio: `front/nginx.conf:23-27`.

## Res

- `200`: {"connected":true|false,"reconnect_required":true|false}; `Cache-Control`: private, no-store. No incluye secretos/tokens.
- connected indica si hay conexión utilizable; reconnect_required indica si el vínculo fue marcado inválido/revocado.
- 401 por sesión no válida; 500 si no se puede consultar almacenamiento.
- La consulta está limitada al user_id del JWT.

## Invocación (Front / Otros Servicios)

- `front/lib/core/services/profile_service.dart:39`, `getDriveStatus` envía la consulta y devuelve connected. `front/lib/features/academic/profile_screen.dart:131`, `_restoreDriveStatus`, refleja el resultado en Perfil.
- No se encontró invocación HTTP de producción desde otros servicios. Notes solicita credenciales mediante `GRPCOAuthTokenProvider.GetValidAccessToken` (`back/notes/internal/drive/oauth_grpc.go:62-89`), que llama al RPC `GetOAuthToken`.

## Flujo

1. `main()` registra `GET /auth/google-drive/status` bajo autenticación (`back/auth/cmd/server/main.go:126`).
2. `DriveHandler.Status` toma `user_id` del JWT y llama al servicio (`back/auth/internal/handler/http/drive_handler.go:88-107`).
3. `DriveOAuthService.GetGoogleDriveConnectionStatus` consulta `OAuthRepository.GetByUserIDAndProvider` (`back/auth/internal/service/drive_oauth_service.go:462-476`; `back/auth/internal/repository/oauth_repository.go:130-143`) sin llamar a Google.
4. `DriveHandler.Status` devuelve el estado sanitizado y no almacenable en caché (`back/auth/internal/handler/http/drive_handler.go:88-107`).

## Tests

- `back/auth/internal/handler/http/drive_handler_test.go:350`, `TestDriveStatusIsAuthenticatedAndScoped`.
- `back/auth/internal/service/drive_oauth_service_test.go:867`, `sin` conexión; :879, conectada; :889, revocada; :899, no hace HTTP.
