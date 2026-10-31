# GET /notes/me

Microservicio `notes`; ruta del router de producción.

## Req

- JWT HS256 en `Authorization: Bearer <access_token>`; identidad extraída del token.
- Permiso: Solo notas propias.
- Entrada: Query cursor?, limit? (default 20, máximo 100).

## Res

- 200 {notes:[],next_cursor:""}.
- 500 persistencia.
- `401` por token ausente/inválido/expirado, antes del handler. Errores con forma `{"error":{"code":"not_found","message":"Nota no encontrada"}}`; ver [catálogo común](../documentation/docHector.md#autenticación-y-errores).

Ejemplo de respuesta (IDs y fechas ilustrativos, campos del contrato real):

```json
{"notes":[],"next_cursor":""}
```

## Flujo

`ListMy` → `ListMy`. El cursor es opaco; reenviarlo sin interpretarlo hasta next_cursor vacío. Cada elemento contiene id, user_id, title, visibility, likes_count, version, created_at y updated_at; subject_id y external_file_id se omiten si no existen. No incluye content, attachments ni drive_url. Fechas UTC RFC3339. El cliente debe recorrer todas las páginas.

Fuentes: `back/notes/cmd/server/main.go`, `internal/handler/http/` y `internal/service/` dentro de `back/notes`.

## Tests

- Handler/router: `TestHandlerListMyPagination`.
- Dominio: `TestListMyPagination`.
- Alcance de integración y E2E: [matriz de cobertura](../documentation/notes_api_matrix.md).
