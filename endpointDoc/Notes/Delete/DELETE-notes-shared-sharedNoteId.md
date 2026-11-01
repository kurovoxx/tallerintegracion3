# DELETE /notes/shared/:sharedNoteId

Microservicio `notes`; ruta del router de producción.

## Req

- JWT HS256 en `Authorization: Bearer <access_token>`; identidad extraída del token.
- Permiso: Autor de nota o admin del grupo.
- Entrada: Path sharedNoteId UUID.

## Res

- 204 sin cuerpo.
- 404 vínculo ausente; 403 permisos; 500 DB/Social.
- `401` por token ausente/inválido/expirado, antes del handler. Errores con forma `{"error":{"code":"not_found","message":"Nota no encontrada"}}`; ver [catálogo común](../documentation/docHector.md#autenticación-y-errores).

## Flujo

`Unshare` → `Unshare`. El parámetro es el ID del vínculo, no el de la nota. Retira la compartición y reconcilia la unión de permisos restantes. No revoca emails requeridos por otro grupo ni acceso público aún necesario. Fallo de revocación Drive queda pendiente de convergencia.

Fuentes: `back/notes/cmd/server/main.go`, `internal/handler/http/` y `internal/service/` dentro de `back/notes`.

## Tests

- Handler/router: `TestHandlerShareAndGroupNotes`.
- Dominio: `TestUnsharePreservesOtherGroupPermissions; TestUnshareRevokesNominalPermissionAndClearsRow`.
- Alcance de integración y E2E: [matriz de cobertura](../documentation/notes_api_matrix.md).
