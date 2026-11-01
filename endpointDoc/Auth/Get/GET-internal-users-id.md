# GET /internal/users/:id

Retorna información pública de un usuario por su ID para uso interno de otros microservicios.

## 1. Req (Petición)
- **Método y Ruta:** `GET /internal/users/:id` (Servicio-a-servicio; `back/auth/cmd/server/main.go:119`).
- **Headers:** `X-Internal-Key: <INTERNAL_API_KEY>`.
- **Path:** `:id` (UUID del usuario).

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK`:** `{"id":"uuid","email":"usuario@estudiantes.uct.cl","display_name":"Nombre"}`.
- **`401 Unauthorized`:** Falta clave interna.
- **`404 Not Found`:** Usuario inexistente.

## 3. Invocación (Front / Otros Servicios)
- **Microservicio Social:**
  - Cliente: `back/social/internal/service/user_directory.go:29` (`HTTPUserDirectory.GetUser`).
  - Uso: Resolver nombres y correos de miembros de grupos.

## 4. Flujo Técnico (Paso a Paso)
1. Valida `X-Internal-Key`.
2. Consulta `identity.users` e `identity.profiles` unificando datos públicos.
3. Responde JSON sin hashes ni contraseñas.
