# POST /auth/google-calendar/connect

Conecta la cuenta de Google Calendar del usuario mediante OAuth para permitir la sincronización automática de reuniones de grupo.

## 1. Req (Petición)
- **Método y Ruta:** `POST /auth/google-calendar/connect` (Protegida con JWT; `back/auth/cmd/server/main.go:113`).
- **Headers:** `Authorization: Bearer <access_token>`, `Content-Type: application/json`.
- **Body:** `{"code": "4/0Aean...", "redirect_uri": "..."}`.

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK`:** `{"status":"connected","provider":"google_calendar"}`.
- **`400 Bad Request`:** `invalid_code`.

## 3. Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/auth/google_calendar_service.dart:110` (`connectCalendar()`).
  - Pantalla: `front/lib/features/workspace/schedule_meeting_screen.dart`.

## 4. Flujo Técnico (Paso a Paso)
1. Canjea el código en Google OAuth API.
2. Cifra los tokens recibidos con AES-256.
3. Guarda o actualiza en `identity.oauth_connections` con `provider = 'google_calendar'`.

## 5. Dependencias y Datos Afectados
- **Tablas PostgreSQL:** `identity.oauth_connections`.
