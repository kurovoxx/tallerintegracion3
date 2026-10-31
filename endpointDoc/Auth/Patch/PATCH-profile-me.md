# `PATCH /profile/me`


## Req

- `PATCH /profile/me`, grupo protegido: `back/auth/cmd/server/main.go:145-152`. Requiere JWT Bearer.
- JSON parcial con display_name, photo_url, phone, institution, description y/o visibility. Campos omitidos no cambian. Body vacío se rechaza.
- user_id está prohibido incluso si coincide con el JWT. ProfileService valida display_name de 1-100 caracteres, photo_url hasta 500, phone hasta 30 e institution hasta 200; description se recorta sin límite. visibility acepta public/private.
- Base Flutter habitual `/api-auth` se elimina en el gateway; la ruta Gin es `/profile/me.`

## Res

- `200`: perfil actualizado con display_name, photo_url, phone, institution, description, visibility y email opcional.
- 400 por JSON/body vacío, user_id, visibilidad o campo inválido.
- 401 por JWT ausente/inválido; 404 si no existe perfil; 500 ante error interno.
- La fila del perfil se actualiza antes de responder. No se actualiza otro user_id elegido por cliente.

## Invocación (Front / Otros Servicios)

- `front/lib/features/academic/profile_screen.dart:224`, `_handleSave` reúne campos editados.
- `front/lib/core/services/profile_service.dart:113`, `patchProfile` envía PATCH autenticado, analiza el perfil devuelto y reemplaza current.
- No se encontró consumidor de producción de otro microservicio.

## Flujo

1. `main()` registra `PATCH /profile/me` bajo `RequireAuth` (`back/auth/cmd/server/main.go:152`).
2. `ProfileHandler.PatchProfile` toma el user ID autenticado, enlaza el body y rechaza cambios de `user_id` (`back/auth/internal/handler/http/profile_handler.go:108-148`).
3. `ProfileService.UpdateProfile` valida los campos (`back/auth/internal/service/profile_service.go:86-166`) y `ProfileRepository.Update` persiste el PATCH para ese user ID (`back/auth/internal/repository/profile_repository.go:57-139`).
4. `ProfileHandler.PatchProfile` devuelve el perfil actualizado (`back/auth/internal/handler/http/profile_handler.go:173-184`). En Flutter, `ProfileService.patchProfile` envía la modificación y reemplaza el perfil en memoria (`front/lib/core/services/profile_service.dart:113-137`), a partir de `ProfileScreen._handleSave` (`front/lib/features/academic/profile_screen.dart:224-260`).

## Tests

- `back/auth/internal/handler/http/profile_handler_test.go:180`, `actualizaci`ón parcial; :202, varios campos; :221, visibility inválida; :240, body vacío; :258, user_id prohibido; :405, entrada inválida no aplica cambios parciales.
- `back/auth/internal/service/profile_service_test.go:135` y :154 cubren actualización parcial y múltiple; :174 cubre visibility inválida.
