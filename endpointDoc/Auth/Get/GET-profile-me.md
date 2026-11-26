# `GET /profile/me`


Obtiene el perfil extendido y datos personales del usuario autenticado.

## 1. Req (Petición)
- **Método y Ruta:** `GET /profile/me` (Protegida con JWT; `back/auth/cmd/server/main.go:107`).
- **Headers Requeridos:** `Authorization: Bearer <access_token>`.

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK`:**
  ```json
  {
    "public_id": "a8kp92mqwx7r",
    "display_name": "Juan Pérez",
    "photo_url": "https://...",
    "phone": "+56912345678",
    "institution": "Universidad Católica de Temuco",
    "description": "Estudiante de Ingeniería",
    "visibility": "private"
  }
  ```
- `public_id` (de `identity.users`) es el identificador que el usuario comparte para que otros abran su perfil público ([`GET /profile/public/:public_id`](GET-profile-public-public_id.md)); nunca es el UUID interno. `PATCH /profile/me` también lo devuelve y no permite modificarlo.
- **Errores:** `401 Unauthorized` (sesión inválida), `404 Not Found` (perfil inexistente).

## 3. Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/core/services/profile_service.dart:94` (`getProfile()`).
  - Pantalla: `front/lib/features/academic/profile_screen.dart:161` (`_loadProfileData`).

## 4. Flujo Técnico (Paso a Paso)
1. `ProfileHandler.GetProfile` (`internal/handler/http/profile_handler.go:46`) extrae `user_id` del JWT.
2. `ProfileService.GetProfile` (`internal/service/profile_service.go:28`) invoca al repositorio.
3. `ProfileRepository.GetByUserID`: `SELECT p.user_id, p.display_name, p.photo_url, p.phone, p.institution, p.description, p.visibility, p.updated_at, u.public_id FROM identity.profiles p JOIN identity.users u ON u.id = p.user_id WHERE p.user_id = $1`.

## 5. Dependencias y Datos Afectados
- **Tablas PostgreSQL:** `identity.profiles`, `identity.users` (`public_id`).

## 6. Tests Asociados
- **Unitarios:** `back/auth/internal/handler/http/profile_handler_test.go` (`TestGetProfile_Success`).
