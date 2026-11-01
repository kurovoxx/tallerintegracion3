# POST /notes/{id}/like

Da "me gusta" a una nota legible. Inserta la fila en `notes.note_likes` e incrementa `notes.notes.likes_count` en la misma transacción. Ruta interna del backend Notes (`:8082`). Desde el navegador: `POST https://ti3-brojas.dev.censei.cl/api-notes/notes/<id>/like` ([prefijos](../COMUN-notes.md#1-rutas-internas-y-prefijos-externos)). Para quitarlo: [DELETE /notes/{id}/like](../Delete/DELETE-notes-id-like.md). No tiene equivalente gRPC.

## Req

- **Registro:** `back/notes/cmd/server/main.go:205` → `protected.POST("/notes/:id/like", h.Like)`. Handler `back/notes/internal/handler/http/note_handler.go:674-690` (`Like`).
- **Autenticación:** `RequireAuth` (`main.go:192`). Ver [COMUN §2](../COMUN-notes.md#2-autenticación-http).
- **Permiso:** debe poder leer la nota (`hasReadAccess`, `back/notes/internal/service/note_service.go:1915-1921`). El acceso se comprueba antes de mirar si ya hay like, para no revelar ni la existencia ni un like previo.
- **Path:** `id` UUID (no UUID → 404 `not_found`).
- **Query / body:** no tiene.

## Res

**201 Created**, sin body (`note_handler.go:689`).

**Errores**

| HTTP | `error.code` | Condición |
| --- | --- | --- |
| 404 | `not_found` "nota no encontrada" | No existe o es privada sin acceso (`note_service.go:1911-1921`) |
| 409 | `already_liked` "Nota ya likeada" | El usuario ya dio like (comprobación previa o carrera revalidada) (`L1922-1934`) |
| 500 | `internal_error` | Fallo de Postgres o de Social al comprobar membresía |
| 401 / 423 / 503 | ver [COMUN §2](../COMUN-notes.md#2-autenticación-http) | Middleware |

**Efectos:** transacción `LikeRepository.LikeAtomic` (`back/notes/internal/repository/like_repository.go:69-95`): `INSERT INTO notes.note_likes` y `UPDATE notes.notes SET likes_count = likes_count + 1, updated_at = now()`; commit. Una violación de unicidad (`23505`) se reporta como `already_liked`. No toca Drive. Nada en segundo plano.

Nota: el UPDATE cambia `updated_at` pero **no** `version`.

## Invocación (Front / Otros Servicios)

- **Frontend Flutter:** No se encontró invocación en el código revisado. El botón de corazón del detalle (`front/lib/features/notes/all_notes_screen.dart:4909-4922`) y de las tarjetas (`L3263`, `L3291`) llaman `_toggleLike` (`L2158-2170`), que solo cambia `_isLiked`/`_likesCount` en memoria. La pantalla muestra `likes_count` del backend solo en la pestaña de grupo (`front/lib/features/groups/group_notes_tab.dart:389`).
- **Otros microservicios:** No se encontró invocación en el código revisado.
- **Tests/scripts:** `scripts/test-notes-e2e.py:133-134` (201 y luego 409).

## Flujo

1. `main.go:205` → `RequireAuth` (`auth_middleware.go:91`).
2. `NoteHandler.Like` (`note_handler.go:674`): `validateNoteID`.
3. `NoteService.Like` (`note_service.go:1903`): `GetByID`, `hasReadAccess` (Social `IsMember` por gRPC para shares `restricted`), `LikeStore.Exists` (`service/store_pg.go:101`), `LikeStore.LikeAtomic` (`store_pg.go:104` → `like_repository.go:69`).
4. Respuesta 201.

Finalmente devuelve `201` sin body; hoy ningún cliente de producción consume ese resultado, aunque el nuevo `likes_count` se refleja en `GET /notes/:id`, `GET /notes/me` y en el orden de `GET /groups/:id/notes`.

## Tests

| Archivo:línea | Prueba | Qué comprueba | Tipo |
| --- | --- | --- | --- |
| `back/notes/internal/handler/http/note_handler_test.go:254` | `TestHandlerLikeAndCopy` | Like 201, repetido 409, unlike 204 | Unitaria |
| `note_handler_test.go:627` | `TestConcurrentLikes` | Likes concurrentes vía HTTP | Unitaria |
| `back/notes/internal/service/note_service_test.go:427` | `TestLikeUnlikeTransactional` | Contador consistente | Unitaria |
| `note_service_test.go:707` | `TestConcurrentLikes` | Concurrencia en servicio | Unitaria |
| `back/notes/internal/service/like_read_contract_test.go:13,77` | `TestLikeReadAccess`, `TestLikeRevokedAccessHidesExistingLike` | Exige lectura; acceso revocado oculta el like previo | Unitaria |
| `back/notes/cmd/server/router_contract_test.go:15` | `TestHTTPRouterContract` | Ruta registrada; sin token → 401 | Unitaria |

Todas usan stores en memoria (`LikeAtomic` en memoria con mutex). No hay prueba directa en el código revisado de `LikeRepository.LikeAtomic` contra Postgres real.
