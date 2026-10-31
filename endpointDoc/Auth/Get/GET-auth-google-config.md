# GET /auth/google-config

## Req
- Método: `GET`, ruta: `/auth/google-config` (pública, sin auth; `back/auth/cmd/server/main.go:130`).
- Sin headers ni body. El `client_id` es público por diseño; el secret jamás sale del backend.

## Res
- `200`: `{"client_id":"e2e-client-id","redirect_uri":"http://localhost/cb"}` (valores de config del servidor; en tests `"e2e-client-id"`).

## Flujo
- `back/auth/internal/handler/http/drive_handler.go` (`GetGoogleConfig`, responde directo sin service/repository).

## Tests
- `back/auth/internal/handler/http/e2e_full_flow_test.go`: `TestE2E_OAuthPerfilInternasHealth` (200 con `"client_id"` en body).
