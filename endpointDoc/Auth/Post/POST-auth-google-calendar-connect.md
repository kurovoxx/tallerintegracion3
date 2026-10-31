# `POST /auth/google-calendar/connect`


## Req

- `POST /auth/google-calendar/connect`; `back/auth/cmd/server/main.go:128`. Requiere JWT Bearer mediante RequireAuth.
- JSON: oauth_code requerido y redirect_uri opcional. La URI se valida contra allowlist; si no viene se usa la del servidor.
- Sin query ni path params. El navegador OAuth obtiene el código; el código no equivale al JWT de Sigma.
- El prefijo externo del gateway (`/api-auth`) no es parte de la ruta Gin.

## Res

- `200`: {"connected":true}; la conexión de Google Calendar queda guardada para el usuario autenticado.
- `400`: JSON/código/redirect inválido.
- `401`: sesión inválida.
- `502`: Google no disponible al canjear el código.
- `500`: otro error de servicio/repositorio.
- No devuelve tokens Google ni registra el código OAuth.

## Invocación (Front / Otros Servicios)

- `front/lib/features/workspace/schedule_meeting_screen.dart:204-229`, `_linkGoogleCalendar` obtiene código/redirect y llama al servicio.
- `front/lib/features/auth/google_calendar_service.dart:84`, `connectCalendar`, envía POST a Auth; la pantalla consulta después el estado.
- No se encontró consumidor de producción de este endpoint HTTP desde Social. Social pide el token actual mediante Identity gRPC.

## Flujo

1. `main()` registra la ruta protegida (`back/auth/cmd/server/main.go:128`).
2. `CalendarHandler.Connect` obtiene el usuario del JWT y valida JSON/redirect (`back/auth/internal/handler/http/calendar_handler.go:36-83`).
3. `CalendarOAuthService.ConnectWithRedirect` intercambia el código y verifica la cuenta (`back/auth/internal/service/calendar_oauth_service.go:177-220`).
4. `OAuthRepository.UpsertGoogleCalendarConnection` guarda credenciales y correo externo (`back/auth/internal/repository/oauth_repository.go:28,45-74`); el handler responde `200` (`back/auth/internal/handler/http/calendar_handler.go:36-83`). El intercambio con Google ocurre sincrónicamente en `ConfigCalendarOAuthProvider.exchangeWithRedirect` (`back/auth/internal/service/calendar_oauth_service.go:73-117`).

## Tests

- `back/auth/internal/handler/http/calendar_handler_test.go:68`, `TestCalendarHandler_Connect_Valido_200`; :94, sin usuario; :103, código vacío; :114, código inválido; :124, Google no disponible.
- `back/auth/internal/service/calendar_oauth_service_test.go:54`, `conexi`ón válida; :66, código vacío; :77, inválido.
- Las pruebas HTTP usan dependencias simuladas; no son validación contra Google real.
