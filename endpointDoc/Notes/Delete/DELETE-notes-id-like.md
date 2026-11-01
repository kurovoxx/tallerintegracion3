# DELETE /notes/{id}/like

Quita el "me gusta" del usuario. Borra la fila de `notes.note_likes` y, solo si existía, decrementa `likes_count` en la misma transacción. Es idempotente. Ruta interna del backend Notes (`:8082`). Desde el navegador: `DELETE https://ti3-brojas.dev.censei.cl/api-notes/notes/<id>/like` ([prefijos](../COMUN-notes.md#1-rutas-internas-y-prefijos-externos)). No tiene equivalente gRPC.

## Req

- **Registro:** `back/notes/cmd/server/main.go:206` → `protected.DELETE("/notes/:id/like", h.Unlike)`. Handler `back/notes/internal/handler/http/note_handler.go:693-709` (`Unlike`).
- **Autenticación:** `RequireAuth` (`main.go:192`). Ver [COMUN §2](../COMUN-notes.md#2-autenticación-http).
- **Permiso:** **no** comprueba lectura ni existencia de la nota (`back/notes/internal/service/note_service.go:1940-1949`); solo borra el like del propio usuario si existe.
- **Path:** `id` UUID (no UUID → 404 `not_found`).
- **Query / body:** no tiene.

## Res

**204 No Content**, sin body (`note_handler.go:708`). Se devuelve 204 aunque el usuario no tuviera like o la nota no exista.

**Errores**

| HTTP | `error.code` | Condición |
| --- | --- | --- |
| 404 | `not_found` "nota no encontrada" | Solo si `id` no es UUID (`note_handler.go:701-703`) |
| 500 | `internal_error` | Fallo de Postgres (`note_service.go:1945-1947`) |
| 401 / 423 / 503 | ver [COMUN §2](../COMUN-notes.md#2-autenticación-http) | Middleware |

El código `not_liked` existe en el catálogo (`back/notes/internal/utils/response.go:43`) pero esta ruta no lo usa.

**Efectos:** transacción `LikeRepository.UnlikeAtomic` (`back/notes/internal/repository/like_repository.go:99-120`): `DELETE FROM notes.note_likes WHERE note_id=$1 AND user_id=$2`; si borró una fila, `UPDATE notes.notes SET likes_count = GREATEST(likes_count - 1, 0), updated_at = now()`; commit. No toca Drive. Nada en segundo plano.

## Invocación (Front / Otros Servicios)

- **Frontend Flutter:** No se encontró invocación en el código revisado. Quitar el corazón en `AllNotesScreen` (`_toggleLike`, `front/lib/features/notes/all_notes_screen.dart:2158-2170`) solo actualiza el estado en memoria.
- **Otros microservicios:** No se encontró invocación en el código revisado.
- **Tests/scripts:** `scripts/test-notes-e2e.py:135-136` (dos DELETE seguidos, ambos 204).

## Flujo

1. `main.go:206` → `RequireAuth` (`auth_middleware.go:91`).
2. `NoteHandler.Unlike` (`note_handler.go:693`): `validateNoteID`.
3. `NoteService.Unlike` (`note_service.go:1940`) → `LikeStore.UnlikeAtomic` (`service/store_pg.go:107` → `like_repository.go:99`).
4. Respuesta 204.

Finalmente devuelve `204` sin body; hoy ningún cliente de producción consume ese resultado.

## Tests

| Archivo:línea | Prueba | Qué comprueba | Tipo |
| --- | --- | --- | --- |
| `back/notes/internal/handler/http/note_handler_test.go:254` | `TestHandlerLikeAndCopy` | Unlike → 204 | Unitaria |
| `back/notes/internal/service/note_service_test.go:427` | `TestLikeUnlikeTransactional` | Contador tras like/unlike | Unitaria |
| `back/notes/internal/service/like_read_contract_test.go:97` | `TestLikeUnlikeOnlyDecrementsDeletedLike` | Solo decrementa si borró una fila | Unitaria |
| `back/notes/cmd/server/router_contract_test.go:15` | `TestHTTPRouterContract` | Ruta registrada; sin token → 401 | Unitaria |

No hay prueba directa en el código revisado de `UnlikeAtomic` contra Postgres real.
