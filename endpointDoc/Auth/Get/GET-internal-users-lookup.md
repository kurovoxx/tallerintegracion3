# GET /internal/users/lookup

Permite consultar en lote (batch) la información pública de múltiples usuarios por sus IDs.

## 1. Req (Petición)
- **Método y Ruta:** `GET /internal/users/lookup` (Servicio-a-servicio; `back/auth/cmd/server/main.go:118`).
- **Headers:** `X-Internal-Key: <INTERNAL_API_KEY>`.
- **Query Params:** `ids=uuid1,uuid2,uuid3` (hasta 200 IDs).

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK`:** Lista de usuarios encontrados `[{"id":"uuid1",...},{"id":"uuid2",...}]`.

## 3. Invocación (Front / Otros Servicios)
- **Microservicio Social:**
  - Cliente: `back/social/internal/service/user_directory.go:70` (`LookupUsers`).
  - Uso: Evitar N llamadas individuales al listar los miembros de un grupo o asistentes de una reunión.

## 4. Flujo Técnico (Paso a Paso)
1. Valida `X-Internal-Key`.
2. Parsea la lista de UUIDs.
3. Ejecuta `SELECT u.id, u.email, p.display_name, p.photo_url FROM identity.users u JOIN identity.profiles p ON u.id = p.user_id WHERE u.id = ANY($1)`.
