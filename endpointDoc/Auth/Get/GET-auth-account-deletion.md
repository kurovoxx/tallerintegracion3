# GET /auth/account-deletion

Consulta el estado actual del proceso de borrado de cuenta para el usuario autenticado (incluso si la cuenta ya está bloqueada).

## 1. Req (Petición)
- **Método y Ruta:** `GET /auth/account-deletion` (Protegida con `RequireIdentity`; `back/auth/cmd/server/main.go:121`).
- **Headers:** `Authorization: Bearer <access_token>`.

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK`:** `{"status":"complete"}` o `{"status":"pending"}`.
- **`404 Not Found`:** `account_not_found` (no hay proceso de borrado ni cuenta asociada).

## 3. Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/core/services/account_deletion_service.dart:141` (`getDeletionStatus()`).
  - Flujo UI: Usado para resolver el estado final cuando el DELETE responde 202 o 503.

## 4. Flujo Técnico (Paso a Paso)
1. Lee `user_id` del token.
2. Consulta `identity.account_deletions`.
3. Retorna el estado actual del tombstone.

## 5. Dependencias y Datos Afectados
- **Tablas PostgreSQL:** `identity.account_deletions` (lectura).
