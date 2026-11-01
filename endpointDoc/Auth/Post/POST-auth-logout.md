# POST /auth/logout

Cierra la sesión del usuario revocando de forma inmediata su Refresh Token en la base de datos.

## 1. Req (Petición)
- **Método y Ruta:** `POST /auth/logout` (Ruta con revocación por token; `back/auth/cmd/server/main.go:88`).
- **Headers Requeridos:** `Content-Type: application/json`.
- **Cuerpo (Body):**
  ```json
  {
    "refresh_token": "3a1b2c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1b2c"
  }
  ```

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK`:** Operación exitosa.
  ```json
  {
    "message": "Sesión cerrada exitosamente"
  }
  ```
- **Errores Posibles:**
  - `400 Bad Request`: `bad_request` (JSON inválido o sin token).
  - *(Nota: Si el token ya estaba revocado o no existe, responde 200 de forma idempotente para permitir limpieza segura en el cliente).*

## 3. Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/core/services/auth_service.dart:20` (`logoutPath = '/auth/logout'`) y método `logout()`.
  - UI: `front/lib/features/academic/profile_screen.dart` (botón de "Cerrar Sesión").

## 4. Flujo Técnico (Paso a Paso)
1. **Handler (`internal/handler/http/auth_handler.go:115`):**
   - Recibe el token y valida formato.
2. **Service (`internal/service/auth_service.go:220`):**
   - Calcula el hash SHA-256 del token.
3. **Repository (`internal/repository/refresh_token_repository.go:70`):**
   - Ejecuta `UPDATE identity.refresh_tokens SET revoked = true WHERE token_hash = $1`.

## 5. Dependencias y Datos Afectados
- **Tablas PostgreSQL:** `identity.refresh_tokens`.

## 6. Tests Asociados
- **Integración:** `back/auth/internal/service/auth_integration_test.go` (`TestAuthIntegration_Logout`).
- **E2E:** `back/auth/internal/handler/http/e2e_full_flow_test.go` (`TestE2E_RegisterLoginMeRefreshLogout`).
