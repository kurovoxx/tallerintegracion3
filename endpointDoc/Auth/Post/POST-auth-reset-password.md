# POST /auth/reset-password

Restablece la contraseña utilizando el código de 6 dígitos enviado previamente.

## 1. Req (Petición)
- **Método y Ruta:** `POST /auth/reset-password` (Pública; `back/auth/cmd/server/main.go:90`).
- **Body:**
  ```json
  {
    "email": "usuario@estudiantes.uct.cl",
    "code": "123456",
    "new_password": "NuevaPassword123"
  }
  ```

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK`:** `{"message":"Contraseña restablecida exitosamente"}`.
- **`400 Bad Request`:** `invalid_code` (código erróneo o expirado), `weak_password`, o `too_many_attempts` (máximo 5 intentos fallidos).

## 3. Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/core/services/auth_service.dart:31` (`resetPassword(...)`).
  - Pantalla: `front/lib/features/auth/password_recovery_screen.dart:110`.

## 4. Flujo Técnico (Paso a Paso)
1. Verifica que el código no haya expirado y que `attempts < 5`. Si falla, incrementa `attempts`.
2. Valida la nueva contraseña con `validator.ValidatePassword`.
3. Actualiza `password_hash` en `identity.users`. Si la migración 007 estaba aplicada al arrancar Auth (`PasswordResetRepository.EnableEmailVerification`), la **misma sentencia** también confirma el correo: `UPDATE identity.users SET password_hash = $2, email_verified_at = COALESCE(email_verified_at, now()) WHERE id = $1`. El código llegó a esa misma dirección, así que prueba el acceso al correo. Es atómico con el cambio de contraseña y solo ocurre tras validar el código; una confirmación previa conserva su fecha. Sin 007, el restablecimiento funciona como antes.
4. Elimina la fila de `identity.password_resets`.
5. Revoca todos los refresh tokens activos del usuario para forzar re-login con la nueva clave.

## 5. Dependencias y Datos Afectados
- **Tablas PostgreSQL:** `identity.users` (`password_hash` y, con 007, `email_verified_at`), `identity.password_resets`, `identity.refresh_tokens`.

## 6. Tests Asociados
- **Integración:** `back/auth/internal/service/password_reset_test.go`.
- **Confirmación por restablecimiento:** `back/auth/internal/service/email_verification_gate_integration_test.go` (`TestAuthIntegration_EmailVerification_PasswordResetMarksVerified`).
