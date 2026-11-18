# `GET /profile/public/:public_id`

Devuelve el perfil de otro usuario identificado por su `public_id` (código
público; nunca por su UUID interno). Perfil `public`: nombre, foto,
descripción, correo, teléfono e institución. Perfil `private`: solo nombre y
foto.

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
- **`200 OK` (perfil `public`, o cualquier perfil consultado por su dueño):**
  ```json
  {
    "public_id": "a8kp92mqwx7r",
    "display_name": "Ana Pérez",
    "photo_url": "data:image/png;base64,iVBORw0KGgo...",
    "institution": "Universidad Católica de Temuco",
    "description": "Apuntes de cálculo y álgebra",
    "email": "ana@ejemplo.cl",
    "phone": "+56912345678",
    "visibility": "public"
  }
  ```
  - `institution`, `description`, `email` y `phone` se **omiten** si no
    están definidos (o son solo espacios).
  - `email` es el correo de la cuenta (`identity.users.email`).
- **`200 OK` (perfil `private` consultado por otra persona):**
  ```json
  {
    "public_id": "a8kp92mqwx7r",
    "display_name": "Ana Pérez",
    "photo_url": "data:image/png;base64,iVBORw0KGgo...",
    "visibility": "private"
  }
  ```
  `ProfileService` construye una vista nueva solo con nombre y foto
  (`privateProfileView`), de modo que descripción, correo, teléfono e
  institución nunca llegan al handler ni al JSON.
- `photo_url` solo contiene una data URL de imagen en base64 (la foto
  recortada y subida desde la app). Si no hay foto o es una URL remota es
  `null`: la app de quien consulta nunca contacta a un servidor ajeno.
- **Nunca** incluye `user_id`.
- **`404 Not Found` (`profile_not_found`):** `public_id` con formato
  inválido, inexistente o usuario sin fila de perfil (indistinguibles).
- **`401` / `423` / `503`:** emitidos por `RequireAuth`.
- Todas las respuestas (incluidas las del middleware) llevan
  `Cache-Control: private, no-store`: ninguna caché intermedia reutiliza la
  respuesta completa de un perfil.

> Cambio de contrato (`feat/mejoras-perfiles-y-apuntes`): antes un perfil
> privado ajeno respondía `404` y correo, teléfono y descripción nunca se
> exponían. El requerimiento actual pide mostrarlos cuando el dueño eligió
> visibilidad pública.

## 3. Invocación (Front / Otros Servicios)
- **Frontend Flutter:** `ProfileService.getPublicProfile`
  (`front/lib/core/services/profile_service.dart`), usada por
  `PublicProfileScreen` (`front/lib/features/academic/public_profile_screen.dart`).
  `PublicProfile.fromJson` (`front/lib/core/models/profile_models.dart`)
  descarta además los campos opcionales de un perfil `private` (defensa en
  profundidad). «Buscar perfil» (`ProfileScreen`) solo acepta el código
  (`parsePublicIdInput`); «Compartir perfil» copia solo el código.

## 4. Flujo Técnico (Paso a Paso)
1. `ProfileHandler.GetPublicProfileByPublicID`
   (`internal/handler/http/profile_handler.go`) fija la cabecera de caché y
   toma el solicitante del JWT.
2. `ProfileService.GetPublicProfileByPublicID`
   (`internal/service/profile_service.go`) normaliza y valida el `public_id`,
   filtra la foto (`publicPhotoURL`) y aplica la visibilidad: cualquier
   valor distinto de `public` se trata como `private`; privado y ajeno →
   `privateProfileView` (solo nombre y foto).
3. `ProfileRepository.GetPublicByPublicID`
   (`internal/repository/profile_repository.go`):
   `SELECT p.user_id, p.display_name, p.photo_url, p.institution,
   p.visibility, u.public_id, p.description, p.phone,
   NULLIF(btrim(u.email), '') FROM identity.users u JOIN identity.profiles p
   ON p.user_id = u.id WHERE u.public_id = $1`.

## 5. Dependencias y Datos Afectados
- **Tablas PostgreSQL:** `identity.users` (`public_id`, migración
  `back/auth/db/migrations/006_public_profile_id.sql`), `identity.profiles`.
  Solo lectura.

## 6. Tests Asociados
- `back/auth/internal/handler/http/profile_handler_test.go`:
  `TestProfileHandler_GetPublicProfileByPublicID_*` (perfil público completo,
  privado ajeno solo nombre y foto, privado propio completo, foto, formatos
  inválidos) y
  `..._CacheControlCubreCadenaCompleta` (200/404/401/423/503).
- `back/auth/internal/service/profile_service_test.go`:
  `TestProfileService_GetPublicProfileByPublicID_*`.
- `front/test/profile_service_test.dart`, `front/test/public_profile_screen_test.dart`.
