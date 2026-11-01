# GET /notes/me

Lista paginada de los apuntes propios del usuario autenticado (solo metadata). Es la fuente que alimenta el índice local Drift/SQLite del front. Ruta interna del backend Notes (`:8082`). Desde el navegador: `GET https://ti3-brojas.dev.censei.cl/api-notes/notes/me` ([prefijos](../COMUN-notes.md#1-rutas-internas-y-prefijos-externos)). Equivalente gRPC: `NotesService/ListMyNotes`.

## Req

- **Registro:** `back/notes/cmd/server/main.go:195` → `protected.GET("/notes/me", h.ListMy)`. Se registra antes que `GET /notes/:id` (`L210`) para que `me` no se interprete como id. Handler `back/notes/internal/handler/http/note_handler.go:254-296` (`ListMy`).
- **Autenticación:** `RequireAuth` (`main.go:192`). Solo devuelve notas cuyo `user_id` es el del token. Ver [COMUN §2](../COMUN-notes.md#2-autenticación-http).
- **Query:**

| Parámetro | Tipo | Obligatorio | Validación |
| --- | --- | --- | --- |
| `limit` | entero | no | Por defecto 20. Si no es entero se ignora (queda 20) (`note_handler.go:262-268`). `<= 0` → 20; `> 100` → 100 (`back/notes/internal/service/note_service.go:1014-1019`) |
| `cursor` | string | no | Valor `next_cursor` de la página anterior. Si no está vacío debe ser UUID (`L1021-1023`). En la práctica es el `id` de la última nota devuelta (`back/notes/internal/repository/note_repository.go:123-127`) |

- **Body:** no tiene.

## Res

**200 OK** (`note_handler.go:275-295`)

```json
{
  "notes": [
    {
      "id": "11111111-1111-4111-8111-111111111111",
      "user_id": "22222222-2222-4222-8222-222222222222",
      "title": "Álgebra",
      "visibility": "private",
      "likes_count": 0,
      "version": 3,
      "created_at": "2026-10-22T12:00:00Z",
      "updated_at": "2026-10-23T09:30:00Z",
      "subject_id": "6b0c1f0e-2f4a-4c1e-9d7a-3a1b2c3d4e5f",
      "external_file_id": "1AbCdriveFileId"
    }
  ],
  "next_cursor": "11111111-1111-4111-8111-111111111111"
}
```

- `notes`: siempre lista (vacía si no hay notas, `note_service.go:1028-1030`). Orden `created_at DESC, id DESC` (`note_repository.go:98-106`).
- Cada elemento: `id`, `user_id`, `title`, `visibility`, `likes_count`, `version`, `created_at`, `updated_at` (RFC3339 UTC); `subject_id` y `external_file_id` solo si existen. **No** incluye `content`, `attachments`, `drive_url`, `sync_status` ni `forked_from_note_id`.
- `next_cursor`: `""` cuando no hay más páginas. El repositorio pide `limit+1` filas para saber si hay otra página (`note_repository.go:100,106,124-127`).

**Errores**

| HTTP | `error.code` | Condición |
| --- | --- | --- |
| 400 | `bad_request` "cursor inválido" | `cursor` no vacío y no UUID (`note_service.go:1021-1023`) |
| 500 | `internal_error` "Error interno" | Fallo de Postgres (`L1024-1027`) |
| 401 / 423 / 503 | ver [COMUN §2](../COMUN-notes.md#2-autenticación-http) | Middleware |

**Efectos:** solo lectura. No consulta Drive ni Social.

## Invocación (Front / Otros Servicios)

1. **Sincronización de la lista de notas** — `AllNotesScreen` (`front/lib/features/notes/all_notes_screen.dart:138`).
   - Acción: abrir la pantalla (`_initAndLoad` `L335` llama `_syncFromBackend` en `L398`, `L405` o `L413`), al final del flujo del botón **REFRESCAR / SINCRONIZAR DRIVE** (`_syncWithDrive` `L856`, botón `L2898-2908`, llamada final `L982`), y tras clonar una nota (`L1984`).
   - Cliente: `_syncFromBackend` (`L474-492`, evita sincronizaciones en paralelo) → `_syncFromBackendImpl` (`L558-683`) → `_fetchAllMyNotes` (`L508-556`).
   - Petición: `GET $notesBaseUrl/notes/me?limit=50[&cursor=…]` (`L518-523`) con `_authHeaders()` (Bearer `SessionManager.token`, `L1540-1550`), timeout 8 s, vía `AuthedHttp.run`. Recorre páginas hasta `next_cursor` vacío, con un tope de 40 páginas (`_kMaxNotesPages`, `L78`) y corte si un cursor se repite (`L515-517`). Si una página falla devuelve `null` y no hace convergencia parcial (`L531-542`).
   - **Índice local Drift/SQLite:** por cada nota remota hace `LocalNotesRepository.upsertNote` (`L616-635`; `front/lib/core/database/local_notes_repository.dart:14`) en la tabla `local_notes` (`front/lib/core/database/app_database.dart:10`). Triggers de SQLite mantienen la tabla virtual FTS5 `local_notes_fts` (`app_database.dart:66-91`; ver comentario en `local_notes_repository.dart:12-13`) que usa la búsqueda offline `searchNotesFts` (`local_notes_repository.dart:152-254`; con fallback `LIKE` si FTS5 no está disponible). Reglas: no resucita notas con tombstone (`L606`), no pisa notas locales con cambios pendientes ni con versión mayor (`L609-613`), y como el listado no trae `content` conserva el contenido local ya guardado (`L623-626`). Con el listado completo limpia tombstones ya confirmados (`L642-658`).
   - Persistencia: en nativo la BD es un archivo (`front/lib/core/database/connection/native.dart`); en web es SQLite WASM **en memoria** (`connection/web.dart`), por lo que el índice no sobrevive a recargar la pestaña.
   - Si falla: muestra "No pudimos sincronizar tus notas. Puedes seguir usando las notas guardadas en este equipo." (`L659-666`).
2. **Selector "Compartir una nota"** — `GroupNotesTab._shareFlow` (`front/lib/features/groups/group_notes_tab.dart:127-135`) → `NotesSharingService.listMyNotes` (`front/lib/core/services/notes_sharing_service.dart:204-220`): `GET {notesApiBaseUrl}/notes/me?limit=50`, **solo la primera página**, sin cursor. No escribe en Drift.
- `NotesApi.listMyNotes` (`front/lib/features/notes/notes_api.dart:119-140`): solo la usa `front/test/cross_service_e2e_test.dart`.
- **Otros microservicios:** No se encontró invocación HTTP. Internamente el servicio reutiliza `ListByUser` (no el endpoint) en `UnshareAll` y en el borrado de cuenta.
- **Tests/scripts:** `scripts/test-notes-e2e.py` (pagina 51 notas con `limit=50`).

## Flujo

1. `main.go:195` → `RequireAuth` (`auth_middleware.go:91`).
2. `NoteHandler.ListMy` (`note_handler.go:254`): lee `cursor` y `limit`.
3. `NoteService.ListMy` (`note_service.go:1013`): normaliza `limit`, valida `cursor`.
4. `PGNoteStore.ListByUser` (`service/store_pg.go:28`) → `NoteRepository.ListByUser` (`repository/note_repository.go:88-129`): keyset `(created_at, id) < (cursor)`.
5. Handler arma la lista mínima y responde 200.

Finalmente devuelve `200 {notes, next_cursor}`, y la pantalla `AllNotesScreen` utiliza ese resultado para actualizar las filas de `local_notes` en Drift (y con ello el índice FTS5 de búsqueda offline), mientras que la pestaña de grupo solo lo usa para llenar el selector de notas a compartir.

## Tests

| Archivo:línea | Prueba | Qué comprueba | Tipo |
| --- | --- | --- | --- |
| `back/notes/internal/handler/http/note_handler_test.go:480` | `TestHandlerListMyPagination` | Paginación por cursor | Unitaria |
| `access_contract_test.go:57,234` | `TestNoteListsEmptyArrays`, `TestNoteListsUTCRFC3339` | Lista vacía como `[]` y fechas UTC | Unitaria |
| `back/notes/internal/service/note_service_test.go:279` | `TestListMyPagination` | Paginación en el servicio | Unitaria |
| `back/notes/internal/handler/grpc/list_my_contract_test.go:9` | `TestListMyNotesPaginationAndIsolation` | Paginación y aislamiento entre usuarios vía gRPC (bufconn) | Unitaria gRPC |
| `front/test/notes_sync_duplicates_regression_test.dart:354` | `cursor cíclico se detiene sin repetir cuarenta solicitudes` | Corte ante cursor repetido | Widget (HTTP falso) |
| `front/test/notes_sync_duplicates_regression_test.dart:139` y siguientes | Regresiones de duplicados | Merge del listado con notas temporales | Widget |
| `front/test/local_notes_isolation_test.dart:43,79,107` | aislamiento / persistencia / logout | Filas de Drift separadas por dueño | Unitaria (Drift en memoria) |
| `front/test/notes_manual_sync_test.dart:106` | `1. todo synced: 0 PATCH, refresh, mensaje, sin abrir nota` | REFRESCAR termina con GET `/notes/me` | Widget |
