# POST /auth/forgot-password

## Req
- Método: `POST`, ruta: `/auth/forgot-password` (pública, sin auth; `back/auth/cmd/server/main.go:93`).
- Headers: `Content-Type: application/json`, `Cache-Control: no-store` en respuesta.
- Body: `{"email":"reset@test.invalid"}`.

## Res
- `200`: `{"message":"Si el correo está registrado, recibirás un código para recuperar tu contraseña. Revisa también la carpeta de spam."}` (mismo mensaje exista o no la cuenta; no filtra enumeración).
- `400`: `{"error":{"code":"bad_request","message":"Ingresa un correo válido."}}`.
- `503`: `{"error":{"code":"mail_unavailable","message":"..."}}` (SMTP no disponible; no cubierto por los tests nuevos).

## Flujo
- `back/auth/internal/handler/http/password_reset_handler.go` (`Forgot`) → `back/auth/internal/service/password_reset_service.go` (`Forgot`) → `back/auth/internal/repository/password_reset_repository.go` + `back/auth/internal/repository/user_repository.go` (+ mailer SMTP; moqueado en tests).

## Tests
- `back/auth/internal/handler/http/e2e_full_flow_test.go`: `TestE2E_ForgotReset` (forgot a cuenta existente y a desconocida: ambos 200 con body idéntico; código de 8 dígitos capturado del mailer mock).
