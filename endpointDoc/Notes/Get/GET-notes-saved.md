# GET /notes/saved

Lista los apuntes que el usuario guardó (marcadores en `notes.saved_notes`), **incluidos los que ya no están disponibles** porque el autor los borró o dejó de compartirlos. Ruta interna del backend Notes (`:8082`). Desde el navegador: `GET https://ti3-brojas.dev.censei.cl/api-notes/notes/saved` ([prefijos](../COMUN-notes.md#1-rutas-internas-y-prefijos-externos)). No tiene equivalente gRPC.

## Req

- **Registro:** `back/notes/cmd/server/main.go` → `protected.GET("/notes/saved", h.ListSaved)` (ruta estática, convive con `/notes/:id`). Handler `ListSaved` en `back/notes/internal/handler/http/note_handler.go`.
- **Autenticación:** `RequireAuth`. Ver [COMUN §2](../COMUN-notes.md#2-autenticación-http).
- **Permiso:** cualquiera autenticado; solo ve **sus** guardados (`user_id` del JWT).
- **Query:**

| Parámetro | Tipo | Defecto | Descripción |
| --- | --- | --- | --- |
| `limit` | int | 20 (máx. 50) | Tamaño de página |
| `cursor` | UUID | — | `saved_id` del último elemento de la página anterior (`next_cursor`) |

- **Body:** no tiene.

## Res

**200 OK**

```json
{
  "items": [
    {
      "saved_id": "c930dff0-…", "note_id": "1386d23f-…", "saved_at": "2026-11-05T19:54:21Z",
      "is_available": true, "title": "Cálculo II", "user_id": "40f9076f-…",
      "subject_id": "…", "visibility": "public",
      "likes_count": 3, "saves_count": 2, "is_liked": true, "is_saved": true, "is_own": false,
      "updated_at": "2026-11-05T19:50:00Z"
    },
    {
      "saved_id": "…", "note_id": "…", "saved_at": "…",
      "is_available": false, "unavailable_reason": "deleted",
      "title": "Redes - Modelo OSI", "user_id": "…", "deleted_at": "2026-11-05T20:01:00Z",
      "likes_count": 0, "saves_count": 0, "is_liked": false, "is_saved": true, "is_own": false
    }
  ],
  "next_cursor": ""
}
```

- Orden: guardado más reciente primero. `next_cursor` vacío = no hay más páginas.
- `is_available=false` nunca se omite ni produce 500. `unavailable_reason`:
  - `deleted`: el autor borró el apunte. `title` sale de la lápida `notes.note_tombstones`.
  - `no_access`: el apunte existe pero el usuario ya no puede leerlo (pasó a privado o se retiró el share). Por zero-knowledge, `title` es el genérico "Apunte no disponible" y no se exponen metadatos actuales.
- En un elemento no disponible la única acción válida es `DELETE /notes/{note_id}/save`.

**Errores**

| HTTP | `error.code` | Condición |
| --- | --- | --- |
| 400 | `bad_request` | `cursor` no es UUID |
| 500 | `internal_error` | Fallo de Postgres |
| 401 / 423 / 503 | ver [COMUN §2](../COMUN-notes.md#2-autenticación-http) | Middleware |

## Flujo

1. `NoteHandler.ListSaved` → `NoteService.ListSaved` (`back/notes/internal/service/saved_interactions.go`).
2. `SavedStore.ListByUser` (todas las filas del usuario) → por cada una, `savedView`: `GetByID` del apunte; si no existe, lápida (`NoteTombstoneReader.GetTombstone`); si existe, `hasReadAccess` y contadores (`interactionsFor`).
3. Paginación por cursor sobre la lista ya filtrada.

## Tests

| Archivo | Prueba | Qué comprueba |
| --- | --- | --- |
| `back/notes/internal/service/saved_interactions_test.go` | `TestSavedNoteSurvivesOwnerDeletionAsUnavailable` | El guardado sobrevive al borrado y sale `is_available=false`, `deleted` |
| `back/notes/internal/service/saved_interactions_test.go` | `TestSavedNoteBecomesUnavailableWhenMadePrivate` | `no_access` sin filtrar el título |
| `back/notes/cmd/server/router_contract_test.go` | `TestHTTPRouterContract` | Ruta registrada; sin token → 401 |
