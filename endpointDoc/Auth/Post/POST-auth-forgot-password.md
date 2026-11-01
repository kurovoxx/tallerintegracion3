# POST /auth/forgot-password

Solicita la recuperación de contraseña generando un código numérico temporal enviado por correo (o logueado en modo dev).

## 1. Req (Petición)
- **Método y Ruta:** `POST /auth/forgot-password` (Pública; `back/auth/cmd/server/main.go:89`).
- **Body:** `{"email": "usuario@estudiantes.uct.cl"}`.

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK`:** `{"message":"Si el correo existe, se ha enviado un código de recuperación"}` (Respuesta genérica anti-enumeración de cuentas).
- **`400 Bad Request`:** `invalid_email` (correo con sintaxis no válida).

## 3. Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/core/services/auth_service.dart:23` (`forgotPassword(email)`).
  - Pantalla: `front/lib/features/auth/password_recovery_screen.dart:45`.

## 4. Flujo Técnico (Paso a Paso)
1. `PasswordResetHandler.Forgot` valida formato del correo.
2. Si el email existe en `identity.users`, genera un código numérico aleatorio de 6 dígitos.
3. Almacena en `identity.password_resets`: `user_id`, `code_hash` (bcrypt), `expires_at` (15 min) e inicializa `attempts = 0`.
4. Envía el código mediante el proveedor SMTP configurado o lo imprime en consola si `SMTP_HOST` está vacío.

## 5. Dependencias y Datos Afectados
- **Tablas PostgreSQL:** `identity.password_resets`, `identity.users`.
- **Servicios Externos:** Servidor SMTP (envío de correos transaccionales).

## 6. Tests Asociados
- **Unitarios:** `back/auth/internal/service/password_reset_service_test.go`.
