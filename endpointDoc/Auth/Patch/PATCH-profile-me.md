# PATCH /profile/me

Actualiza parcialmente los campos del perfil personal del usuario autenticado.

## 1. Req (Petición)
- **Método y Ruta:** `PATCH /profile/me` (Protegida con JWT; `back/auth/cmd/server/main.go:108`).
- **Headers Requeridos:** `Authorization: Bearer <access_token>`, `Content-Type: application/json`.
- **Cuerpo (Body - campos opcionales):**
  ```json
  {
    "display_name": "Juan P.",
    "phone": "+56987654321",
    "description": "Nueva descripción de perfil",
    "visibility": "public"
  }
  ```
- **Validaciones:** `display_name` 1-100 caracteres si está presente; `visibility` debe ser `public` o `private`.

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK`:** Perfil completo actualizado.
- **`400 Bad Request`:** `invalid_body` o visibilidad no permitida.
- **`401 Unauthorized`:** Token ausente o expirado.

## 3. Invocación (Front / Otros Servicios)
- **Frontend Flutter:**
  - Cliente API: `front/lib/core/services/profile_service.dart:111` (`updateProfile(patch)`).
  - Pantalla: `front/lib/features/academic/profile_screen.dart:222` (`_saveProfileChanges`).

## 4. Flujo Técnico (Paso a Paso)
1. `ProfileHandler.PatchProfile` (`profile_handler.go:80`) valida payload JSON.
2. `ProfileService.UpdateProfile` sanitiza strings y valida rangos.
3. `ProfileRepository.Update`: ejecuta `UPDATE identity.profiles SET ... WHERE user_id = $1 RETURNING ...`.

## 5. Dependencias y Datos Afectados
- **Tablas PostgreSQL:** `identity.profiles`.

## 6. Tests Asociados
- **Unitarios:** `back/auth/internal/handler/http/profile_handler_test.go` (`TestPatchProfile_Success`).
