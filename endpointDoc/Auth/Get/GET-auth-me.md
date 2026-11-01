# GET /auth/me

Devuelve los datos de identidad básicos del usuario autenticado (ID y correo) a partir del JWT.

## 1. Req (Petición)
- **Método y Ruta:** `GET /auth/me` (Protegida con JWT; `back/auth/cmd/server/main.go:104`).
- **Headers Requeridos:** `Authorization: Bearer <access_token>`.

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK`:**
  ```json
  {
    "id": "e4a2d3c1-1111-4444-8888-abcdef123456",
    "email": "usuario@estudiantes.uct.cl"
  }
  ```
- **Errores Posibles:**
  - `401 Unauthorized`: token ausente, expirado o con firma inválida.
  - `404 Not Found`: el usuario del token ya no existe en la base de datos.

## 3. Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/core/services/auth_service.dart` (método de verificación de sesión activa).

## 4. Flujo Técnico (Paso a Paso)
1. **Middleware (`internal/middleware/auth_middleware.go`):** Valida el JWT y deposita el `user_id` en el contexto.
2. **Handler (`cmd/server/main.go:104`):**
   - Lee `user_id` del contexto.
   - Llama a `userRepo.GetByID(ctx, userID)`.
   - Responde con ID y Email.

## 5. Dependencias y Datos Afectados
- **Tablas PostgreSQL:** `identity.users` (lectura).

## 6. Tests Asociados
- **Unitarios:** `back/auth/internal/handler/http/auth_handler_test.go` (`TestAuthMe_Success`).
