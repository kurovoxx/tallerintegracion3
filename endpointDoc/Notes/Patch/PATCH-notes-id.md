# `PATCH /notes/:id`


## Req

- `PATCH /notes/:id`, JWT requerido: `back/notes/cmd/server/main.go:208`.
- JSON parcial: title (1–300 caracteres), visibility (public/private), content (opcional) y version opcional positiva. Debe venir al menos un campo editable; límite del body: 1 MiB.
- Header opcional `X-Idempotency-Key` permite reintento seguro del mismo cambio. La identidad/autor procede del JWT.

## Res

- `200 OK`: `{"id":"<uuid>","title":"...","visibility":"private","version":3,"updated_at":"<RFC3339 UTC>"}`.
- `400 Bad Request`: JSON inválido o nada que actualizar devuelve `bad_request` con `Request inválido` o `nada que actualizar`; título o visibilidad inválidos devuelven `invalid_title`/`Título requerido (1-255 caracteres)` o `invalid_visibility`/`Visibilidad debe ser public o private`.
- `401 Unauthorized`: JWT ausente/inválido/expirado; `unauthorized` con `No autorizado: token ausente, inválido o expirado`.
- `404 Not Found`: ID inválido, nota ausente o de otra cuenta devuelve `not_found` con `nota no encontrada`; archivo Drive no disponible devuelve `note_unavailable` con `Nota no disponible en almacenamiento remoto`.
- `409 Conflict`: versión obsoleta, clave idempotente con otro payload o operación en curso; `conflict` con `Conflicto` o el mensaje específico `solicitud en progreso`.
- `413 Content Too Large`: body supera 1 MiB o el contenido supera su límite; `file_too_large` con `Archivo muy grande`.
- `403 Forbidden`: OAuth Drive requiere conexión/renovación; `forbidden` con `Conecte o renueve su Google Drive`.
- `502 Bad Gateway`: Drive no disponible; `internal_error` con `Servicio de almacenamiento no disponible`. `500 Internal Server Error`: fallo DB u otro error no tipado; `internal_error` con `Error interno`.
- Los errores usan `{"error":{"code":"...","message":"..."}}`. `handleServiceError` aplica el mapeo (`back/notes/internal/handler/http/note_handler.go:890-941`) y `StatusForCode` asigna status (`back/notes/internal/utils/response.go:88-105`). La versión enviada es precondición optimista; Drive puede quedar pendiente de reconciliación.

## Invocación (Front / Otros Servicios)

- `AllNotesScreen._patchNoteRemote` construye y envía PATCH con versión e `X-Idempotency-Key`, y guarda la versión devuelta (`front/lib/features/notes/all_notes_screen.dart:1678-1718`).
- Las acciones de guardado llaman al método y usan su resultado para actualizar la nota o mantener el error (`front/lib/features/notes/all_notes_screen.dart:765-805,1933-1948`).
- No se encontró consumidor backend de producción.

## Flujo

1. `main()` registra `PATCH /notes/:id` bajo autenticación (`back/notes/cmd/server/main.go:208`).
2. `NoteHandler.Patch` obtiene actor/ID, valida body y versión, lee `X-Idempotency-Key` y llama al servicio (`back/notes/internal/handler/http/note_handler.go:308-354`).
3. `NoteService.UpdateWithExpectedVersion` aplica idempotencia y delega el cambio (`back/notes/internal/service/note_service.go:1052-1082`); `updateInnerLocked` verifica autor/versión y coordina Drive con el guardado PostgreSQL (`back/notes/internal/service/note_service.go:1106-1120,1265-1310`). `PGNoteStore.Update` delega a `NoteRepository.Update` (`back/notes/internal/service/store_pg.go:31-33`; `back/notes/internal/repository/note_repository.go:136-181`).
4. El handler devuelve `200` con la nueva versión (`back/notes/internal/handler/http/note_handler.go:342-353`). `AllNotesScreen._patchNoteRemote` incorpora esa versión para evitar sobrescribir una edición posterior (`front/lib/features/notes/all_notes_screen.dart:1712-1718`).

## Tests

- `back/notes/internal/handler/http/note_handler_test.go:591`, `TestHandlerPatchVersionConflict`.
- `back/notes/internal/handler/http/note_handler_spec95_test.go:31`, `replay` idempotente; :86, actualización sin key; :114, claves distintas y limpieza de errores.
- `back/notes/internal/handler/http/note_postgres_edit_test.go` contiene la integración con versionado optimista y PostgreSQL.
