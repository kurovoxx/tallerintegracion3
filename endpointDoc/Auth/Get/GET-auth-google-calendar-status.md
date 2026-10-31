# `GET /auth/google-calendar/status`


## Req

- `GET /auth/google-calendar/status`, JWT Bearer requerido: `back/auth/cmd/server/main.go:129`.
- Sin body, path ni query. El estado se consulta para el usuario autenticado.
- La ruta interna no contiene `/api-auth`; el gateway usa ese prefijo externo y lo reescribe.

## Res

- `200`: {"connected":false}, o {"connected":true,"external_email":"<cuenta Google>"} cuando el correo externo existe. `Cache-Control`: private, no-store.
- No expone access ni refresh tokens.
- 401 sin JWT válido; 500 si falla la consulta.
- El servicio consulta PostgreSQL, no comprueba el acceso remoto de Google en cada lectura.

## Invocación (Front / Otros Servicios)

- `front/lib/features/auth/google_calendar_service.dart:177`, `fetchStatus` consulta el estado y convierte JSON a CalendarStatus.
- `_ScheduleMeetingScreenState._loadCalendarStatus` consulta el estado al cargar; `_linkGoogleCalendar` lo vuelve a consultar tras vincular (`front/lib/features/workspace/schedule_meeting_screen.dart:176-200,204-248`).
- No se encontró consumidor directo desde otro microservicio para esta ruta HTTP. Social obtiene credenciales por `CalendarGRPCGateway.GetToken` (`back/social/internal/service/calendar_grpc_gateway.go:45-68`) y el RPC Identity `GetOAuthToken`, no por esta ruta.

## Flujo

1. `main()` registra `GET /auth/google-calendar/status` bajo autenticación (`back/auth/cmd/server/main.go:129`).
2. `CalendarHandler.Status` toma `user_id` del JWT y llama al servicio (`back/auth/internal/handler/http/calendar_handler.go:95-113`).
3. `CalendarOAuthService.GetCalendarConnectionStatus` consulta `OAuthRepository.GetByUserIDAndProvider` (`back/auth/internal/service/calendar_oauth_service.go:290-302`; `back/auth/internal/repository/oauth_repository.go:130-143`).
4. `CalendarHandler.Status` serializa `connected` y, si existe, `external_email` (`back/auth/internal/handler/http/calendar_handler.go:95-113`).

## Tests

- `back/auth/internal/handler/http/calendar_handler_test.go:144`, `TestCalendarHandler_Status_Desconectado_200`; :157, TestCalendarHandler_Status_Conectado_ConEmail_200; :175, falta de usuario.
- `back/auth/internal/service/calendar_oauth_service_test.go:116`, `status` desconectado; :124, status conectado con correo.
