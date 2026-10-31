# GET /internal/groups/:id/member-emails

Emails de miembros para shares `restricted` de Notes (Social, tarea 3_5_4).

## Req
- Método: `GET`, ruta: `/internal/groups/:id/member-emails` (protegida con `X-Internal-Key`, NO con JWT).
- Path: `id` (uuid del grupo).

## Res
- `200`: `["a@x.com",...]` (siempre array; `[]` si nadie tiene email; minúsculas, dedup, en orden de miembros — lo que espera `SocialHTTPAdapter.ListMemberEmails`).
- `401`: key ausente/mala. `404`: grupo inexistente. `500`: Identity inalcanzable o directorio sin configurar (fail-closed: Notes aborta el share con `ErrInternalDatabase` en vez de calcular permisos incompletos, `note_service.go:1975-1978`).

## Flujo
- `RequireInternalKey` → `InternalHandler.MemberEmails` → `GroupService.ListMemberEmailsInternal` (miembros + `LookupEmailsStrict` a Identity `GET /internal/users/lookup`) → `social.group_memberships` + `identity.users`. La vía best-effort anterior (`LookupUsers`, usada en enriquecer miembros) no se tocó.

## Tests
- `internal/handler/http/internal_handler_test.go`: `TestInternal_MemberEmails` (200 + forma, 404, 500 con directorio caído/sin configurar, 401, `[]` nunca null).
- `internal/service/internal_members_test.go`: `TestLookupEmailsStrict_Normaliza` (minúsculas/dedupe/orden), `TestLookupEmailsStrict_FailClosed` (500/red → error), `TestListMemberEmailsInternal` (2 emails, 404, sin directorio → error).

## Pendiente (fuera de alcance)
- `GET /internal/users/:id/followers-count`: sin lógica de follows en social (solo tablas); el share lo trata best-effort (`followers, _ :=`, snapshot 0).
