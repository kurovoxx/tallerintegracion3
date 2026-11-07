# POST /notes/{id}/save

Guarda una referencia (marcador) a una nota legible en `notes.saved_notes`, sin copiar el archivo. Ruta interna del backend Notes (`:8082`). Desde el navegador: `POST https://ti3-brojas.dev.censei.cl/api-notes/notes/<id>/save` ([prefijos](../COMUN-notes.md#1-rutas-internas-y-prefijos-externos)). No tiene equivalente gRPC. Para quitarlo: [`DELETE /notes/{id}/save`](../Delete/DELETE-notes-id-save.md); para listarlos: [`GET /notes/saved`](../Get/GET-notes-saved.md).

## Req

- **Registro:** `back/notes/cmd/server/main.go:203` → `protected.POST("/notes/:id/save", h.Save)`. Handler `back/notes/internal/handler/http/note_handler.go:620-637` (`Save`).
- **Autenticación:** `RequireAuth` (`main.go:192`). Ver [COMUN §2](../COMUN-notes.md#2-autenticación-http).
- **Permiso:** el usuario debe poder leer la nota (`hasReadAccess`, `note_service.go:617-642`): autor, `public`, share `link`, o miembro de un grupo con share `restricted`. El autor puede guardar su propia nota.
- **Path:** `id` UUID (no UUID → 404 `not_found`).
- **Query / body:** no tiene.

## Res

**201 Created** con contadores (`NoteHandler.Save` → `respondInteractions`).

```json
{ "note_id": "…", "likes_count": 0, "saves_count": 1, "is_liked": false, "is_saved": true }
```

El body son los contadores exactos tras la escritura (igual que [`GET /notes/{id}/stats`](../Get/GET-notes-id-stats.md)). Si releerlos fallara, se responde `201` sin body: la escritura ya quedó hecha.

**Idempotencia:** repetir la operación nunca duplica la fila ni el contador (`UNIQUE` en BD + comprobación previa + revalidación tras carrera). La repetición responde el `409` del contrato (`agentApiContract2.md` §3); el cliente debe tratarlo como "ya estaba en ese estado".

**Errores**

| HTTP | `error.code` | Condición |
| --- | --- | --- |
| 404 | `not_found` "nota no encontrada" | No existe o es privada sin acceso (indistinguibles) (`note_service.go:1659-1670`) |
| 410 | `note_deleted` | El autor lo borró y era público (o ya estaba guardado) |
| 409 | `already_saved` "Nota ya guardada" | Ya existe la fila (comprobación previa o carrera revalidada) (`L1674-1685`) |
| 500 | `internal_error` | Fallo de Postgres, o de Social al comprobar membresía (`L632-635`) |
| 401 / 423 / 503 | ver [COMUN §2](../COMUN-notes.md#2-autenticación-http) | Middleware |

**Efectos:** una fila en `notes.saved_notes (user_id, note_id)` (`back/notes/internal/repository/saved_repository.go:20-37`). No toca Drive ni crea permisos. Si el autor borra la nota, **la fila sobrevive** (migración `004_saved_notes_unavailable.sql`: `saved_notes.note_id` sin FK y lápida en `notes.note_tombstones`) y aparece en `GET /notes/saved` con `is_available=false`. Nada en segundo plano.

## Invocación (Front / Otros Servicios)

- **Frontend Flutter:** No se encontró invocación en el código revisado. El botón **Guardar** del detalle (`front/lib/features/notes/all_notes_screen.dart:4923-4932`) llama `_toggleSave` (`L2172-2196`), que solo cambia el mapa en memoria `_isSaved` y muestra "Nota guardada localmente". No envía HTTP ni escribe en Drift.
- **Otros microservicios:** No se encontró invocación en el código revisado.
- **Tests/scripts:** `scripts/test-notes-e2e.py:131-132` (201 y luego 409).

## Flujo

1. `main.go:203` → `RequireAuth` (`auth_middleware.go:91`).
2. `NoteHandler.Save` (`note_handler.go:620`): `validateNoteID`.
3. `NoteService.Save` (`note_service.go:1651`): `GetByID`, `hasReadAccess` (para shares `restricted`, Social `IsMember` por gRPC, `service/social_grpc_adapter.go:57`), `SavedStore.Exists` (`service/store_pg.go:81`), `SavedStore.Save` (`store_pg.go:78` → `saved_repository.go:20`).
4. Respuesta 201.

Finalmente devuelve `201` con los contadores actualizados.

## Tests

| Archivo:línea | Prueba | Qué comprueba | Tipo |
| --- | --- | --- | --- |
| `back/notes/internal/handler/http/note_handler_test.go:314` | `TestHandlerSaveConflict` | 201 y segundo intento 409 | Unitaria |
| `back/notes/internal/service/note_service_test.go:355` | `TestSaveAndAlreadySaved` | Regla `already_saved` | Unitaria |
| `back/notes/cmd/server/router_contract_test.go:15` | `TestHTTPRouterContract` | Ruta registrada; sin token → 401 | Unitaria |

No hay prueba directa en el código revisado del caso "nota privada sin acceso → 404" para esta ruta concreta (la regla `hasReadAccess` se prueba en GET y like).
