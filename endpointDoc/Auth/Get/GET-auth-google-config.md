# GET /auth/google-config

Entrega al cliente la configuración pública de OAuth de Google (Client ID y Scopes) para evitar quemar credenciales en el cliente web/móvil.

## 1. Req (Petición)
- **Método y Ruta:** `GET /auth/google-config` (Pública; `back/auth/cmd/server/main.go:96`).
- **Headers:** Ninguno requerido.

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK`:**
  ```json
  {
    "client_id": "123456789-abc.apps.googleusercontent.com",
    "drive_scopes": "https://www.googleapis.com/auth/drive.file openid email profile",
    "calendar_scopes": "https://www.googleapis.com/auth/calendar.events openid email profile"
  }
  ```

## 3. Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/features/auth/google_drive_service.dart:95` y `google_calendar_service.dart:110`.
  - Pantallas: Vinculación de Drive en Perfil y Agendar Reunión con Calendar.

## 4. Flujo Técnico (Paso a Paso)
1. `DriveHandler.GetGoogleConfig` lee las variables de entorno `GOOGLE_CLIENT_ID` y scopes preconfigurados.
2. Retorna el payload JSON sin exponer el `client_secret`.

## 5. Dependencias y Datos Afectados
- **Configuración:** Variables de entorno del servidor.
