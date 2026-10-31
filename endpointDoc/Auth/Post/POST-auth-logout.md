# `POST /auth/logout`


## Req

- `POST /auth/logout`, ruta pública sin JWT: `back/auth/cmd/server/main.go:120`.
- `Content-Type`: application/json.
- Body requerido: {"refresh_token":"<refresh>"}.
- Ruta de servicio; con gateway Flutter se antepone la base configurada, normalmente `/api-auth`, que nginx retira (`front/nginx.conf:23-27`).

## Res

- 204 sin body si revoca el refresh. También devuelve 204 si no existe, ya estaba revocado o está vencido; no revela si el token existía.
- 400 si el JSON es inválido o refresh_token está vacío.
- 500 ante un fallo interno.
- No emite access ni refresh tokens. La revocación se registra en Identity.

## Invocación (Front / Otros Servicios)

- `front/lib/features/academic/profile_screen.dart:472-478`, `_handleLogout` guarda el refresh actual, limpia la sesión y llama `AuthService.logout`. `front/lib/core/services/auth_service.dart:112` construye el POST; la revocación no bloquea la navegación al login.
- No se encontró consumidor de producción de otro microservicio.

## Flujo

1. `main()` registra `POST /auth/logout` como ruta pública (`back/auth/cmd/server/main.go:120`).
2. `AuthHandler.Logout` valida `refresh_token` y llama al servicio (`back/auth/internal/handler/http/auth_handler.go:130-164`).
3. `AuthService.Logout` busca el token y solicita su revocación (`back/auth/internal/service/auth_service.go:294-316`); `RefreshTokenRepository.Revoke` persiste `revoked=true` (`back/auth/internal/repository/refresh_token_repository.go:160-165`).
4. `AuthHandler.Logout` devuelve `204` sin contenido (`back/auth/internal/handler/http/auth_handler.go:130-164`). Flutter llama `AuthService.logout` (`front/lib/core/services/auth_service.dart:112-138`) desde `_handleLogout` (`front/lib/features/academic/profile_screen.dart:472-478`) y limpia su sesión local aunque el endpoint sea idempotente.

## Tests

- `back/auth/internal/handler/http/auth_handler_test.go:569`, `TestAuthHandler_Logout_Activo_204`; :620, TestAuthHandler_Logout_YaRevocado_204; :664, token inexistente; :688, token vencido.
- `back/auth/internal/service/auth_service_test.go:627`, `TestLogout_Activo_204_Revocado`; :643, TestLogout_YaRevocado_204; :660, token inexistente; :675, expirado.
