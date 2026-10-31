# POST /groups/:id/transfer-admin

Un admin traspasa su rol a otro miembro en una sola transacción (microservicio `social`).

## Req
- Método: `POST`, ruta: `/groups/:id/transfer-admin` (protegida con JWT `Bearer`; solo admin; `back/social/cmd/server/main.go:224`).
- Headers: `Content-Type: application/json`, `Authorization: Bearer <access_token>`.
- Path: `id` (uuid del grupo). Body (`transferAdminRequest`, `group_handler.go:258-260`): `{"new_admin_user_id":"<uuid>"}`.

## Res
- `200`: sin body (destinatario → `admin`, solicitante → `member`).
- `400`: body malo o transferencia propia. `403`: solicitante no-admin. `404`: grupo inexistente o destino no-miembro. `401`: sin token.

## Flujo
- `GroupHandler.TransferAdmin` (`internal/handler/http/group_handler.go:263`) → `GroupService.TransferAdmin` (`internal/service/group_service.go:488`) → `GroupRepository.TransferAdmin` (lock del grupo, exige admin, destino debe pertenecer, actualiza ambos roles juntos; `internal/repository/group_repository.go:378`). No toca otros admins ni `owner_user_id`.

## Tests
- `internal/handler/http/group_handler_test.go`: `TestTransferAdminContract`.
- `internal/service/group_service_test.go`: `TestTransferAdmin`.
- `internal/handler/http/groups_postgres_e2e_test.go`: `TestGroupsPostgresE2E_MembershipLifecycle` (ambos roles verificados por SQL).
