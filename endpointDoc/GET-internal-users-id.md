# GET /internal/users/:id

Resuelve un usuario por id para servicio-a-servicio — GetUser singular, complemento del batch `lookup` (Identity, tarea 3_5_6).

## Req
- Método: `GET`, ruta: `/internal/users/:id` (protegida con `X-Internal-Key`, NO con JWT; `back/auth/cmd/server/main.go:106`).
- Path: `id` (uuid del usuario).

## Res
- `200`: `{"user_id":"<uuid>","email":"a@x.com","display_name":"..."}` (nunca hashes ni tokens).
- `401`: sin key o key mala (desde `RequireInternalKey`).
- `404`: `{"error":{"code":"user_not_found","message":"Usuario no encontrado"}}`.

## Flujo
- `InternalUsersHandler.GetByID` (`back/auth/internal/handler/http/internal_users_handler.go`) → `UsersService.Lookup` con 1 id (`back/auth/internal/service/users_service.go:30-53`) → `UserRepository.GetPublicByIDs` (`identity.users` + perfil). Vacío → 404. Código `user_not_found` agregado en `back/auth/internal/utils/response.go` (mensaje + map a 404).

## Tests
- `back/auth/internal/handler/http/internal_auth_test.go`: `TestInternalUsers_GetByID` (200 con email/display_name, inexistente → 404 `user_not_found`), `TestInternal356_SinKey_401`.
- Consumidor: `back/social/internal/identity/client.go` (`GetUser` → `ErrUserNotFound` ante 404) + `client_test.go`.
