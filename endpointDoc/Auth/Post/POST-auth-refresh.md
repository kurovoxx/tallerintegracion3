# `POST /auth/refresh`

## Req

- Ruta pública, sin JWT, registrada en `back/auth/cmd/server/main.go:119`.
- Requiere `Content-Type: application/json`. El refresh token del body es la credencial.
- Body: `{"refresh_token":"<refresh vigente>"}`; `refresh_token` es obligatorio.
- Flutter usa la ruta relativa a `API_BASE_URL`. El gateway agrega y retira `/api-auth`; ese prefijo no llega a Gin (`front/nginx.conf:23-27`).

## Res

- `200 OK`: `{"access_token":"<jwt sintético>","refresh_token":"<refresh rotado sintético>","expires_in":900}`. `expires_in` es el TTL configurado para el access token, en segundos; `900` es ilustrativo.
- `400 Bad Request`: JSON ausente/malformado o campo `refresh_token` vacío/ausente (la validación está en `refreshRequest`, `back/auth/internal/handler/http/auth_handler.go:50-52`). Body: `{"error":{"code":"bad_request","message":"Request inválido: refresh_token requerido"}}`. Una cadena que solo contiene espacios supera el binding, se recorta en `AuthService.Refresh` y responde `401 invalid_token`.
- `401 Unauthorized`: refresh inexistente, revocado, reutilizado o perteneciente a una cuenta en borrado responde `{"error":{"code":"invalid_token","message":"Token inválido"}}`. Un refresh con huella indexada que se encuentra vencido responde `{"error":{"code":"token_expired","message":"Token expirado"}}`. Los candidatos legacy vencidos se excluyen de la búsqueda y, por ello, responden `invalid_token` si no hay coincidencia vigente. `AuthHandler.Refresh` fuerza `401` para estos códigos, no `403` (`back/auth/internal/handler/http/auth_handler.go:103-113`).
- `500 Internal Server Error`: los errores no tipados de base de datos, generación de tokens o rotación devuelven `{"error":{"code":"internal_error","message":"Error interno"}}`. Los errores internos tipados conservan el mensaje del servicio. Ejemplos: `{"error":{"code":"internal_error","message":"jwt service no configurado"}}` o `{"error":{"code":"internal_error","message":"no se pudo comprobar el estado de la cuenta"}}`.
- La transacción revoca el token anterior e inserta el nuevo antes de devolver `200`. Los tokens legacy se buscan solo entre candidatos vigentes/no revocados, pero la comparación bcrypt todavía puede ser lenta mientras queden sesiones legacy válidas.

## Invocación (Front / Otros Servicios)

- `AuthedHttp.run` reintenta una petición protegida después de un `401` de sesión y coordina el refresh concurrente (`front/lib/core/services/authed_client.dart:77-104,109-130`).
- `AuthedHttp._doRefresh` llama `AuthService.refresh` (`front/lib/core/services/authed_client.dart:38-42`). Este método envía el POST, analiza la respuesta y tiene timeout de 10 segundos (`front/lib/core/services/auth_service.dart:67-108`). No se encontró invocación de producción desde otro microservicio.
- `AuthedHttp._performRefresh` descarta respuestas de una sesión antigua y entrega la pareja a `SessionManager.updateTokens` (`front/lib/core/services/authed_client.dart:134-189`).
- `SessionManager.updateTokens` aplica primero los tokens en memoria; si `rememberMe` está activo intenta guardar el par y las claves individuales, verifica los bool devueltos y reintenta como máximo dos veces (`front/lib/core/services/session_manager.dart:95-175`). Distingue `rotationAppliedAndPersisted`, `rotationAppliedInMemoryOnly` y `rotationAppliedInMemoryPersistenceFailed`.

## Flujo

1. `main()` registra el endpoint público (`back/auth/cmd/server/main.go:119`).
2. `AuthHandler.Refresh` enlaza y valida el JSON, llama al servicio y escribe el envelope de error o `refreshResponse` (`back/auth/internal/handler/http/auth_handler.go:96-124`).
3. `AuthService.Refresh` recorta el token, localiza la fila, verifica la cuenta y el vencimiento, firma un access token y prepara el nuevo refresh (`back/auth/internal/service/auth_service.go:228-289`).
4. `RefreshTokenRepository.FindByRawToken` calcula SHA-256 y consulta `idx_refresh_tokens_fingerprint` (`back/auth/internal/repository/refresh_token_repository.go:39-42,104-122`; inicialización del índice: `docker/postgres/init.sql:52-54`). Si no hay huella, `FindByRawToken` materializa solo candidatos legacy vigentes y no revocados, libera las filas y luego compara bcrypt (`back/auth/internal/repository/refresh_token_repository.go:123-156`). Ese fallback todavía puede tardar mientras haya sesiones legacy válidas.
5. `RefreshTokenRepository.Rotate` bloquea usuario y refresh anterior, vuelve a comprobar revocación y vencimiento, revoca el anterior, inserta el nuevo y confirma todo en una transacción (`back/auth/internal/repository/refresh_token_repository.go:170-212`).
6. `AuthService.Refresh` devuelve access, refresh rotado y TTL (`back/auth/internal/service/auth_service.go:285-289`); `AuthHandler.Refresh` los escribe en JSON (`back/auth/internal/handler/http/auth_handler.go:119-124`).
7. En Flutter, `AuthService.refresh` analiza el JSON (`front/lib/core/services/auth_service.dart:67-108`); `AuthedHttp._performRefresh` aplica la respuesta si sigue siendo la misma sesión (`front/lib/core/services/authed_client.dart:134-189`); `SessionManager.updateTokens` informa si la pareja quedó en memoria o también persistida (`front/lib/core/services/session_manager.dart:95-175`). Si falla el guardado, no se restaura el refresh anterior que el servidor pudo revocar.

`AuthedHttp.restorePersistedSession` intenta renovar si el access venció, pero su bool significa que quedan credenciales en memoria, no que el refresh haya sido validado (`front/lib/core/services/authed_client.dart:49-53`). En el arranque, `main()` selecciona `MainShell` si ese bool es verdadero (`front/lib/main.dart:24-27`). Por eso, un timeout transitorio puede abrir la app con renovación pendiente sin probar que las llamadas protegidas funcionen. Si falla el guardado de la pareja rotada, el proceso actual conserva los tokens nuevos en memoria, pero un reinicio puede leer el refresh anterior ya invalidado.

## Tests

- `back/auth/internal/handler/http/auth_handler_test.go:433`, `TestAuthHandler_Refresh_Valido_200` (handler con dependencias simuladas).
- `back/auth/internal/handler/http/auth_extra_coverage_test.go:126`, `TestCoverage_Refresh_JSONMalformado_400`; `back/auth/internal/handler/http/auth_extra_coverage_test.go:133`, `TestCoverage_Refresh_Expirado_401`; `back/auth/internal/handler/http/auth_extra_coverage_test.go:153`, `TestCoverage_Refresh_Rotado_Reutilizado_401`.
- `back/auth/internal/service/auth_service_test.go:474`, `TestRefresh_Valido_Rota`; `back/auth/internal/service/auth_service_test.go:521`, `TestRefresh_TokenInexistente_401`; `back/auth/internal/service/auth_service_test.go:534`, `TestRefresh_TokenRevocado_401`; `back/auth/internal/service/auth_service_test.go:554`, `TestRefresh_TokenExpirado_401`; `back/auth/internal/service/auth_service_test.go:588`, `TestRefresh_ReutilizacionTokenViejo_Falla`.
- `back/auth/internal/service/auth_integration_test.go:150`, `TestAuthIntegration_RegisterLoginRefreshLogout` (PostgreSQL real cuando se configura la suite).
