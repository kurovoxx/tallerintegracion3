# `POST /auth/refresh`

Renueva un Access Token expirado utilizando un Refresh Token activo e implementa **rotación de tokens** (revoca el token usado y emite uno nuevo para detectar reutilizaciones no autorizadas).

## 1. Req (Petición)
- **Método y Ruta:** `POST /auth/refresh` (Pública, no exige JWT previo; `back/auth/cmd/server/main.go:87`).
- **Headers Requeridos:** `Content-Type: application/json`.
- **Cuerpo (Body):**
  ```json
  {
    "refresh_token": "9f8a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a"
  }
  ```

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK`:** Retorna el nuevo par de tokens.
  ```json
  {
    "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "refresh_token": "3a1b2c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1b2c",
    "expires_in": 900
  }
  ```
- **Errores Posibles:**
  - `400 Bad Request`: `bad_request` (cuerpo malformado o sin `refresh_token`).
  - `401 Unauthorized`: `invalid_token` (token expirado, revocado, no encontrado o reutilizado fraudulentamente).
  - `401 Unauthorized`: `email_not_verified` (solo con `EMAIL_VERIFICATION_REQUIRED=true`): el refresh es válido, pero la cuenta no confirmó su correo, típicamente una sesión iniciada antes de activar el gate. Es 401 y no 403 porque el Flutter actual solo cierra la sesión ante un 401; cualquier otro estado lo trata como fallo transitorio. El refresh **no se revoca**: tras confirmar el correo vuelve a servir.
  - `423 Locked`: `account_deletion_in_progress` (la cuenta está en proceso de borrado).

## 3. Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/core/services/auth_service.dart:19` (`refreshPath = '/auth/refresh'`) y `front/lib/core/services/authed_client.dart` (`AuthedHttp.run` intercepta errores 401 para hacer refresh transparente).
  - Manejador de sesión: `front/lib/core/services/session_manager.dart`.
- **Otros Microservicios:** No es consumido directamente por otros servicios.

## 4. Flujo Técnico (Paso a Paso)
1. **Handler (`internal/handler/http/auth_handler.go:90`):**
   - Extrae `refresh_token` del body.
2. **Service (`internal/service/auth_service.go:195`):**
   - Calcula el SHA-256 del token provisto.
   - Consulta `identity.refresh_tokens`. Si ya fue revocado, activa alarma de robo de token y revoca todas las sesiones activas de ese usuario.
   - Comprueba que `expires_at > now()`.
   - Verifica que la cuenta no esté bloqueada por borrado (`AccountAccessGate`).
   - **Gate de confirmación** (solo con `EMAIL_VERIFICATION_REQUIRED=true`): después de validar el refresh (inexistente, revocado o expirado conservan sus errores) comprueba `email_verified_at` del usuario; si es NULL → `email_not_verified` sin emitir tokens. Así una sesión anterior a la activación no se prolonga: su access token deja de valer como máximo cuando expira (`ACCESS_TOKEN_EXPIRES_IN`, 900 s por defecto).
3. **Rotación Transaccional (`internal/repository/refresh_token_repository.go`):**
   - Marca el token anterior como `revoked = true`.
   - Genera un nuevo Refresh Token y lo almacena con su hash SHA-256.
   - Emite un nuevo JWT Access Token.

## 5. Dependencias y Datos Afectados
- **Tablas PostgreSQL:** `identity.refresh_tokens` (`UPDATE revoked=true`, `INSERT nuevo token`).

## 6. Tests Asociados
- **Unitarios:** `back/auth/internal/service/auth_service_test.go` (`TestRefresh_TokenRotation`, `TestRefresh_RevokedToken_RevokesAll`).
- **Integración:** `back/auth/internal/service/auth_integration_test.go` (`TestAuthIntegration_RefreshToken`).
- **Gate de confirmación:** `back/auth/internal/service/email_verification_gate_test.go` (`TestEmailGate_RefreshCannotExtendUnverifiedSession`, `TestEmailGate_RefreshInvalidTokenTakesPrecedence`), `back/auth/internal/handler/http/email_verification_gate_handler_test.go` (`TestE2E_EmailVerificationGate_RefreshOfPreviousSessionIs401`).
