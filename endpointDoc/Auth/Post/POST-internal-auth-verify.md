# POST /internal/auth/verify

Verifica un JWT de usuario para servicio-a-servicio (Identity, tarea 3_5_6).

## Req
- Método: `POST`, ruta: `/internal/auth/verify` (protegida con `X-Internal-Key`, NO con JWT; `back/auth/cmd/server/main.go:107`).
- Headers: `Content-Type: application/json`, `X-Internal-Key: <secreto>`.
- Body: `{"token":"<access_token>"}`.

## Res
- `200`: `{"valid":true,"user_id":"<uuid>"}`.
- `400`: `{"error":{"code":"bad_request","message":"token requerido"}}` (body sin token).
- `401`: `{"error":{"code":"invalid_token|token_expired","message":"..."}}` (token malo o expirado; también sin key o key mala, desde `RequireInternalKey`).

## Flujo
- `InternalAuthHandler.Verify` (`back/auth/internal/handler/http/internal_auth_handler.go`) → `JWTService.ValidarAccessToken` (firma HS256 + `iss`/`aud` + `exp`, `back/auth/internal/service/jwt_service.go:105-133`). Sin DB: la verificación es solo criptográfica. Expirado se distingue por `jwt.ErrTokenExpired` o "expired" en el error (igual que el middleware HTTP).

## Tests
- `back/auth/internal/handler/http/internal_auth_test.go`: `TestInternalVerify_OK` (JWT real firmado en el test), `TestInternalVerify_Rechazos` (malo → 401, vacío → 400, expirado → 401 `token_expired`), `TestInternal356_SinKey_401`.
- Consumidor: `back/social/internal/identity/client.go` (`VerifyToken`) + `client_test.go` (fail closed ante 401).
