# POST /notes/unshare-all

Microservicio `notes`; ruta del router de producción.

## Req

- JWT HS256 en `Authorization: Bearer <access_token>`; identidad extraída del token.
- Permiso: Usuario objetivo o admin del grupo (JWT).
- Entrada: JSON user_id UUID, group_id UUID.

```json
{"user_id":"22222222-2222-4222-8222-222222222222","group_id":"55555555-5555-4555-8555-555555555555"}
```

## Res

- 204 sin cuerpo.
- 400 JSON/UUID; 403 permisos; 413 JSON; 500 DB.
- `401` por token ausente/inválido/expirado, antes del handler. Errores con forma `{"error":{"code":"not_found","message":"Nota no encontrada"}}`; ver [catálogo común](../documentation/docHector.md#autenticación-y-errores).

## Flujo

`UnshareAll` → `UnshareAll`. Fallback HTTP deprecado; Social debe usar NotesService/UnshareAll con x-internal-key. user_id es el objetivo explícito, nunca identidad del llamante. Elimina vínculos del objetivo en el grupo y reconcilia ACL de notas de otros autores para retirar acceso del miembro saliente. Conserva permisos de otros grupos; es idempotente.

Fuentes: `back/notes/cmd/server/main.go`, `internal/handler/http/` y `internal/service/` dentro de `back/notes`.

## Tests

- Handler/router: `TestUnshareAllIDORForbidden; TestUnshareAllSelfAllowedAndAdminAllowed`.
- Dominio: `TestUnshareAllRevokesOnlyOwnerGroupAcrossPages; TestUnshareAllRevokesLeaverOnOtherAuthorsNote`.
- Alcance de integración y E2E: [matriz de cobertura](../documentation/notes_api_matrix.md).
