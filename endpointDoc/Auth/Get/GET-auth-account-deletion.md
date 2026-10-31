# GET /auth/account-deletion

Estado del borrado de la cuenta propia. Público vía nginx como `/api-auth/auth/account-deletion`.

## Req
- Método: `GET`, ruta: `/auth/account-deletion` (`back/auth/cmd/server/main.go:121`, middleware `RequireIdentity`).
- Headers requeridos: `Authorization: Bearer <access_token>`. Funciona aunque la cuenta ya esté bloqueada o borrada (mientras el JWT no venza).

## Res
- `200`: `{"status":"pending"|"complete","updated_at":"<RFC3339>"}` desde `identity.account_deletions`.
- `404`: `{"status":"not_started"}`. Nunca se inició: reintentar el `DELETE` es seguro.
- `503`: `{"error":"account_status_unavailable"}`.
- `401`: `{"error":{"code":...,"message":"..."}}` (middleware).

## Uso en el frontend
`AccountDeletionService.checkStatus` (`front/lib/core/services/account_deletion_service.dart`): tras un `DELETE` ambiguo (red, timeout, `503`, `403/409/5xx` o cuerpo inesperado) y en el sondeo del estado `pending` (cada 4 s, hasta 15 veces, más el botón «Consultar estado»). Un `404` sin `{"status":"not_started"}` se trata como estado desconocido.

## Flujo
`AccountDeletionHandler.Status` (`internal/handler/http/account_deletion_handler.go`) → `AccountDeletionCoordinator.State` → `AccountDeletionRepository.State`.

## Tests
- `back/auth/internal/handler/http/account_deletion_contract_test.go` (not_started, pending, complete con `updated_at`, 503, 401).
- `front/test/account_deletion_service_test.dart` (`GET /auth/account-deletion`).
