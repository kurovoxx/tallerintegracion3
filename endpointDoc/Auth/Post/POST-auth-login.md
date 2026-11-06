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

## 3. Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/core/services/auth_service.dart:17` (`loginPath = '/auth/login'`) y método `login(email, password)`.
  - Pantalla: `front/lib/features/auth/login_screen.dart` (pantalla de inicio de sesión).
- **Pruebas de Carga (Loadgen):**
  - Script: `scripts/fortio-kanban.sh:58` invoca `POST /auth/login` para obtener el Bearer token antes de atacar el cluster.

## 4. Flujo Técnico (Paso a Paso)
1. **Handler (`internal/handler/http/auth_handler.go:65`):**
   - Valida que vengan `email` y `password`.
2. **Service (`internal/service/auth_service.go:157`):**
   - Consulta el usuario por email (`userRepo.GetByEmail`). Si no existe, realiza un dummy hash check para mitigar ataques de timing y retorna `invalid_credentials`.
   - Compara la contraseña plana con `bcrypt.CompareHashAndPassword`.
   - Genera el Access Token JWT (`jwt_service.go:74`) firmado con HMAC-SHA256 (`JWT_SECRET`), emisor `iss=apuntes-auth`, audiencia `aud=apuntes-client`, duración 15 minutos (900s).
   - Genera un Refresh Token aleatorio criptoseguro de 32 bytes (64 caracteres hex) con expiración de 7 días.
3. **Repository (`internal/repository/refresh_token_repository.go`):**
   - Calcula el hash SHA-256 del refresh token y lo inserta en `identity.refresh_tokens`.

## 5. Dependencias y Datos Afectados
- **Tablas PostgreSQL:** `identity.users` (lectura), `identity.refresh_tokens` (inserción).
- **Criptografía:** `bcrypt` (cost 10), `crypto/rand`, `golang-jwt/jwt/v5` (HS256).

## 6. Tests Asociados
- **Unitarios:** `back/auth/internal/handler/http/auth_handler_test.go` (`TestLogin_Success`, `TestLogin_InvalidCredentials_401`).
- **Cobertura:** `back/auth/internal/handler/http/auth_extra_coverage_test.go` (`TestCoverage_Login_JSONMalformado_400`, `TestCoverage_Login_EmailInexistente_401`).
- **E2E:** `back/auth/internal/handler/http/e2e_full_flow_test.go` (`TestE2E_RegisterLoginMeRefreshLogout`).
