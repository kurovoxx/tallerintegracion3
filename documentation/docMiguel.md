

## 1. `POST /auth/register`

Contrato: `agentApiContract2.md §1` → `201 {id,email,created_at}`, `409` email ya existe.
Ruta: `back/auth/cmd/server/main.go:85` → `AuthHandler.Register`
(`back/auth/internal/handler/http/auth_handler.go:142-180`).

### 1.1 Validar email único, formato y fortaleza mínima
- Forma: `binding:"required"` en `registerRequest` (`auth_handler.go:21-30`).
- Formato: `utils.ValidateEmail` — `mail.ParseAddress` + regex
  (`back/auth/internal/utils/validator.go:14-23`).
- Fortaleza: `utils.ValidatePassword` — 8-72 chars (límite bcrypt), ≥1 letra, ≥1 dígito,
  sin espacios (`validator.go:31-48`). Códigos `invalid_email` / `weak_password`
  (`back/auth/internal/utils/response.go:34-35`, mensajes en `MessageForCode`).
- Unicidad: no se chequea con `SELECT` previo; se inserta y se mapea el `23505`
  de Postgres a `email_taken` (`back/auth/internal/repository/user_repository.go:100-106`
  → `auth_service.go:141-143` → `StatusForCode` 409 en `response.go:77-78`).
  El handler además loguea `auth register fail ... code=email_taken`
  (`auth_handler.go:183`).

### 1.2 Hash bcrypt antes de persistir
`auth_service.go:133` `bcrypt.GenerateFromPassword(..., DefaultCost)` (cost 10).
Nunca se guarda ni se loguea el password plano (los logs solo llevan `email`, `id`, `ip`).

### 1.3 Insert en `identity.users` + fila inicial en `identity.profiles`
Transacción única en `user_repository.go` (`CreateUser`): `INSERT identity.users`
+ `INSERT identity.profiles (user_id, display_name, ..., visibility)` + `COMMIT`.
`display_name` es `NOT NULL` (`agentSql.md:40-49`); como el contrato ya no lo pide,
se deriva del local-part del email o del campo opcional (1-100 chars), fallback `"Usuario"`
(`auth_service.go:93-117`). Opcionales (`photo_url`, `phone`, `institution`, `description`)
van `trim` y vacío → `NULL` (`normalizeOptional`). `visibility` por defecto `private`,
solo `public`/`private` (`auth_service.go:119-125`, `agentSql.md:47`).

### 1.4 Error 409 si el email ya existe
Flujo: `users.CreateUser` → error `email_taken` → `ServiceError(ErrEmailTaken)` →
`StatusForCode` → `409` + `{"error":{"code":"email_taken","message":"Email ya registrado"}}`.

### 1.5 Tests unitarios
- Servicio (`back/auth/internal/service/auth_service_test.go`): registro exitoso sin role,
  email duplicado → `email_taken`, email inválido → 400, password débil → 400,
  `display_name` derivado/explícito/vacío/largo, local-part largo truncado a 100.
  **No hay test de rol inválido** (no existe el campo, ver §0).
- Handler (`back/auth/internal/handler/http/auth_handler_test.go`, mocks `mockUserRepoH`):
  cubre mapeo a 201/409/400.
- Correr: `go test ./internal/service/ ./internal/handler/http/ -count=1` desde `back/auth`.

## 2. `POST /auth/login`

Contrato: `200 {access_token, refresh_token, expires_in}`, `401` credenciales inválidas.
Ruta: `main.go:86` → `AuthHandler.Login` (`auth_handler.go:65-86`).

### 2.1 Verificar credenciales con bcrypt
`auth_service.go:157-170`: `GetByEmail` (email normalizado lower+trim) → si `nil`,
mismo error que password mala (`invalid_credentials`, no revela existencia) →
`bcrypt.CompareHashAndPassword` (tiempo constante).

### 2.2 JWT de vida corta (lógica existente conectada)
`jwt_service.go:74-103` `GenerarAccessToken`: claims `{user_id, iss=apuntes-auth,
aud=apuntes-client, iat, exp}`, HS256 con `JWT_SECRET`. Duración desde config
(`AccessExpiresIn`, típico 900s). `Login` lo llama en `auth_service.go:175`
y devuelve `ExpiresIn = cfgAccessExp`.

### 2.3 Refresh token con hash + `expires_at`
`auth_service.go:180-193`: `repository.GenerateRawToken()` (aleatorio) + hash bcrypt,
`expires_at = now + cfgRefreshExp` (típico 7d), `refreshTokens.Create(userID, hash, expires_at)`
→ `identity.refresh_tokens (user_id, token_hash, expires_at, revoked=false)`
(`agentSql.md:29-38`). Se devuelve el **raw** una sola vez al cliente; en BD solo el hash.
Logs: `auth login ok email=...` / `fail ... code=invalid_credentials` (`auth_handler.go:77-85`).

### 2.4 Tests
`auth_service_test.go` (login válido emite JWT sin role, password incorrecta → 401,
usuario inexistente → 401 mismo mensaje) + `auth_handler_test.go` (200/401).
Correr igual que §1.5.

## 3. Docker microservicios + Postgres

`docker-compose.yml`: `postgres:16-alpine` (`5432:5432`, volumen `postgres_data`,
seed `docker/postgres/init.sql`, healthcheck `pg_isready`), `auth` (`8085:8080`,
`PORT=8080`, `host 8081` reservado al callback OAuth de Drive), `notes` (`8082`),
`social` (`8083`), `front` (`8086:80`), `logs` Dozzle (`8087`).
Env común: `DOCKER_DATABASE_URL=postgresql://postgres:...@postgres:5432/postgres?sslmode=disable`
+ `JWT`/`JWT_SECRET`/`DISCOVERY_URL` desde `.env` (ver `.env.example`).
Levantar: `./scripts/run-local.sh` (`build front` + `up -d`, front `localhost:8086`,
auth `localhost:8085/health`) o `./scripts/run-all.sh`. Solo front contra pillán:
`./scripts/run-deploy.sh` (usa `.env.prod` con `https://ti3-brojas.dev.censei.cl/api-*`).


## 5. 401 / 403 y middleware de access token

No hay 403 por rol en auth (ver §0). Todo lo de token es 401:
`invalid_token`, `token_expired`, `unauthorized` (`response.go:79-80`).

Middleware: `back/auth/internal/middleware/auth_middleware.go:39-83` `RequireAuth()`:
falta header / no `Bearer ` / token vacío → 401; `jwt.ValidarAccessToken` valida
firma HS256 + `exp` (`jwt_service.go:105-...`); expirado → `token_expired`, resto →
`invalid_token`. Inyecta **solo `user_id`** en `gin.Context` + `request.Context`
(`auth_middleware.go:77-79`,helpers `GetUserID`). Se aplica por ruta en `main.go:89-113`
(`/auth/google-drive/*`, `/profile/me`, `/internal` usa `RequireInternalKey` en vez de JWT).

Tests: `auth_middleware_test.go` (token válido inyecta `user_id`, expirado → 401
`token_expired`, malformado/firma mala → 401 `invalid_token`, sin header /
sin `Bearer` / vacío → 401) + `jwt_service_test.go` (generar/validar, expiración,
algoritmo). Correr: `go test ./internal/middleware/ ./internal/service/ -count=1`.

## 6. Frontend: `/login` y `/register` conectados

Un solo archivo con tabs: `front/lib/features/auth/login_screen.dart`
(`_submitRegister:156-206`, login en `:91`), servicios en
`front/lib/core/services/auth_service.dart` (`login:74`, `register:125`,
paths `/auth/login` y `/auth/register` en `:18-19`, `baseUrl` desde
`api_config.dart` → `--dart-define=API_BASE_URL` o default `http://localhost:8085`).
`LoginResult`/`RegisterResult` mapean `invalid_credentials` → `Credenciales inválidas`,
`email_taken` → `Ese correo ya está registrado`, etc. (`auth_service.dart:194-231`).
Sesión/refresh centralizados en `session_manager.dart` + `authed_client.dart`.
Probar local: `run-local.sh` + `flutter run -d linux` (opción 1 de `run-front.sh`);
contra pillán: opción 5/6 (inyectan `API_BASE_URL=https://ti3-brojas.dev.censei.cl/api-auth`
+ `ALLOW_INSECURE=true` por el Fake Certificate del Ingress).
