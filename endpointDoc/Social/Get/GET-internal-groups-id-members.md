# GET /internal/groups/:id/members

Lista miembros para servicio-a-servicio — checks `is_admin`/`member` de Notes al compartir (Social, tarea 3_5_4).

## Req
- Método: `GET`, ruta: `/internal/groups/:id/members` (protegida con `X-Internal-Key`, NO con JWT; `back/social/cmd/server/main.go` grupo `/internal`).
- Path: `id` (uuid del grupo).

## Res
- `200`: `[{"user_id":"<uuid>","role":"admin|member"}]` (siempre array; forma exacta que espera `SocialHTTPAdapter.listMembers` en Notes).
- `400`: `{"error":"invalid group ID"}`. `401`: key ausente/mala. `404`: `{"error":"group not found"}`. `500`: `{"error":"internal error"}`. `503`: internal key no configurada.

## Flujo
- `RequireInternalKey` (`back/social/internal/middleware/internal_middleware.go`, copia de auth) → `InternalHandler.Members` (`internal/handler/http/internal_handler.go`) → `GroupService.ListMembersInternal` (existe-grupo, sin exigir membresía) → `GroupRepository.ListMembers` (`social.group_memberships`).

## Tests
- `internal/handler/http/internal_handler_test.go`: `TestInternal_Members` (forma `[{user_id,role}]`, 401/404/400, 503 sin key configurada).
- `internal/service/internal_members_test.go`: `TestListMembersInternal` (2 miembros, 404 grupo/id malo).
- `internal/middleware/internal_middleware_test.go`: `TestRequireInternalKey`.
- Activa en Notes (sin cambios de código allá): `IsMember`/`IsAdmin` del share (`note_service.go:1921,2312,2587`) y `IsGroupAdmin` de `unshare-all` (desbloquea 3_5_5 kick/ban e2e).
