# POST /auth/verify-email

Canjea el token recibido por correo y marca la cuenta como confirmada (`identity.users.email_verified_at`). El token es de un solo uso y el canje es atómico, también ante peticiones simultáneas.

El token llega en el correo enviado al registrarse ([POST /auth/register](POST-auth-register.md)) o al pedir un reenvío ([POST /auth/send-verification-email](POST-auth-send-verification-email.md)).

## 1. Req (Petición)
- **Método y Ruta:** `POST /auth/verify-email` (Pública, sin autenticación: el token prueba el acceso al correo; `back/auth/cmd/server/main.go:165`). Desde el navegador: `POST https://ti3-brojas.dev.censei.cl/api-auth/auth/verify-email`.
- **Headers Requeridos:** `Content-Type: application/json`.
- **Parámetros de ruta/query:** ninguno. El token **no** viaja en la URL de la API (no queda en logs de acceso ni de proxies). El enlace del correo (`EMAIL_VERIFICATION_URL?token=…`) apunta al frontend, que debe leer el parámetro y llamar a este endpoint con `POST`.
- **Cuerpo (Body):** límite 4096 bytes.
  ```json
  {
    "token": "q3Jm0bS7m2bVx3Zl8dFq1W9kYd2cN5pT0aRrU4sE6hI"
  }
  ```
- **Validaciones de Entrada:**
  - `token`: obligatorio. Se recortan espacios. Debe tener exactamente 43 caracteres base64url canónicos (32 bytes); cualquier otro valor se rechaza como `invalid_verification_token` **sin consultar la base**.

Ejemplo:

```http
POST /auth/verify-email
Content-Type: application/json

{"token":"q3Jm0bS7m2bVx3Zl8dFq1W9kYd2cN5pT0aRrU4sE6hI"}
```

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK` — confirmada ahora:**
  ```json
  {
    "status": "verified",
    "message": "Tu correo quedó confirmado."
  }
  ```
- **`200 OK` — ya estaba confirmada** (se abre otra vez el mismo enlace después de confirmar):
  ```json
  {
    "status": "already_verified",
    "message": "Tu correo ya estaba confirmado."
  }
  ```
  - Solo quien tiene el token puede obtener esta respuesta, así que no expone información a terceros.
- **Errores Posibles:**

| HTTP | `error.code` | Condición |
| --- | --- | --- |
| 400 | `bad_request` "Falta el código de confirmación." | JSON mal formado, sin `token`, `token` vacío o body > 4096 bytes |
| 400 | `invalid_verification_token` "El enlace de confirmación es inválido, expiró o ya fue reemplazado por uno más reciente. Solicita uno nuevo." | Formato inválido, token inexistente, reemplazado por un reenvío, expirado, o usado sin que la cuenta esté confirmada. Las causas son indistinguibles en la respuesta (sí aparecen en el log como `outcome=`) |
| 500 | `internal_error` "No se pudo confirmar el correo. Inténtalo más tarde." | Fallo de PostgreSQL. Nada queda a medias: la transacción se revierte |

- **Headers de respuesta:** `Cache-Control: no-store` en todas las respuestas.

## 3. Invocación (Front / Otros Servicios)
- **Frontend Flutter:** todavía no la consume. La pantalla que recibe el enlace o el código es una tarea posterior.
- **Otros Microservicios:** no la consumen.

## 4. Flujo Técnico (Paso a Paso)
1. **Handler** `EmailVerificationHandler.Verify` (`internal/handler/http/email_verification_handler.go`): `Cache-Control`, límite del body, binding y timeout de 10 s.
2. **Service** `EmailVerificationService.Confirm` (`internal/service/email_verification_service.go`): valida el formato y calcula `SHA-256(token)`.
3. **Repository** `EmailVerificationRepository.Confirm` (`internal/repository/email_verification_repository.go`), en una transacción:
   1. `SELECT user_id FROM identity.email_verifications WHERE token_hash = $1` (índice único). Sin fila → `invalid`.
   2. `SELECT email_verified_at IS NOT NULL FROM identity.users WHERE id = $1 FOR NO KEY UPDATE`.
   3. Relee el token bajo `FOR UPDATE` exigiendo `user_id` **y** `token_hash`: si un reenvío simultáneo lo reemplazó → `invalid`. Así un token solo puede confirmar la cuenta a la que pertenece.
   4. `consumed_at` fijado → `already_verified` (cuenta confirmada) o `used`; `expires_at <= now()` (hora de PostgreSQL) → `expired`.
   5. `UPDATE identity.users SET email_verified_at = now()` y `UPDATE identity.email_verifications SET consumed_at = now()`; `COMMIT`.
   - Mismo orden de bloqueo que la emisión (usuario → token): emisión y canje simultáneos no se interbloquean. Dos canjes simultáneos del mismo token: uno obtiene `verified` y el resto `already_verified`.
4. La fila se conserva tras el canje (límite de reenvíos y respuesta `already_verified`).

## 5. Dependencias y Datos Afectados
- **Tablas PostgreSQL:** `identity.users` (`email_verified_at`), `identity.email_verifications` (`consumed_at`). Migración `back/auth/db/migrations/007_email_verification.sql`.
- **Servicios Externos:** ninguno.
- **Login y refresh:** con `EMAIL_VERIFICATION_REQUIRED=true` exigen `email_verified_at`; el canje de este endpoint es lo que desbloquea [POST /auth/login](POST-auth-login.md) (`403 email_not_verified` antes) y [POST /auth/refresh](POST-auth-refresh.md) (`401 email_not_verified` antes). Con el gate apagado (por defecto) no lo miran.

## 6. Seguridad y Logs
- 256 bits de entropía: no se puede adivinar ni recorrer; por eso el endpoint no necesita límite de intentos.
- Se guarda solo el `SHA-256` y se busca por índice. No hace falta comparación en tiempo constante: medir la búsqueda no ayuda sin una preimagen del hash.
- Uso único, expiración (`EMAIL_VERIFICATION_TTL`, 24 h por defecto) y reemplazo en cada reenvío.
- Logs (nunca el token, su hash ni el correo):
  ```text
  [auth] POST /auth/verify-email op=VerifyEmail status=200 user_id=- outcome=verified duration_ms=6
  [auth] email-verification op=VerifyEmail user_id=<uuid> step=confirm status=ok outcome=verified
  [auth] POST /auth/verify-email op=VerifyEmail status=400 user_id=- code=invalid_verification_token outcome=expired duration_ms=4
  [auth] email-verification op=VerifyEmail user_id=- step=validate status=rejected outcome=malformed
  [auth] POST /auth/verify-email op=VerifyEmail status=500 user_id=- step=confirm code=internal_error duration_ms=5001
  ```

## 7. Tests Asociados
- **Unitarios:** `back/auth/internal/service/email_verification_service_test.go` (`TestVerificationToken_*`, `TestEmailVerification_Confirm`, `..._LogsNeverLeakSecrets`).
- **Handler:** `back/auth/internal/handler/http/email_verification_handler_test.go` (`TestEmailVerificationHandler_VerifyOutcomes`, `..._VerifyExpiredAndInternalError`, `..._LogsAreUsefulAndSafe`).
- **Integración (PostgreSQL):** `back/auth/internal/service/email_verification_integration_test.go` (`..._RegisterPersistsHashAndConfirms`, `..._InvalidExpiredAndReplacedTokens`, `..._TokenOnlyVerifiesItsOwnAccount`, `..._ConcurrentConfirmConsumesOnce`, `..._ConcurrentIssueAndConfirm`, `..._DatabaseFailures`).
