# `GET /profile/me`


## Req

- `GET /profile/me`, dentro del grupo protegido de Auth: `back/auth/cmd/server/main.go:145-151`.
- Requiere `Authorization`: Bearer <access JWT> válido. No tiene body, path ni query.
- La identidad se obtiene del JWT; el cliente no puede seleccionar otro usuario.
- Con el frontend web el base URL habitual agrega `/api-auth`; nginx retira ese prefijo antes de Auth (`front/nginx.conf:23-27`).

## Res

- `200`: objeto con display_name, photo_url, phone, institution, description, visibility y email opcional. Los campos nullable se serializan como null; email se omite si no se resolvió.
- 401 sin identidad/token válido; 404 si no existe el perfil; 500 ante error interno.
- Lectura solamente: no modifica perfil.

## Invocación (Front / Otros Servicios)

- `front/lib/features/academic/profile_screen.dart:163`, `_loadProfile` carga la vista de perfil.
- `front/lib/core/services/profile_service.dart:95`, `getProfile` envía GET con el JWT, analiza el JSON y actualiza el notifier `ProfileService.current`.
- No se encontró invocación de producción desde otro microservicio.

## Flujo

1. `main()` registra `GET /profile/me` con `AuthMiddleware.RequireAuth` (`back/auth/cmd/server/main.go:151`).
2. `ProfileHandler.GetProfile` toma `user_id` del contexto autenticado y llama al servicio (`back/auth/internal/handler/http/profile_handler.go:67-104`).
3. `ProfileService.GetProfile` solicita el perfil y completa el correo (`back/auth/internal/service/profile_service.go:55-68`); `ProfileRepository.GetByUserID` lee `identity.profiles` (`back/auth/internal/repository/profile_repository.go:25-39`), y `ProfileService.fillEmail` usa `UserRepository.GetEmailByID` (`back/auth/internal/service/profile_service.go:42-51`; `back/auth/internal/repository/user_repository.go:169-182`).
4. `ProfileHandler.GetProfile` serializa `profileResponse` y devuelve `200` (`back/auth/internal/handler/http/profile_handler.go:96-104`). Flutter llama `ProfileService.getProfile` (`front/lib/core/services/profile_service.dart:95`) desde `ProfileScreen._loadProfile` (`front/lib/features/academic/profile_screen.dart:163-178`) para rellenar la cuenta activa.

## Tests

- `back/auth/internal/handler/http/profile_handler_test.go:104`, `TestProfileHandler_GetProfile_200`; :132, perfil inexistente; :149, falta de autenticación.
- `back/auth/internal/service/profile_service_test.go:87`, `TestProfileService_GetProfile_Existente`; :109, inexistente.
