# GET /internal/oauth/calendar-token

Entrega tokens OAuth descifrados de Google Calendar para que el microservicio Social pueda interactuar con la Google Calendar API.

## 1. Req (Petición)
- **Método y Ruta:** `GET /internal/oauth/calendar-token` (Servicio-a-servicio; `back/auth/cmd/server/main.go:116`).
- **Headers Requeridos:** `X-Internal-Key: <INTERNAL_API_KEY>`.
- **Query Params:** `user_id=<UUID>`.

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK`:** `{"access_token":"ya29...","expires_at":"2026-10-31T..."}`.
- **`401 Unauthorized`:** `X-Internal-Key` inválida o ausente.
- **`404 Not Found`:** El usuario no tiene vinculado Google Calendar.

## 3. Invocación (Front / Otros Servicios)
- **Microservicio Social:**
  - Cliente: `back/social/internal/service/calendar_gateway.go:55` (`GetCalendarToken`).
  - Uso: Crear eventos en el Google Calendar del usuario al agendar una reunión en un grupo.

## 4. Flujo Técnico (Paso a Paso)
1. `RequireInternalKey` valida la clave compartida.
2. Lee `user_id` del query param.
3. Consulta `identity.oauth_connections` para `provider = 'google_calendar'`.
4. Descifra el token de acceso con AES-256-GCM.
5. Si el token está expirado, utiliza el refresh token para refrescarlo en Google antes de devolverlo.

## 5. Dependencias y Datos Afectados
- **Tablas PostgreSQL:** `identity.oauth_connections`.
- **Seguridad:** Bloqueado en Nginx Gateway (`location /api-auth/internal/ { return 404; }`).
