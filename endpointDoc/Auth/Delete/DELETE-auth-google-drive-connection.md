# DELETE /auth/google-drive/connection

Desconecta la cuenta de Google Drive del usuario y revoca los tokens almacenados.

## 1. Req (Petición)
- **Método y Ruta:** `DELETE /auth/google-drive/connection` (Protegida con JWT; `back/auth/cmd/server/main.go:112`).
- **Headers:** `Authorization: Bearer <access_token>`. Sin body.

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK`:** `{"status":"disconnected"}`.
- **`404 Not Found`:** No existía conexión activa.

## 3. Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/auth/google_drive_service.dart:393` (`disconnectDrive()`).
  - Pantalla: `front/lib/features/academic/profile_screen.dart`.

## 4. Flujo Técnico (Paso a Paso)
1. Obtiene los tokens cifrados de `identity.oauth_connections`.
2. Intenta revocar el token en Google vía `https://oauth2.googleapis.com/revoke` (*best-effort*).
3. Elimina la fila de `identity.oauth_connections` para ese usuario y proveedor.

## 5. Dependencias y Datos Afectados
- **Tablas PostgreSQL:** `identity.oauth_connections`.
- **Servicios Externos:** Google OAuth Revocation Endpoint.
