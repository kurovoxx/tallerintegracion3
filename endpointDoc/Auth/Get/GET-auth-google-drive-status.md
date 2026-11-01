# GET /auth/google-drive/status

Verifica si el usuario tiene una conexión activa y válida con Google Drive.

## 1. Req (Petición)
- **Método y Ruta:** `GET /auth/google-drive/status` (Protegida con JWT; `back/auth/cmd/server/main.go:111`).
- **Headers:** `Authorization: Bearer <access_token>`.

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK`:**
  ```json
  {
    "connected": true,
    "provider": "google_drive",
    "scope": "https://www.googleapis.com/auth/drive.file"
  }
  ```

## 3. Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/core/services/profile_service.dart:45` y `front/lib/features/auth/google_drive_service.dart`.
  - Pantalla: `front/lib/features/academic/profile_screen.dart` (muestra el switch o badge de estado de Drive).

## 4. Flujo Técnico (Paso a Paso)
1. Consulta `identity.oauth_connections` buscando `provider = 'google_drive'` para el `user_id`.
2. Si existe fila no revocada, retorna `connected: true`. De lo contrario, `connected: false`.

## 5. Dependencias y Datos Afectados
- **Tablas PostgreSQL:** `identity.oauth_connections` (lectura).
