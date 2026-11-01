# GET /groups/:id/notes

Microservicio `notes`; ruta del router de producción.

## Req

- JWT HS256 en `Authorization: Bearer <access_token>`; identidad extraída del token.
- Permiso: Miembro del grupo.
- Entrada: Path id UUID; cursor?, limit?.

## Res

- 200 {notes:[],next_cursor:""}.
- 404 UUID inválido; 403 no miembro; 500 DB/Social.
- `401` por token ausente/inválido/expirado, antes del handler. Errores con forma `{"error":{"code":"not_found","message":"Nota no encontrada"}}`; ver [catálogo común](../documentation/docHector.md#autenticación-y-errores).

Ejemplo de respuesta (IDs y fechas ilustrativos, campos del contrato real):

```json
{"notes":[],"next_cursor":""}
```

## Flujo

`ListGroupNotes` → `ListGroupNotes → ListGroupNotePairs`. Devuelve metadata: id, user_id, title, visibility, likes_count, version, created_at y updated_at. No incluye shared_note_id, access_mode, content ni adjuntos; gRPC sí expone NoteWithShare. Se pagina el store con keyset sobre el orden total `(is_admin_note DESC, shared_at DESC, id DESC)` y cada página se reordena por admin, likes y fecha; no promete un ranking global entre páginas. Antes el cursor comparaba solo `shared_at`: omitía comparticiones con el mismo instante e ignoraba `is_admin_note` (2 de 6 alcanzables en la prueba PG).

Fuentes: `back/notes/cmd/server/main.go`, `internal/handler/http/` y `internal/service/` dentro de `back/notes`.

## Tests

- Handler/router: `TestHandlerShareAndGroupNotes; TestListGroupNotesOrdering`.
- Dominio: `ListGroupNotePairs ejercitado por TestListGroupNotesPaginationAndOrdering (gRPC)`.
- PostgreSQL real: `internal/repository/shared_pagination_integration_test.go` (`TestSharedListByGroupKeysetTiesAndAdmin`).
- E2E: `scripts/test-notes-e2e.py` pagina 5 comparticiones con `limit=2` sin omisiones.
- Alcance de integración y E2E: [matriz de cobertura](../documentation/notes_api_matrix.md).
