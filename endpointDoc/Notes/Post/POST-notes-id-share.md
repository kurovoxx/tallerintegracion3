# `POST /notes/:id/share`


## Req

- `POST /notes/:id/share`, JWT requerido: `back/notes/cmd/server/main.go:207`.
- id es UUID de la nota propia. JSON requerido: group_id (UUID) y access_mode (link o restricted). El handler limita el body a 1 MiB.
- No aceptar user_id de autor: la identidad se obtiene del JWT.

## Res

- `201 Created`: `{"shared_note_id":"<uuid>"}`. El share queda en PostgreSQL; un fallo al sincronizar permisos Drive después de persistirlo queda para convergencia y no cambia el `201`.
- `400 Bad Request`: JSON inválido, `group_id` inválido o `access_mode` ausente/no permitido; `bad_request` con `group_id y access_mode requeridos`.
- `401 Unauthorized`: JWT ausente/inválido/expirado; error `unauthorized`.
- `403 Forbidden`: usuario no propietario de la nota o no miembro del grupo; `forbidden` con `Acceso denegado`.
- `404 Not Found`: ID de nota inválido o nota ausente; `not_found` con `nota no encontrada`. Un UUID de grupo válido inexistente/no accesible no se distingue de no ser miembro y devuelve `403`.
- `413 Content Too Large`: body excede 1 MiB; `file_too_large` con `Archivo muy grande`.
- `500 Internal Server Error`: fallo DB o de la consulta a Social para miembros; `internal_error` con `Error interno`.
- `handleServiceError` define el mapeo (`back/notes/internal/handler/http/note_handler.go:890-941`) y `respondNotesError` el envelope `{"error":{"code":"...","message":"..."}}` (`back/notes/internal/handler/http/telemetry.go:56-61`).

## Invocación (Front / Otros Servicios)

- `_GroupNotesTabState._shareFlow` inicia el diálogo y la llamada de compartir (`front/lib/features/groups/group_notes_tab.dart:127-162`).
- `front/lib/core/services/notes_sharing_service.dart:224`, `shareNote` envía POST y utiliza shared_note_id para refrescar la vista.
- Para `restricted`, `NoteService.ComputeDesiredDrivePermissions` obtiene correos mediante `SocialGRPCAdapter.ListMemberEmails` (`back/notes/internal/service/note_service.go:2037-2086`; `back/notes/internal/service/social_grpc_adapter.go:111-121`); no es una llamada iniciada por Flutter.

## Flujo

1. `main()` registra `POST /notes/:id/share` bajo autenticación (`back/notes/cmd/server/main.go:207`).
2. `NoteHandler.Share` obtiene actor, valida ID/body y responde con el share o un error (`back/notes/internal/handler/http/note_handler.go:717-744`).
3. `NoteService.Share` toma el lock de la nota; `shareLocked` comprueba propiedad, pertenencia y modo (`back/notes/internal/service/note_service.go:1959-2002`). Para `restricted`, `SocialGRPCAdapter.IsMember` valida membresía y `ListMemberEmails` obtiene correos (`back/notes/internal/service/social_grpc_adapter.go:57-68,111-121`).
4. `ComputeDesiredDrivePermissions` calcula el ACL esperado (`back/notes/internal/service/note_service.go:2037-2086`). `PGSharedStore.Create` guarda el share y `SharedRepository.Create` persiste su estado `pending` (`back/notes/internal/service/store_pg.go:118-120`; `back/notes/internal/repository/shared_repository.go:72-88`).
5. `NoteService.syncDrivePermissions` aplica los permisos de Drive y persiste el resultado de convergencia (`back/notes/internal/service/note_service.go:2128-2295`). `NoteHandler.Share` devuelve `201` con el ID del share (`back/notes/internal/handler/http/note_handler.go:717-744`); Flutter lo usa para refrescar el grupo (`front/lib/core/services/notes_sharing_service.dart:224-252`; `front/lib/features/groups/group_notes_tab.dart:153-162`).

## Tests

- `back/notes/internal/handler/http/note_handler_test.go:386`, `TestHandlerShareAndGroupNotes`; :1063, TestHandlerShareRestrictedGrantsDrive.
- `back/notes/internal/service/note_service_test.go:470`, `TestShareAndAccessControl`; :529, autor incorrecto; :548, usuario no miembro; :1323, grants restricted; :1415, convergencia ante fallo parcial.
- Las pruebas de servicio usan Drive/Social simulados; no demuestran autorización con proveedores reales.
