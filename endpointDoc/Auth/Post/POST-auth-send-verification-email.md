# POST /auth/send-verification-email

Solicita (o vuelve a solicitar) el correo de confirmación de cuenta. Genera un token nuevo, invalida el anterior y lo envía por SMTP. La respuesta es la misma exista o no la cuenta, para que el endpoint no sirva para descubrir correos registrados.

El primer correo se envía automáticamente al registrarse ([POST /auth/register](POST-auth-register.md)); este endpoint es el reenvío. Para canjear el token: [POST /auth/verify-email](POST-auth-verify-email.md).

## 1. Req (Petición)
- **Método y Ruta:** `POST /auth/send-verification-email` (Pública, sin autenticación; `back/auth/cmd/server/main.go:164`). Desde el navegador: `POST https://ti3-brojas.dev.censei.cl/api-auth/auth/send-verification-email` (nginx quita el prefijo `/api-auth`).
- **Headers Requeridos:** `Content-Type: application/json`.
- **Parámetros de ruta/query:** ninguno.
- **Cuerpo (Body):** límite 4096 bytes (`http.MaxBytesReader`).
  ```json
  {
    "email": "usuario@estudiantes.uct.cl"
  }
  ```
- **Validaciones de Entrada:**
  - `email`: obligatorio (binding `required`). Se recorta y pasa a minúsculas; luego `utils.ValidateEmail` (`internal/utils/validator.go`). Un correo con saltos de línea u otros caracteres inválidos se rechaza antes de tocar la base o el SMTP.

Ejemplo:

```http
POST /auth/send-verification-email
Content-Type: application/json

{"email":"usuario@estudiantes.uct.cl"}
```

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK`:** respuesta genérica, idéntica en todos estos casos: cuenta registrada y pendiente (se envía correo), correo no registrado, cuenta ya confirmada, reenvío dentro del enfriamiento o sobre el tope por hora, envío descartado porque los 4 cupos están ocupados, fallo del servidor SMTP y fallo al guardar el token.
  ```json
  {
    "message": "Si el correo está registrado y aún no fue confirmado, recibirás un enlace de confirmación. Revisa también la carpeta de spam."
  }
  ```
  - `200` **no** confirma que el correo se haya entregado.
- **Errores Posibles** (formato común `{"error":{"code","message"}}`):

| HTTP | `error.code` | Condición |
| --- | --- | --- |
| 400 | `bad_request` "Ingresa un correo válido." | JSON mal formado, sin `email`, `email` vacío o body > 4096 bytes |
| 400 | `invalid_email` "Formato de email inválido" | `email` presente pero con formato inválido |
| 503 | `mail_unavailable` "El envío de correos no está disponible temporalmente. Inténtalo más tarde." | Auth arrancó sin `SMTP_PASSWORD`. Se decide antes de buscar la cuenta: no revela nada |
| 500 | `internal_error` "No se pudo procesar la solicitud. Inténtalo más tarde." | Falló la búsqueda de la cuenta en PostgreSQL (ocurre igual para cualquier correo) |

- **Headers de respuesta:** `Cache-Control: no-store` en todas las respuestas.

## 3. Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `AuthService.sendVerificationEmail(email)` (`front/lib/core/services/auth_service.dart`), que normaliza el correo (trim + minúsculas) y devuelve `LoginResult` (`success` con el 200; si no, `message` con el texto del backend, p. ej. el de `mail_unavailable`, o «No se pudo completar la solicitud…» sin conexión o con una respuesta no JSON).
  - Pantalla: botón «Reenviar correo» de `EmailVerificationScreen` (`front/lib/features/auth/email_verification_screen.dart`). Con 200 muestra siempre el mismo aviso genérico (`emailVerificationResentMessage`: «Si el correo está registrado y aún no fue confirmado, recibirás un nuevo código… Revisa también la carpeta de spam.») y bloquea el botón 60 s (mismo enfriamiento que el backend). Con error muestra el mensaje y **no** afirma que el correo salió ni inicia la cuenta atrás.
  - Login: opción «Reenviar correo» del diálogo del 403 `email_not_verified` (`front/lib/features/auth/email_not_verified_dialog.dart`). La petición se hace con el diálogo abierto: «Reenviar correo» muestra un indicador y todos los botones quedan deshabilitados, así que no hay solicitudes duplicadas. Si el backend acepta, cierra el diálogo y abre una sola pantalla con el aviso genérico y la cuenta atrás; si falla (503, sin conexión), muestra el error dentro del diálogo, sin afirmar que el correo salió, y permite reintentar.
- **Otros Microservicios:** no la consumen.

## 4. Flujo Técnico (Paso a Paso)
1. **Handler** `EmailVerificationHandler.Send` (`internal/handler/http/email_verification_handler.go`): fija `Cache-Control`, limita el body, hace binding y llama al servicio con timeout de 10 s.
2. **Service** `EmailVerificationService.Request` (`internal/service/email_verification_service.go`):
   - normaliza y valida el correo; si SMTP no está listo responde `mail_unavailable`;
   - busca la cuenta (`UserRepository.GetByEmail`). Si no existe, responde lo mismo que si existiera;
   - si existe, `dispatchSend` intenta ocupar uno de los **4 cupos** de envío de Auth (compartidos con el registro) **sin esperar**. Con cupo, lanza una goroutine con contexto propio (`context.WithoutCancel` + 30 s) que libera el cupo al terminar (`defer`, también ante error o panic). Sin cupo, descarta el envío al instante (`step=dispatch status=skipped reason=busy`) antes de generar o guardar token, por lo que no consume enfriamiento ni tope y la respuesta sigue siendo el mismo `200`;
   - en ambos casos responde sin esperar al SMTP, así que el tiempo de respuesta tampoco revela si la cuenta existe. El límite de 4 deja libre al menos una de las 5 conexiones del pool para login y el resto de Auth.
3. **Emisión** (`issueAndSend`):
   - genera 32 bytes con `crypto/rand` → token base64url de 43 caracteres (256 bits). Persiste solo su `SHA-256`;
   - `EmailVerificationRepository.Issue` (`internal/repository/email_verification_repository.go`), en una transacción:
     - `SELECT email_verified_at IS NOT NULL FROM identity.users WHERE id = $1 FOR NO KEY UPDATE` → inexistente o ya confirmada: no emite;
     - `INSERT … ON CONFLICT (user_id) DO UPDATE … WHERE requested_at <= now() - 60 s AND (ventana vencida OR sends_in_window < 5)`: reemplaza el token anterior (queda inválido), reinicia `consumed_at` y lleva la cuenta de la ventana de 1 hora. 0 filas = limitado.
4. **Envío:** `SMTPMailer.SendVerificationEmail` (`internal/service/email_verification_service.go`, transporte en `internal/service/smtp_mailer.go`). El correo incluye el enlace `EMAIL_VERIFICATION_URL?token=<token>` si está configurado y siempre el token, la vigencia y el aviso de uso único.
5. **Fallo de SMTP:** `ReleaseCooldown` retrocede `requested_at` (solo si ese token sigue vigente y sin usar) para permitir un reintento inmediato. El token no se borra: si el fallo fue tardío, el correo pudo haber salido. El tope por hora no se devuelve.

## 5. Dependencias y Datos Afectados
- **Tablas PostgreSQL:** `identity.users` (lectura, bloqueo de fila), `identity.email_verifications` (upsert). Migración `back/auth/db/migrations/007_email_verification.sql`.
- **Servicios Externos:** servidor SMTP (`SMTP_HOST`/`SMTP_PORT`, STARTTLS o TLS implícito, TLS 1.2+).
- **Configuración:** `SMTP_*`, `EMAIL_VERIFICATION_URL`, `EMAIL_VERIFICATION_TTL` (ver `documentation/email-verification.md`).

## 6. Seguridad y Logs
- Anti-enumeración: mismo `200` y mismo cuerpo para cuentas existentes y desconocidas; el envío ocurre fuera de la petición.
- Abuso: 60 s entre envíos y máximo 5 por hora por cuenta, evaluados en el mismo `UPSERT` (válido con varias réplicas). Además, como máximo 4 envíos en curso por réplica de Auth: no se crean goroutines por encima de ese número. No hay rate limiting por IP en Auth.
- La base nunca guarda el token en claro; un reenvío invalida el anterior.
- Logs (`[auth]`, mismo formato que `logAuthOp`). Nunca incluyen correo, token, hash, enlace ni el texto de error del SMTP o de PostgreSQL:
  ```text
  [auth] POST /auth/send-verification-email op=SendVerificationEmail status=200 user_id=- accepted=true duration_ms=3
  [auth] email-verification op=SendVerificationEmail user_id=<uuid> step=send status=ok outcome=sent
  [auth] email-verification op=SendVerificationEmail user_id=- step=lookup status=ok outcome=unknown_email
  [auth] email-verification op=SendVerificationEmail user_id=<uuid> step=persist status=skipped outcome=throttled
  [auth] email-verification op=SendVerificationEmail user_id=<uuid> step=dispatch status=skipped reason=busy
  [auth] email-verification op=SendVerificationEmail user_id=<uuid> step=dispatch status=error cause=panic
  [auth] email-verification op=SendVerificationEmail user_id=<uuid> step=send status=error smtp_stage=auth smtp_code=535
  [auth] email-verification op=SendVerificationEmail user_id=<uuid> step=persist status=error sqlstate=42703
  ```
  `smtp_stage`: `config`, `address`, `dial`, `tls`, `greeting`, `starttls`, `auth`, `mail_from`, `rcpt`, `data`.
  `sqlstate=42703` (columna inexistente) es lo que se ve si la migración 007 no está aplicada: la emisión empieza leyendo `identity.users.email_verified_at`. `42P01` solo aparece si existe la columna pero falta la tabla `identity.email_verifications`.

## 7. Tests Asociados
- **Unitarios:** `back/auth/internal/service/email_verification_service_test.go` (`TestEmailVerification_Request`, `..._RequestDispatchesOutsideRequest`, `..._LogsNeverLeakSecrets`), `back/auth/internal/service/email_verification_dispatch_test.go` (4 cupos, descarte inmediato sin token, cupos compartidos con el registro, liberación tras éxito/fallo/panic, concurrencia), `back/auth/internal/service/smtp_mailer_test.go` (servidor SMTP falso con TLS).
- **Handler:** `back/auth/internal/handler/http/email_verification_handler_test.go` (`TestEmailVerificationHandler_SendIsGenericAndNoStore`, `..._SendErrors`, `..._LogsAreUsefulAndSafe`).
- **Integración (PostgreSQL):** `back/auth/internal/service/email_verification_integration_test.go` (`TestAuthIntegration_EmailVerification_HourlyLimit`, `..._ConcurrentRequestsIssueOnce`, `..._SMTPFailureAllowsImmediateRetry`, `..._BusyDispatchLeavesNoTraceAndResendWorks`, `..._DatabaseFailures`).
