# DELETE /notes/{id}/save

Quita un apunte de los guardados del usuario. **Idempotente** y válido aunque el apunte original ya no exista (es la única acción permitida sobre un guardado "no disponible"). Desde el navegador: `DELETE https://ti3-brojas.dev.censei.cl/api-notes/notes/<id>/save`. No tiene equivalente gRPC.

## Req

- **Registro:** `back/notes/cmd/server/main.go` → `protected.DELETE("/notes/:id/save", h.Unsave)`. Handler `Unsave` en `note_handler.go`.
- **Autenticación:** `RequireAuth`.
- **Path:** `id` = `note_id` (UUID; no UUID → 404 `not_found`).
- **Query / body:** no tiene.

## Res

**204 No Content**, sin body, tanto si existía el guardado como si no.

| HTTP | `error.code` | Condición |
| --- | --- | --- |
| 404 | `not_found` | `id` no es UUID |
| 500 | `internal_error` | Fallo de Postgres |

**Efectos:** `DELETE FROM notes.saved_notes WHERE user_id=$1 AND note_id=$2` (`repository/saved_repository.go`). No consulta el apunte ni Drive.

## Flujo

`NoteHandler.Unsave` → `NoteService.Unsave` (`note_service.go`) → `SavedStore.Delete`.

## Tests

`back/notes/internal/service/saved_interactions_test.go`: `TestInteractionsCountersAreExactAndIdempotent` (doble DELETE) y `TestSavedNoteSurvivesOwnerDeletionAsUnavailable` (DELETE sobre apunte borrado).
