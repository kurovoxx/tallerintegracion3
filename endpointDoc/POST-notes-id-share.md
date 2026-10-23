# POST /notes/:id/share

Microservicio `notes`; ruta del router de producción.

## Req

- JWT HS256 en `Authorization: Bearer <access_token>`; identidad extraída del token.
- Permiso: Autor y miembro del grupo.
- Entrada: JSON group_id UUID, access_mode link|restricted.

```json
{"group_id":"55555555-5555-4555-8555-555555555555","access_mode":"restricted"}
```

## Res

- 201 {shared_note_id}.
- 400 validación; 403 autor/membresía; 404 nota; 413 JSON; 500 DB/directorio.
- `401` por token ausente/inválido/expirado, antes del handler. Errores con forma `{"error":{"code":"not_found","message":"Nota no encontrada"}}`; ver [catálogo común](../documentation/docHector.md#autenticación-y-errores).

Ejemplo de respuesta (IDs y fechas ilustrativos, campos del contrato real):

```json
{"shared_note_id":"66666666-6666-4666-8666-666666666666"}
```

## Flujo

`Share` → `Share`. Persiste intención y ACL deseado antes de converger Drive bajo lock por nota. restricted concede reader a emails de Google conectados de miembros; link concede anyone reader. Un fallo Drive posterior no revierte el share: consultar access para convergencia failed/pending/synced. Social/directorio caído falla cerrado antes de crear la intención. HTTP no retorna el estado de permisos en el 201.

Fuentes: `back/notes/cmd/server/main.go`, `internal/handler/http/` y `internal/service/` dentro de `back/notes`.

## Tests

- Handler/router: `TestHandlerShareRestrictedGrantsDrive; TestHandlerShareAndGroupNotes`.
- Dominio: `TestRestrictedSharePersistsEmailPermissionIDs; TestRestrictedUsesGoogleEmailsNotAppEmails`.
- Alcance de integración y E2E: [matriz de cobertura](../documentation/notes_api_matrix.md).
