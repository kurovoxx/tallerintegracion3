# `GET /auth/account-deletion`


## Req

- `GET /auth/account-deletion`, protegido por RequireIdentity: `back/auth/cmd/server/main.go:124`.
- Requiere JWT Bearer de la cuenta en borrado; sin body ni query. La identidad solo viene del JWT.
- No requiere persistir un JWT de larga duración en el servidor: el cliente consulta con su JWT vigente.

## Res

- `200`: {"status":"pending|complete","updated_at":"<RFC3339>"} si existe operación durable.
- `404`: {"status":"not_started"} si aún no comenzó; 401 si no hay identidad; 503 si no se pudo leer estado.
- Esta ruta informa estado. No ejecuta la limpieza por sí sola; `DELETE /auth/account` vuelve a invocar la coordinación/reintento.

## Invocación (Front / Otros Servicios)

- `AccountDeletionService.checkStatus` consulta el estado tras un DELETE ambiguo o pendiente (`front/lib/core/services/account_deletion_service.dart:271-329`); `_resolveWithStatus` también la invoca tras respuestas ambiguas (`front/lib/core/services/account_deletion_service.dart:243-268`).
- `AccountDeletionDialog._checkStatus` solicita la consulta desde el diálogo (`front/lib/features/auth/account_deletion_dialog.dart:170-175`) y `_apply` presenta el estado resultante (`front/lib/features/auth/account_deletion_dialog.dart:178-205`).
- Sin consumidor de otro microservicio.

## Flujo

1. `main()` registra `GET /auth/account-deletion` bajo `RequireIdentity` (`back/auth/cmd/server/main.go:124`).
2. `AccountDeletionHandler.Status` obtiene el usuario del JWT y responde el estado o los errores HTTP (`back/auth/internal/handler/http/account_deletion_handler.go:43-59`).
3. `AccountDeletionCoordinator.State` delega la lectura durable (`back/auth/internal/service/account_deletion.go:151-156`) a `AccountDeletionRepository.State`, que consulta `identity.account_deletions` (`back/auth/internal/repository/account_deletion_repository.go:147-157`).
4. Flutter lee `status` y `updated_at` con `AccountDeletionService.checkStatus` (`front/lib/core/services/account_deletion_service.dart:271-329`) y `AccountDeletionDialog._apply` muestra pendiente o finalización (`front/lib/features/auth/account_deletion_dialog.dart:178-205`).

## Tests

- `back/auth/internal/handler/http/account_deletion_contract_test.go:146` y :181 cubren estados complete y pending; :218 cubre no autorizado y errores.
