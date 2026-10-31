# GET /auth/google-calendar/status

Verifica si el usuario tiene vinculada su cuenta de Google Calendar.

## 1. Req (Petición)
- **Método y Ruta:** `GET /auth/google-calendar/status` (Protegida con JWT; `back/auth/cmd/server/main.go:114`).
- **Headers:** `Authorization: Bearer <access_token>`.

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK`:** `{"connected": true, "provider": "google_calendar"}`.

## 3. Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/auth/google_calendar_service.dart:173` (`getCalendarStatus()`).
  - Pantalla: `front/lib/features/workspace/schedule_meeting_screen.dart:45`.

## 4. Flujo Técnico (Paso a Paso)
1. Consulta `identity.oauth_connections` buscando `provider = 'google_calendar'`.
2. Responde el estado booleano de conexión.
