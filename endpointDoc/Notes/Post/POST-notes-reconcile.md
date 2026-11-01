# POST /notes/reconcile

Microservicio `notes`; ruta del router de producción.

## Req

- JWT HS256 en `Authorization: Bearer <access_token>`; identidad extraída del token.
- Permiso: Solo recursos propios.
- Entrada: Sin body.

## Res

- 200 {removed_notes,removed_attachments,pending,removed_note_ids}.
- 403 OAuth; 502 Drive; 500 DB.
- `401` por token ausente/inválido/expirado, antes del handler. Errores con forma `{"error":{"code":"not_found","message":"Nota no encontrada"}}`; ver [catálogo común](../documentation/docHector.md#autenticación-y-errores).

Ejemplo de respuesta (IDs y fechas ilustrativos, campos del contrato real):

```json
{"removed_notes":0,"removed_attachments":0,"pending":0,"removed_note_ids":[]}
```

## Flujo

`Reconcile` → `ReconcileDriveDeletions`. Reconcilia borrados hechos directamente en Drive. Solo missing confirmado (404/papelera) permite borrar metadata mediante flujo normal de Delete. Timeout, 403 ambiguo o OAuth revocado conservan datos. removed_note_ids permite limpiar SQLite del usuario. Es distinto del job periódico ReconcilePendingNotes, que procesa outbox y permisos.

Fuentes: `back/notes/cmd/server/main.go`, `internal/handler/http/` y `internal/service/` dentro de `back/notes`.

## Tests

- Handler/router: `TestHandlerReconcileDriveDeletions`.
- Dominio: `TestReconcileTimeoutNeverDeletes; TestReconcileRevokedOAuthNeverDeletes`.
- Alcance de integración y E2E: [matriz de cobertura](../documentation/notes_api_matrix.md).
