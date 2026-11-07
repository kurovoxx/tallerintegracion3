# GET /notes/public

Lista apuntes **públicos de otros usuarios** para descubrirlos, darles like o guardarlos desde su tarjeta. Los propios no aparecen (ya están en `GET /notes/me`). Desde el navegador: `GET https://ti3-brojas.dev.censei.cl/api-notes/notes/public`. No tiene equivalente gRPC.

> Alcance: es un listado mínimo por fecha con búsqueda literal en el título, no el "feed de descubrimiento" con búsqueda difusa que `agentMain.md` §8 sigue dejando como diseño futuro.

## Req

- **Registro:** `back/notes/cmd/server/main.go` → `protected.GET("/notes/public", h.ListPublic)`.
- **Autenticación:** `RequireAuth`. Ver [COMUN §2](../COMUN-notes.md#2-autenticación-http).
- **Query:**

| Parámetro | Tipo | Defecto | Descripción |
| --- | --- | --- | --- |
| `q` | texto (≤ 300) | — | Búsqueda literal sin distinguir mayúsculas en el título (`ILIKE` con `%`/`_` escapados) |
| `subject_id` | UUID | — | Solo esa materia |
| `limit` | int | 20 (máx. 50) | Tamaño de página |
| `cursor` | UUID | — | `id` del último apunte de la página anterior |

## Res

**200 OK**

```json
{
  "items": [
    {
      "id": "…", "user_id": "…", "title": "Integrales dobles", "subject_id": "…",
      "visibility": "public", "likes_count": 4, "saves_count": 2,
      "is_liked": false, "is_saved": true,
      "created_at": "2026-11-05T19:54:21Z", "updated_at": "2026-11-05T19:54:21Z"
    }
  ],
  "next_cursor": ""
}
```

- Solo `visibility = 'public'`, con archivo ya en Drive (`external_file_id IS NOT NULL`) y de un autor distinto al solicitante. Orden: `created_at DESC, id DESC` (keyset).
- No incluye `content` (no descarga de Drive); para leerlo: `GET /notes/{id}`.

| HTTP | `error.code` | Condición |
| --- | --- | --- |
| 400 | `bad_request` | `cursor` no UUID o `q` > 300 |
| 400 | `invalid_subject_id` | `subject_id` no UUID |
| 500 | `internal_error` | Fallo de Postgres |

## Flujo

`NoteHandler.ListPublic` → `NoteService.ListPublic` (`service/saved_interactions.go`) → `PublicNoteLister.ListPublic` (`NoteRepository.ListPublic`, `repository/note_repository.go`) → `interactionsFor` por elemento.

## Invocación

- **Frontend Flutter:** `NotesApi.listPublic` y la pantalla **Explorar públicos** (`front/lib/features/notes/public_notes_screen.dart`), con like y guardar en cada tarjeta.

## Tests

`back/notes/internal/service/saved_interactions_test.go` → `TestListPublicExcludesOwnAndPrivateAndCanBeSaved`; `cmd/server/router_contract_test.go` → ruta registrada y protegida.
