# GET /profile/me

Obtiene el perfil extendido y datos personales del usuario autenticado.

## 1. Req (Petición)
- **Método y Ruta:** `GET /profile/me` (Protegida con JWT; `back/auth/cmd/server/main.go:107`).
- **Headers Requeridos:** `Authorization: Bearer <access_token>`.

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK`:**
  ```json
  {
    "display_name": "Juan Pérez",
    "photo_url": "https://...",
    "phone": "+56912345678",
    "institution": "Universidad Católica de Temuco",
    "description": "Estudiante de Ingeniería",
    "visibility": "private"
  }
  ```
- **Errores:** `401 Unauthorized` (sesión inválida), `404 Not Found` (perfil inexistente).

## 3. Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/core/services/profile_service.dart:94` (`getProfile()`).
  - Pantalla: `front/lib/features/academic/profile_screen.dart:161` (`_loadProfileData`).

## 4. Flujo Técnico (Paso a Paso)
1. `ProfileHandler.GetProfile` (`internal/handler/http/profile_handler.go:46`) extrae `user_id` del JWT.
2. `ProfileService.GetProfile` (`internal/service/profile_service.go:28`) invoca al repositorio.
3. `ProfileRepository.GetByUserID`: `SELECT display_name, photo_url, phone, institution, description, visibility FROM identity.profiles WHERE user_id = $1`.

## 5. Dependencias y Datos Afectados
- **Tablas PostgreSQL:** `identity.profiles`.

## 6. Tests Asociados
- **Unitarios:** `back/auth/internal/handler/http/profile_handler_test.go` (`TestGetProfile_Success`).
