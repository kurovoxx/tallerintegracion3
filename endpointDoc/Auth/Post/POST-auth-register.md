# POST /auth/register

Registra un nuevo usuario en la plataforma (`identity.users`) e inicializa su perfil básico (`identity.profiles`).

## 1. Req (Petición)
- **Método y Ruta:** `POST /auth/register` (Pública, sin autenticación previa; `back/auth/cmd/server/main.go:85`).
- **Headers Requeridos:** `Content-Type: application/json`.
- **Cuerpo (Body):**
  ```json
  {
    "email": "usuario@estudiantes.uct.cl",
    "password": "PasswordSegura123",
    "display_name": "Nombre Opcional"
  }
  ```
- **Validaciones de Entrada (`internal/utils/validator.go`):**
  - `email`: Obligatorio, validado con `mail.ParseAddress` y regex institucional/estándar (`ValidateEmail`).
  - `password`: Obligatorio, entre 8 y 72 caracteres (tope por limitación de bcrypt), al menos 1 letra y 1 número, sin espacios en blanco (`ValidatePassword`).
  - `display_name`: Opcional (1-100 caracteres; si no se provee, se deriva del local-part del email o fallback a "Usuario").

## 2. Res (Respuestas y Códigos HTTP)
- **`201 Created`:** Retorna los datos básicos del usuario creado.
  ```json
  {
    "id": "e4a2d3c1-1111-4444-8888-abcdef123456",
    "email": "usuario@estudiantes.uct.cl",
    "created_at": "2026-10-31T12:00:00Z"
  }
  ```
- **Errores Posibles:**
  - `400 Bad Request`: `invalid_email` (correo malformado), `weak_password` (no cumple requisitos de longitud o caracteres).
  - `409 Conflict`: `email_taken` ("Email ya registrado").

## 3. Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/core/services/auth_service.dart:18` (`registerPath = '/auth/register'`) y método `register(email, password)`.
  - Pantalla: `front/lib/features/auth/register_screen.dart` (formulario de registro).
- **Otros Microservicios:** No es consumido por otros servicios (registro exclusivo de usuarios finales).

## 4. Flujo Técnico (Paso a Paso)
1. **Handler (`internal/handler/http/auth_handler.go:142`):**
   - Hace binding de `registerRequest`. Valida formato de email y robustez de password con `validator.go`.
2. **Service (`internal/service/auth_service.go:133`):**
   - Genera el hash de la contraseña usando `bcrypt.GenerateFromPassword(..., bcrypt.DefaultCost)` (cost 10).
   - Deriva el `display_name` y normaliza campos opcionales (`normalizeOptional`).
3. **Repository (`internal/repository/user_repository.go:80`):**
   - Ejecuta una transacción `pgx.Tx`:
     - `INSERT INTO identity.users (email, password_hash) VALUES ($1, $2) RETURNING id, created_at;`
     - `INSERT INTO identity.profiles (user_id, display_name, visibility) VALUES ($1, $2, 'private');`
     - Captura error de violación de unicidad `23505` de PostgreSQL y lo mapea al error de dominio `ErrEmailTaken`.
   - `COMMIT` de la transacción.

## 5. Dependencias y Datos Afectados
- **Tablas PostgreSQL:** `identity.users`, `identity.profiles`.
- **Servicios Externos / Microservicios:** Ninguno.

## 6. Tests Asociados
- **Unitarios:** `back/auth/internal/handler/http/auth_handler_test.go` (`TestAuthHandler_Register_Success`, `TestAuthHandler_Register_WeakPassword`).
- **Integración:** `back/auth/internal/service/auth_integration_test.go` (`TestAuthIntegration_Register_Success`, `TestAuthIntegration_Register_DuplicateEmail_409`).
