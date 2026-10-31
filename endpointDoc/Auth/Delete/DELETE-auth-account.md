# `DELETE /auth/account`

## Req

- Ruta protegida por `AuthMiddleware.RequireIdentity`: `back/auth/cmd/server/main.go:123`.
- Requiere `Authorization: Bearer <access JWT>` válido. `user_id` se obtiene únicamente del JWT. No tiene body ni query.
- `RequireIdentity` valida la identidad sin exigir que la cuenta siga activa (`back/auth/internal/middleware/auth_middleware.go:54-57`); permite consultar/reanudar el borrado mientras el JWT siga vigente.
- En el gateway web el prefijo `/api-auth` se retira antes de llegar a Auth; no forma parte de la ruta Gin (`front/nginx.conf:23-27`).

## Res

- `200 OK`: `{"status":"complete"}` cuando Social y Notes informan `COMPLETE` y Identity elimina la cuenta local.
- `202 Accepted`: `{"status":"pending"}` si queda trabajo pendiente, un RPC falla/expira o devuelve un estado no explícito. El registro durable permite reintentar; la cuenta local y OAuth aún se conservan.
- `404 Not Found`: `{"error":"account_not_found"}` si no existe la cuenta ni un borrado durable previo.
- `503 Service Unavailable`: `{"error":"account_deletion_unavailable"}` ante fallo del bloqueo o del estado local. Los errores de limpieza remota se convierten en `202`, no autorizan el borrado.
- `401 Unauthorized`: `AuthMiddleware.require` rechaza el encabezado/JWT antes del handler (`back/auth/internal/middleware/auth_middleware.go:60-94`). Si falta `Authorization`, el cuerpo es `{"error":{"code":"unauthorized","message":"Authorization header requerido"}}`; si no usa el prefijo `Bearer `, `{"error":{"code":"unauthorized","message":"Formato de Authorization debe ser Bearer {token}"}}`; si el bearer está vacío, `{"error":{"code":"unauthorized","message":"Token vacío"}}`. JWT inválido y vencido devuelven respectivamente `{"error":{"code":"invalid_token","message":"Token inválido"}}` y `{"error":{"code":"token_expired","message":"Token expirado"}}`, con mensajes de `utils.MessageForCode` (`back/auth/internal/utils/response.go:45-75`).
- La operación elimina Identity y limpia membresías/accesos compartidos. Conserva los apuntes propios y archivos personales de Drive. El tombstone `complete` queda para rechazar JWT antiguos.

## Invocación (Front / Otros Servicios)

- Tras la confirmación de la persona, `AccountDeletionDialog._submit` llama `AccountDeletionService.requestDeletion` (`front/lib/features/auth/account_deletion_dialog.dart:158-167`).
- `AccountDeletionService._requestDeletion` envía el DELETE con el JWT actual (`front/lib/core/services/account_deletion_service.dart:175-245`). Si el resultado es ambiguo, consulta `AccountDeletionService.checkStatus` (`front/lib/core/services/account_deletion_service.dart:271-329`) en vez de repetir el DELETE.
- `AccountDeletionDialog._apply` muestra el estado pendiente y consulta de nuevo, o finaliza el flujo al recibir `complete` (`front/lib/features/auth/account_deletion_dialog.dart:178-193`).
- Identity coordina Notes y Social mediante RPC gRPC autenticados con `x-internal-key`; no usa sus endpoints HTTP para esta orquestación.

## Flujo

1. `main()` registra la ruta protegida y su middleware (`back/auth/cmd/server/main.go:123`). `AuthMiddleware.RequireIdentity` valida el JWT sin comprobar que la cuenta siga activa (`back/auth/internal/middleware/auth_middleware.go:54-60`).
2. `AccountDeletionHandler.Delete` toma el `user_id` del contexto JWT y llama al coordinador (`back/auth/internal/handler/http/account_deletion_handler.go:24-40`).
3. `AccountDeletionCoordinator.DeleteAccount` serializa por usuario, guarda `pending`, inicia los gates de Notes y Social, ejecuta ambas limpiezas y acepta solo `COMPLETE` explícito (`back/auth/internal/service/account_deletion.go:64-104`). `AccountDeletionRepository.WithExclusiveUserLock` serializa solicitudes (`back/auth/internal/repository/account_deletion_repository.go:84-99`) y `AccountDeletionRepository.Begin` crea el tombstone durable (`back/auth/internal/repository/account_deletion_repository.go:126-144`).
4. `GRPCAccountCleanup.BeginNotes`, `BeginSocial`, `CleanupSocial` y `CleanupNotes` realizan los RPC (`back/auth/internal/service/account_deletion_grpc.go:52-120`). `withCleanupKey` adjunta `x-internal-key` (`back/auth/internal/service/account_deletion_grpc.go:127-129`) y `cleanupRPCContext` aplica el límite de 30 segundos (`back/auth/internal/service/account_deletion_grpc.go:123-125`). Un error, timeout o estado desconocido no equivale a `COMPLETE`.
5. Tras ambas limpiezas, `AccountDeletionRepository.CompleteAndDelete` marca el tombstone `complete` y elimina `identity.users` dentro de una transacción (`back/auth/internal/repository/account_deletion_repository.go:174-189`). Las FK eliminan refresh tokens y OAuth por cascada (`docker/postgres/init.sql:41-54,76-84`); el tombstone no depende de la fila de usuario.
6. `AccountDeletionHandler.Delete` traduce el resultado al status HTTP (`back/auth/internal/handler/http/account_deletion_handler.go:30-40`). En Flutter, `AccountDeletionService._requestDeletion` interpreta `200/202` o consulta el estado, y `AccountDeletionDialog._apply` actualiza la vista (`front/lib/core/services/account_deletion_service.dart:205-245`; `front/lib/features/auth/account_deletion_dialog.dart:178-193`).

## Tests

- `back/auth/internal/handler/http/account_deletion_contract_test.go:146`, `TestAccountDeletionContract_CompleteIsIdempotentAndBlocksNormalRoutes`; `back/auth/internal/handler/http/account_deletion_contract_test.go:181`, `TestAccountDeletionContract_PendingStagesReturn202`; `back/auth/internal/handler/http/account_deletion_contract_test.go:218`, `TestAccountDeletionContract_ErrorsAndAuth`.
- `back/auth/internal/service/account_deletion_integration_test.go:48`, `TestAuthIntegration_AccountDeletionPreservesRowsUntilCompleteAndCascades` (PostgreSQL de integración cuando la suite está configurada).
