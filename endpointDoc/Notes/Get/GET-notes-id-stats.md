# GET /notes/{id}/stats

Contadores exactos de un apunte y estado del solicitante, **sin descargar el contenido de Drive**. Útil para refrescar tarjetas tras dar/quitar like o guardar. Desde el navegador: `GET https://ti3-brojas.dev.censei.cl/api-notes/notes/<id>/stats`.

## Req

- **Registro:** `main.go` → `protected.GET("/notes/:id/stats", h.Stats)`.
- **Autenticación:** `RequireAuth`.
- **Permiso:** el usuario debe poder leer el apunte (`hasReadAccess`: autor, `public`, share `link` o miembro de grupo con share `restricted`).
- **Path:** `id` UUID.

## Res

**200 OK**

```json
{ "note_id": "…", "likes_count": 3, "saves_count": 2, "is_liked": true, "is_saved": false }
```

- `likes_count`: columna desnormalizada `notes.notes.likes_count`, actualizada en la misma transacción que `note_likes` (`LikeAtomic` / `UnlikeAtomic`).
- `saves_count`: `count(*)` de `notes.saved_notes` por `note_id` (índice `idx_saved_notes_note_id`).

| HTTP | `error.code` | Condición |
| --- | --- | --- |
| 404 | `not_found` | No existe o es privado sin acceso (indistinguibles) |
| 410 | `note_deleted` | El autor lo borró y el solicitante lo tenía guardado o era público |
| 500 | `internal_error` | Fallo de Postgres / Social |

## Flujo

`NoteHandler.Stats` → `NoteService.Interactions` (`service/saved_interactions.go`) → `GetByID`, `hasReadAccess`, `interactionsFor` (`SavedStore.CountByNote`, `LikeStore.Exists`, `SavedStore.Exists`).
