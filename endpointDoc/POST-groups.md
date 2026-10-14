# POST /groups

Crea un grupo y registra al creador como administrador (microservicio `social`).

## Req
- Método: `POST`, ruta: `/groups` (protegida con JWT `Bearer`; `back/social/cmd/server/main.go:224`).
- Headers: `Content-Type: application/json`, `Authorization: Bearer <access_token>`.
- Body (`createGroupRequest`, `internal/handler/http/group_handler.go:80-83`): `{"name":"Grupo de estudio","description":"opcional"}`.
- `name` requerido, no vacío tras recortar, máx. 200 bytes (`group_service.go:141`). `description` opcional (vacía = `nil`).

## Res
- `201`: `{"group_id":"<uuid>"}` (no devuelve el objeto completo).
- `400`: JSON inválido o nombre inválido. `401`: sin token o token inválido (middleware). `500`: `internal`.

## Flujo
- `GroupHandler.Create` (`internal/handler/http/group_handler.go:85`) lee el JSON y saca el `userID` del contexto (nunca del body) → `GroupService.Create` (`internal/service/group_service.go:141`: normaliza y valida) → `GroupRepository.CreateWithOwner` (`INSERT INTO social.groups` + membresía `admin` en una transacción, `internal/repository/group_repository.go:26`). Notificación Stream best-effort, no revierte el grupo.

## Tests
- `internal/handler/http/group_handler_test.go`: `TestCreateGroupContract` (201, 400, 401).
- `internal/service/group_service_test.go`: `TestCreate` (validaciones con dobles).
- `internal/handler/http/groups_postgres_e2e_test.go`: `TestGroupsPostgresE2E_MembershipLifecycle` (creación por HTTP real + rol `admin` verificado por SQL).
