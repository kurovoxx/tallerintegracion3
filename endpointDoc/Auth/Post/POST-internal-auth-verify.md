# POST /internal/auth/verify

Verifica la validez de un JWT de usuario para servicios que no poseen la clave secreta o requieren verificación canónica centralizada.

## 1. Req (Petición)
- **Método y Ruta:** `POST /internal/auth/verify` (Servicio-a-servicio; `back/auth/cmd/server/main.go:120`).
- **Headers:** `X-Internal-Key: <INTERNAL_API_KEY>`, `Content-Type: application/json`.
- **Body:** `{"token": "<access_token>"}`.

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK`:** `{"valid":true,"user_id":"uuid","email":"..."}`.
- **`401 Unauthorized`:** `X-Internal-Key` inválida o JWT inválido/expirado (`{"valid":false}`).

## 3. Invocación (Front / Otros Servicios)
- **Microservicios Internos:** Usado como fallback de verificación de identidad o por servicios auxiliares.

## 4. Flujo Técnico (Paso a Paso)
1. Valida `X-Internal-Key`.
2. Parsea y valida claims del JWT (`jwt_service.ValidateAccessToken`).
3. Comprueba que el usuario exista y no tenga bloqueo por borrado.
