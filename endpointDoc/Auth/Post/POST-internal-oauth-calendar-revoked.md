# POST /internal/oauth/calendar-revoked

Permite a otros microservicios (como Social) notificar que un token de Calendar fue revocado por el usuario en Google para invalidar la conexión localmente.

## 1. Req (Petición)
- **Método y Ruta:** `POST /internal/oauth/calendar-revoked` (Servicio-a-servicio; `back/auth/cmd/server/main.go:117`).
- **Headers:** `X-Internal-Key: <INTERNAL_API_KEY>`.
- **Body:** `{"user_id": "<UUID>"}`.

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK`:** `{"status":"marked_revoked"}`.
- **`401 Unauthorized`:** `X-Internal-Key` incorrecta.

## 3. Invocación (Front / Otros Servicios)
- **Microservicio Social:**
  - Cliente: `back/social/internal/service/calendar_gateway.go:100` (`ReportCalendarRevoked`).
  - Se activa cuando la Google Calendar API responde 401/invalid_grant.

## 4. Flujo Técnico (Paso a Paso)
1. Valida `X-Internal-Key`.
2. Actualiza `identity.oauth_connections` eliminando o marcando como revocada la conexión de Calendar del usuario.

## 5. Dependencias y Datos Afectados
- **Tablas PostgreSQL:** `identity.oauth_connections`.
