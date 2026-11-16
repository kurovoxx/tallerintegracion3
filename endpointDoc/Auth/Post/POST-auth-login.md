# POST /auth/login

Autentica a un usuario mediante email y contraseña, generando un par de tokens (Access Token JWT de corta duración y Refresh Token opaco rotativo).

## 1. Req (Petición)
- **Método y Ruta:** `POST /auth/login` (Pública, sin auth; `back/auth/cmd/server/main.go:86`).
- **Headers Requeridos:** `Content-Type: application/json`.
- **Cuerpo (Body):**
  ```json
  {
    "email": "usuario@estudiantes.uct.cl",
    "password": "PasswordSegura123"
  }
  ```
- **Validaciones de Entrada:** `email` y `password` requeridos en el JSON.

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK`:** Retorna los tokens de sesión.
  ```json
  {
    "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "refresh_token": "9f8a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a",
    "expires_in": 900
  }
  ```
- **Errores Posibles:**
  - `400 Bad Request`: `bad_request` (JSON malformado o campos faltantes).
  - `401 Unauthorized`: `invalid_credentials` (email no encontrado o hash de contraseña no coincide; tiempo constante contra enumeración de cuentas).
  - `403 Forbidden`: `email_not_verified`, solo con `EMAIL_VERIFICATION_REQUIRED=true`: credenciales correctas, pero la cuenta no confirmó su correo. No se emite ningún token.
    ```json
    {
      "error": {
        "code": "email_not_verified",
        "message": "Confirma tu correo para iniciar sesión. Revisa tu bandeja de entrada o solicita un nuevo correo de confirmación."
      }
    }
    ```
    - Solo aparece con la contraseña correcta: no permite descubrir qué correos están registrados. Contraseña incorrecta, cuenta inexistente o cuenta en borrado siguen siendo `401 invalid_credentials`.
    - El cliente ofrece reenviar el correo ([POST /auth/send-verification-email](POST-auth-send-verification-email.md)) y canjearlo ([POST /auth/verify-email](POST-auth-verify-email.md)). El Flutter actual muestra el `message`; detectar el `code` corresponde a la tarea de Flutter.
  - `500 Internal Server Error`: `internal_error` si falla la consulta de confirmación (gate activo): no se emite sesión.

## 3. Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/core/services/auth_service.dart:17` (`loginPath = '/auth/login'`) y método `login(email, password)`.
  - Pantalla: `front/lib/features/auth/login_screen.dart` (pantalla de inicio de sesión).
  - `LoginResult.errorCode` conserva `error.code`. Con `403 email_not_verified` el login abre el diálogo «Confirma tu correo» (`front/lib/features/auth/email_not_verified_dialog.dart`) con «Reenviar correo» ([POST /auth/send-verification-email](POST-auth-send-verification-email.md)) y «Tengo un código» (`EmailVerificationScreen` con el correo del formulario). El resto de errores (401 `invalid_credentials`, red, etc.) muestran el aviso de siempre.
  - Un `401 email_not_verified` en `POST /auth/refresh` lo trata la lógica de sesión existente como refresh rechazado: cierra la sesión una vez y vuelve al login («Tu sesión expiró»); el siguiente login muestra el 403 con el reenvío.
- **Pruebas de Carga (Loadgen):**
  - Script: `scripts/fortio-kanban.sh:58` invoca `POST /auth/login` para obtener el Bearer token antes de atacar el cluster.

## 4. Flujo Técnico (Paso a Paso)
1. **Handler (`internal/handler/http/auth_handler.go:65`):**
   - Valida que vengan `email` y `password`.
2. **Service (`internal/service/auth_service.go:157`):**
   - Consulta el usuario por email (`userRepo.GetByEmail`). Si no existe, realiza un dummy hash check para mitigar ataques de timing y retorna `invalid_credentials`.
   - Compara la contraseña plana con `bcrypt.CompareHashAndPassword`.
   - **Gate de confirmación** (`requireVerifiedEmail`, solo con `EMAIL_VERIFICATION_REQUIRED=true`): después de las credenciales y del estado de borrado, y antes de emitir tokens, `EmailVerificationRepository.IsEmailVerified` ejecuta `SELECT email_verified_at IS NOT NULL FROM identity.users WHERE id = $1`. Si es `false` → `email_not_verified`. Con el gate apagado (por defecto) no se consulta nada y el comportamiento es el de antes.
   - Genera el Access Token JWT (`jwt_service.go:74`) firmado con HMAC-SHA256 (`JWT_SECRET`), emisor `iss=apuntes-auth`, audiencia `aud=apuntes-client`, duración 15 minutos (900s).
   - Genera un Refresh Token aleatorio criptoseguro de 32 bytes (64 caracteres hex) con expiración de 7 días.
3. **Repository (`internal/repository/refresh_token_repository.go`):**
   - Calcula el hash SHA-256 del refresh token y lo inserta en `identity.refresh_tokens`.

## 5. Dependencias y Datos Afectados
- **Tablas PostgreSQL:** `identity.users` (lectura; `email_verified_at` con el gate activo, migraciones `007_email_verification.sql` y `008_email_verification_backfill.sql`), `identity.refresh_tokens` (inserción).
- **Configuración:** `EMAIL_VERIFICATION_REQUIRED` (por defecto `false`). Con `true`, Auth no arranca si faltan 007/008 o SMTP (ver `documentation/email-verification.md`).
- **Criptografía:** `bcrypt` (cost 10), `crypto/rand`, `golang-jwt/jwt/v5` (HS256).

## 6. Tests Asociados
- **Unitarios:** `back/auth/internal/handler/http/auth_handler_test.go` (`TestLogin_Success`, `TestLogin_InvalidCredentials_401`).
- **Cobertura:** `back/auth/internal/handler/http/auth_extra_coverage_test.go` (`TestCoverage_Login_JSONMalformado_400`, `TestCoverage_Login_EmailInexistente_401`).
- **E2E:** `back/auth/internal/handler/http/e2e_full_flow_test.go` (`TestE2E_RegisterLoginMeRefreshLogout`).
- **Gate de confirmación:** `back/auth/internal/service/email_verification_gate_test.go` (`TestEmailGate_*`), `back/auth/internal/handler/http/email_verification_gate_handler_test.go` (`TestE2E_EmailVerificationGate_RegisterLoginVerifyLogin`), `back/auth/internal/service/email_verification_gate_integration_test.go` (`TestAuthIntegration_EmailVerification_GateLoginRefreshFlow`).
