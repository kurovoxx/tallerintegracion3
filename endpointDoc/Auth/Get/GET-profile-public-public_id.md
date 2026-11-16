# `GET /profile/public/:public_id`

Devuelve la vista pública del perfil de otro usuario identificado por su
`public_id` (nunca por su UUID interno). Expone nombre, institución y foto
(solo si es una imagen subida desde la app).

## 1. Req (Petición)
- **Método y Ruta:** `GET /profile/public/:public_id` (registrada en
  `back/auth/cmd/server/main.go`, fuera del grupo `protected`, con
  `middleware.PrivateNoStore()` **antes** de `authMw.RequireAuth()`).
- **Headers Requeridos:** `Authorization: Bearer <access_token>`.
- **Path:** `public_id`, 12 caracteres Crockford Base32 en minúsculas
  (`^[0-9a-hj-km-np-tv-z]{12}$`). Se recortan espacios y se pasa a minúsculas
  antes de validar; `l`/`o` no se corrigen a `1`/`0`.
- El solicitante sale exclusivamente del JWT.

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK`:**
  ```json
  {
    "public_id": "a8kp92mqwx7r",
    "display_name": "Ana Pérez",
    "photo_url": "data:image/png;base64,iVBORw0KGgo...",
    "institution": "Universidad Católica de Temuco",
    "visibility": "public"
  }
  ```
  - `institution` es `null` si no está definida.
  - `photo_url` solo contiene una data URL de imagen en base64 (la foto
    recortada y subida desde la app). Si no hay foto o es una URL remota es
    `null`: la app de quien consulta nunca contacta a un servidor ajeno.
  - **Nunca** incluye `user_id`, `email`, `phone` ni `description` (la
    descripción del perfil es privada).
- **`404 Not Found` (`profile_not_found`):** `public_id` con formato inválido,
  inexistente, usuario sin fila de perfil, o perfil `private` consultado por
  otra persona. Las cuatro causas son indistinguibles.
- **`200` para el dueño:** un perfil `private` sí se devuelve a su propio
  dueño.
- **`401` / `423` / `503`:** emitidos por `RequireAuth`.
- Todas las respuestas (incluidas las del middleware) llevan
  `Cache-Control: private, no-store`.

## 3. Invocación (Front / Otros Servicios)
- **Frontend Flutter:** `ProfileService.getPublicProfile`
  (`front/lib/core/services/profile_service.dart`), usada por
  `PublicProfileScreen` (`front/lib/features/academic/public_profile_screen.dart`).
  El cliente valida el ID con `normalizePublicId` y no llama al backend si es
  inválido. «Buscar perfil» (`ProfileScreen`) interpreta antes el texto pegado
  con `parsePublicIdInput` (ID o `sigma-academy://perfil/<id>`). El avatar
  pinta la foto con `MemoryImage` y solo acepta data URLs de imagen.

## 4. Flujo Técnico (Paso a Paso)
1. `ProfileHandler.GetPublicProfileByPublicID`
   (`internal/handler/http/profile_handler.go`) fija la cabecera de caché y
   toma el solicitante del JWT.
2. `ProfileService.GetPublicProfileByPublicID`
   (`internal/service/profile_service.go`) normaliza y valida el `public_id`
   y aplica la regla de visibilidad (privado ajeno → 404; cualquier valor
   distinto de `public` se trata como privado). `publicPhotoURL` deja la foto
   solo si es una data URL de imagen.
3. `ProfileRepository.GetPublicByPublicID`
   (`internal/repository/profile_repository.go`):
   `SELECT p.user_id, p.display_name, p.photo_url, p.institution,
   p.visibility, u.public_id FROM identity.users u JOIN identity.profiles p
   ON p.user_id = u.id WHERE u.public_id = $1`. No selecciona correo,
   teléfono ni descripción.

## 5. Dependencias y Datos Afectados
- **Tablas PostgreSQL:** `identity.users` (`public_id`, migración
  `back/auth/db/migrations/006_public_profile_id.sql`), `identity.profiles`.
  Solo lectura.

## 6. Tests Asociados
- `back/auth/internal/handler/http/profile_handler_test.go`:
  `TestProfileHandler_GetPublicProfileByPublicID_*` (DTO sin datos sensibles,
  privado ajeno 404, formatos inválidos, 401) y
  `..._CacheControlCubreCadenaCompleta` (200/404/401/423/503).
- `back/auth/internal/service/profile_service_test.go`:
  `TestProfileService_GetPublicProfileByPublicID_*`.
- `front/test/profile_service_test.dart`, `front/test/public_profile_screen_test.dart`.
