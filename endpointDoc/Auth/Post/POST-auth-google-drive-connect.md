# `POST /auth/google-drive/connect`


## Req

- `POST /auth/google-drive/connect`, registrada con RequireAuth: `back/auth/cmd/server/main.go:125`. Requiere `Authorization`: Bearer <JWT Sigma>.
- JSON: oauth_code obligatorio; expected_email y redirect_uri opcionales. redirect_uri solo acepta los callbacks permitidos por el servicio; expected_email se compara con la cuenta Google devuelta.
- El código OAuth se intercambia por credenciales de Google; no enviar tokens Google directamente.
- Ruta interna `/auth/google-drive/connect.` El gateway web agrega `/api-auth` y lo retira al enrutar (`front/nginx.conf:23-27`).

## Res

- `200`: {"connected":true}. Auth guarda o reemplaza la conexión Drive del usuario identificado por JWT en `identity.oauth_connections`.
- `400`: body/código inválido, redirect no permitido o expected_email distinto.
- `401`: JWT inválido.
- `502`: Google no disponible.
- `500`: persistencia u otro error interno.
- No devuelve ni registra access/refresh tokens Google.

## Invocación (Front / Otros Servicios)

- `front/lib/features/academic/profile_screen.dart:279`, `_handleConnectDrive` inicia el flujo desde Perfil.
- `front/lib/features/auth/google_drive_service.dart:279`, `connectDrive` obtiene un authorization code mediante OAuth y envía POST; la respuesta actualiza el estado visible. El client_id/redirect_uri se obtienen del endpoint compartido `GET /auth/google-config` ([ficha existente](../Get/GET-auth-google-config.md)).
- No se encontró consumidor de producción desde otro microservicio para el endpoint HTTP; Notes obtiene el token de Drive mediante Identity gRPC.

## Flujo

1. `main()` registra la ruta protegida (`back/auth/cmd/server/main.go:125`).
2. `DriveHandler.Connect` obtiene el user ID del JWT, enlaza el JSON y mapea los errores (`back/auth/internal/handler/http/drive_handler.go:110-165`).
3. `DriveOAuthService.Connect` valida redirect/correo, intercambia el código y confirma la cuenta (`back/auth/internal/service/drive_oauth_service.go:336-388`).
4. `OAuthRepository.UpsertGoogleDriveConnection` persiste credenciales (`back/auth/internal/repository/oauth_repository.go:23,45-74`); el handler devuelve `connected: true` (`back/auth/internal/handler/http/drive_handler.go:110-165`). El intercambio de Google ocurre en `ConfigDriveOAuthProvider.exchangeWithRedirect` (`back/auth/internal/service/drive_oauth_service.go:121-212`).

## Tests

- `back/auth/internal/handler/http/drive_handler_test.go:100`, `TestDriveHandler_Connect_Valido_200`; :145, body inválido; :186, código inválido; :206, Google no disponible; :260, no expone tokens; :331, expected_email distinto.
- `back/auth/internal/service/drive_oauth_service_test.go:79`, `primera` conexión; :109, reconexión; :959-992, comparación opcional de correo.
- Las pruebas citadas usan providers simulados; no demuestran OAuth Google en producción.
