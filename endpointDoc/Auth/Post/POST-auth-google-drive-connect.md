# POST /auth/google-drive/connect

Intercambia el código de autorización OAuth devuelto por Google por tokens de acceso/refresco y los almacena cifrados para el usuario.

## 1. Req (Petición)
- **Método y Ruta:** `POST /auth/google-drive/connect` (Protegida con JWT; `back/auth/cmd/server/main.go:110`).
- **Headers Requeridos:** `Authorization: Bearer <access_token>`, `Content-Type: application/json`.
- **Body:**
  ```json
  {
    "code": "4/0AeanS0...",
    "redirect_uri": "http://localhost:8081/auth/google/callback"
  }
  ```

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK`:** `{"status":"connected","provider":"google_drive"}`.
- **`400 Bad Request`:** `invalid_code`, código ya canjeado o expirado.
- **`401 Unauthorized`:** Sesión inválida.
- **`502 Bad Gateway`:** Error de red al comunicarse con Google OAuth API.

## 3. Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/auth/google_drive_service.dart:287` (`connectDrive(authCode)`).
  - Pantalla: `front/lib/features/academic/profile_screen.dart` (sección Google Drive).

## 4. Flujo Técnico (Paso a Paso)
1. Extrae `user_id` del JWT.
2. Canjea `code` contra el endpoint `https://oauth2.googleapis.com/token` enviando `GOOGLE_CLIENT_ID` y `GOOGLE_CLIENT_SECRET`.
3. Recibe `access_token`, `refresh_token` y `expires_in`.
4. Cifra los tokens usando AES-256-GCM (`OAUTH_ENCRYPTION_KEY`).
5. Realiza UPSERT en `identity.oauth_connections`:
   ```sql
   INSERT INTO identity.oauth_connections (user_id, provider, encrypted_access_token, encrypted_refresh_token, token_expires_at, scope)
   VALUES ($1, 'google_drive', $2, $3, $4, $5)
   ON CONFLICT (user_id, provider) DO UPDATE SET ...;
   ```

## 5. Dependencias y Datos Afectados
- **Tablas PostgreSQL:** `identity.oauth_connections`.
- **Servicios Externos:** Google OAuth 2.0 Token Endpoint.

## 6. Tests Asociados
- **Unitarios:** `back/auth/internal/handler/http/google_drive_handler_test.go`.
