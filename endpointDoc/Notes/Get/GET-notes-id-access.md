# `GET /notes/:id/access`


## Req

- `GET /notes/:id/access`, protegido por JWT: `back/notes/cmd/server/main.go:199`.
- `Authorization`: Bearer <JWT>; id es el UUID de la nota. Sin body ni query.
- Ruta interna Notes; el gateway externo puede usar `/api-notes`, no es parte del path del servidor.

## Res

- `200 OK`: `NoteAccessResponse` incluye `can_read`, `can_write`, `visibility`, `access_mode`, `drive_sync_status`, `drive_access_verified`, `drive_connection_required` y `drive_url` nullable (`back/notes/internal/model/note.go:6-15`). La autorización se calcula por propietario, visibilidad y shares; OAuth faltante se refleja en `drive_connection_required`.
- `401 Unauthorized`: JWT ausente/inválido/expirado; `{"error":{"code":"unauthorized","message":"No autorizado: token ausente, inválido o expirado"}}`.
- `404 Not Found`: ID inválido, nota inexistente o acceso no concedido; `{"error":{"code":"not_found","message":"nota no encontrada"}}`. Si la nota no tiene archivo remoto, devuelve `note_unavailable` con `Nota no disponible en almacenamiento remoto`.
- `500 Internal Server Error`: fallo PostgreSQL o de la consulta de membresía Social; `{"error":{"code":"internal_error","message":"Error interno"}}`.
- `handleServiceError` aplica los estados (`back/notes/internal/handler/http/note_handler.go:890-941`); `respondNotesError` forma el envelope (`back/notes/internal/handler/http/telemetry.go:56-61`). No entrega Markdown ni modifica datos.

## Invocación (Front / Otros Servicios)

- No se encontró invocación HTTP en `front/lib` ni un consumidor HTTP de producción en otros servicios. Las referencias encontradas son tests.
- Notes expone también el RPC GetNoteAccess; no se encontró un cliente de producción que sustituya esta ruta HTTP.

## Flujo

1. `main()` registra `GET /notes/:id/access` bajo JWT (`back/notes/cmd/server/main.go:199`).
2. `NoteHandler.GetAccess` toma requester y note ID y serializa la respuesta o error (`back/notes/internal/handler/http/note_handler.go:828-856`).
3. `NoteService.GetAccess` obtiene los metadatos de la nota; `PGNoteStore.GetByID` delega a `NoteRepository.GetByID` (`back/notes/internal/service/note_service.go:2554-2564`; `back/notes/internal/service/store_pg.go:25-27`; `back/notes/internal/repository/note_repository.go:72-85`).
4. Para notas privadas, `SharedRepository.ListByNote` recupera shares y `SocialGRPCAdapter.IsMember` comprueba las membresías restricted (`back/notes/internal/service/note_service.go:2574-2605`; `back/notes/internal/repository/shared_repository.go:107-125`; `back/notes/internal/service/social_grpc_adapter.go:57-68`).
5. `NoteService.GetAccessInfo` deriva el estado de permisos desde `SharedRepository.ListManagedPermissions`; `driveAccessFlags` solo verifica acceso remoto para el dueño de una nota no pública (`back/notes/internal/service/note_service.go:2618-2632,2635-2669,2687-2703`; `back/notes/internal/repository/shared_repository.go:296-320`). La ruta devuelve metadatos y URL, no descarga contenido.
6. `NoteHandler.GetAccess` devuelve `NoteAccessResponse` (`back/notes/internal/handler/http/note_handler.go:828-856`). No se encontró consumidor HTTP de producción en Flutter ni otro servicio.

## Tests

- `back/notes/internal/handler/http/note_handler_test.go:922`, `TestHandlerGetAccessAuthor200`; :958, not found; :972, cero conocimiento; :1004, flujo restricted.
- `back/notes/internal/service/access_contract_test.go:12`, `TestGetAccessTypedContract`; :68, estado de sincronización; :126, conexión Drive requerida solo para propietario; :248, lector no verifica Drive.
- Estas pruebas usan dobles para dependencias externas; no cubren la evaluación de permisos con proveedores reales.
