# gRPC notes.v1.NotesService

Servicio gRPC de Notes. Corre en paralelo al HTTP ("dual-serve") sobre el **mismo** `*service.NoteService`, así que reglas, locks, idempotencia y efectos en Drive son los mismos que en HTTP. Puerto `:9082` (`NOTES_GRPC_PORT` → `GRPC_PORT` → `9082`, `back/notes/internal/config/config.go:77,114-116`). Solo accesible dentro del cluster/Compose: no pasa por nginx ni por el Ingress ([prefijos](../COMUN-notes.md#1-rutas-internas-y-prefijos-externos)).

Este documento cubre los 9 RPCs de **usuario** (autenticados con JWT). Los 3 RPCs **internos** tienen documento propio:

- [UnshareAll](GRPC-notes-UnshareAll.md) — Social → Notes, `x-internal-key`.
- [BeginAccountDeletion](GRPC-notes-BeginAccountDeletion.md) — Auth → Notes, `x-internal-key`.
- [AccountDeletionCleanup](GRPC-notes-AccountDeletionCleanup.md) — Auth → Notes, `x-internal-key`.

Las llamadas que Notes hace **hacia** Social e Identity están en [GRPC-notes-clientes-salientes.md](GRPC-notes-clientes-salientes.md).

## Req

- **Contrato:** `back/proto/notes/v1/notes.proto` (`package notes.v1`, `L34`; `service NotesService`, `L42-110`). Módulo buf propio (`back/proto/notes/buf.yaml`), se regenera con `back/proto/notes/generate.sh` hacia `back/notes/internal/gen/notesv1/v1/` (`buf.gen.yaml`). Auth y Social tienen copias generadas en `back/auth/internal/gen/notesv1/` y `back/social/internal/gen/notesv1/`. No vive en `back/proto/gen/go/` y no importa `common.v1` (duplica `PageRequest`/`PageResponse` a propósito, `notes.proto:189-207`).
- **Registro:** `back/notes/cmd/server/main.go:233-240` → `grpc.NewServer(ChainUnaryInterceptor(NewLogging, NewAuthRouter(NewJWTAuth(validator, deletionGate), NewInternalAuth(INTERNAL_API_KEY))))` y `RegisterNotesServiceServer(grpcSrv, NewNotesServer(svc))`. Implementación `back/notes/internal/handler/grpc/server.go`. Sin reflexión: `grpcurl` necesita el `.proto`.
- **Interceptores** (`back/notes/internal/interceptor/`):
  - `NewLogging` (`logging.go:19`): registra cada RPC, incluidos los rechazos de auth.
  - `NewAuthRouter` (`auth.go:99-108`): `UnshareAll`, `BeginAccountDeletion` y `AccountDeletionCleanup` van a `NewInternalAuth`; el resto a `NewJWTAuth`.
  - `NewJWTAuth` (`auth.go:58-94`): metadata `authorization: Bearer <JWT>`, mismo validador HS256 que HTTP (`iss=apuntes-auth`, `aud=apuntes-client`). Publica `user_id` en el contexto (`middleware.WithUserID`). Con Postgres aplica el gate de borrado de cuenta.
- **Identidad:** el `user_id` del llamante sale siempre del JWT (`caller`, `server.go:34-40`); ningún mensaje de usuario lo lleva como campo.

| RPC | `.proto` | Request (campos) | Servicio | Equivalente HTTP |
| --- | --- | --- | --- | --- |
| `CreateNote` | `L48`, req `L210-221` | `title`, `subject_id?`, `visibility`, `content?`, `idempotency_key?` | `Create` (`server.go:42-52`) | [POST /notes](../Post/POST-notes.md) |
| `GetNote` | `L53`, req `L229-232` | `id` | `Get` (`L54-64`) | [GET /notes/{id}](../Get/GET-notes-id.md) |
| `ListMyNotes` | `L57`, req `L240-242` | `page {page_size, page_token}` | `ListMy` (`L66-83`) | [GET /notes/me](../Get/GET-notes-me.md) |
| `UpdateNote` | `L63`, req `L251-263` | `id`, `title?`, `visibility?`, `content?`, `expected_version`, `idempotency_key?` | `UpdateWithExpectedVersion` (`L85-95`) | [PATCH /notes/{id}](../Patch/PATCH-notes-id.md) |
| `DeleteNote` | `L67`, req `L272-274` | `id` | `Delete` (`L97-106`) | [DELETE /notes/{id}](../Delete/DELETE-notes-id.md) |
| `ShareNote` | `L75`, req `L279-286` | `note_id`, `group_id`, `access_mode` | `Share` (`L108-118`) | [POST /notes/{id}/share](../Post/POST-notes-id-share.md) |
| `UnshareNote` | `L80`, req `L294-297` | `shared_note_id` | `Unshare` (`L120-129`) | [DELETE /notes/shared/{id}](../Delete/DELETE-notes-shared-sharedNoteId.md) |
| `ListGroupNotes` | `L86`, req `L302-306` | `group_id`, `page` | `ListGroupNotePairs` (`L131-151`) | [GET /groups/{id}/notes](../Get/GET-groups-id-notes.md) |
| `GetNoteAccess` | `L92`, req `L314-316` | `id` | `GetAccess` (`L153-163`) | [GET /notes/{id}/access](../Get/GET-notes-id-access.md) |

Diferencias de entrada con HTTP:

- Los opcionales usan presencia proto3 (`optional`).
- `UpdateNote.expected_version`: `> 0` exige esa versión; `0` usa la versión leída bajo el lock (`notes.proto:258-260`). En HTTP es el campo `version`.
- `CreateNote.visibility` vacío se convierte en `private` en el servicio (`back/notes/internal/service/note_service.go:725-727`); en HTTP es obligatorio por binding.
- Sin validación de binding: los límites que en HTTP pone Gin (p. ej. `title` ≤ 300, `oneof`) aquí solo los aplica el servicio. No hay límite de 1 MiB del body HTTP, pero el servicio sigue rechazando `content` > 1 MiB.
- `page_size` fuera de 1–100 se normaliza en el servicio (`ListMy` `note_service.go:1014-1019`); `page_token` es el mismo cursor opaco que HTTP.
- Save, Copy, Like/Unlike, adjuntos, upload y reconcile **no** tienen RPC.

Ejemplo (`grpcurl`, Compose local):

```sh
grpcurl -plaintext -import-path back/proto/notes -proto v1/notes.proto \
  -H "authorization: Bearer $TOKEN" \
  -d '{"title":"Álgebra","visibility":"private","content":"# Matrices","idempotencyKey":"create-demo-1"}' \
  localhost:9082 notes.v1.NotesService/CreateNote
# {"noteId":"<uuid>"}
```

## Res

| RPC | Respuesta |
| --- | --- |
| `CreateNote` | `CreateNoteResponse{note_id}` (`L224-226`) |
| `GetNote` | `GetNoteResponse{note}` con `content` y `drive_url`; **sin adjuntos** (`L235-237`) |
| `ListMyNotes` | `notes[]` + `page{next_page_token}` (`L245-248`) |
| `UpdateNote` | `UpdateNoteResponse{note}`: la entidad completa, no el subconjunto de HTTP (`L267-269`) |
| `DeleteNote`, `UnshareNote` | Mensaje vacío |
| `ShareNote` | `ShareNoteResponse{shared_note_id}` (`L289-291`) |
| `ListGroupNotes` | `notes[]` de `NoteWithShare{note, share}` + `page` (`L308-311`). A diferencia de HTTP **sí incluye** el `SharedNote` (con su `id`, necesario para `UnshareNote`) |
| `GetNoteAccess` | `GetNoteAccessResponse{access}` (`L318-320`) |

Mensajes (`toProtoNote`, `toProtoSharedNote`, `toProtoAccess`, `server.go:194-250`):

- `Note` (`notes.proto:121-148`): `id`, `user_id`, `subject_id?`, `title`, `external_file_id?`, `visibility`, `likes_count` (int64), `forked_from_note_id?`, `sync_status` (`synced`|`pending_drive`|`failed_sync`), `version`, `created_at`, `updated_at` (RFC3339 UTC; `""` si cero, `server.go:254-259`), `content?`, `drive_url?`.
- `SharedNote` (`L151-165`): `id`, `note_id`, `group_id`, `is_admin_note`, `access_mode`, `author_followers_snapshot`, `permission_sync_status` (`pending`|`in_sync`|`failed`), `shared_at`.
- `NoteAccess` (`L174-187`): `can_read`, `can_write`, `visibility`, `access_mode` (`owner`|`public`|`link`|`restricted`), `drive_sync_status` (`synced`|`pending`|`failed`), `drive_access_verified`, `drive_connection_required`, `drive_url?`.

**Códigos de estado** (`mapError`, `back/notes/internal/handler/grpc/errors.go:18-55`; `mapCode`, `L60-82`):

| Código gRPC | Origen |
| --- | --- |
| `UNAUTHENTICATED` | Sin metadata, sin `authorization`, formato no `Bearer`, token vacío o inválido/expirado (`auth.go:60-79`); `user_id` ausente en contexto (`server.go:37`) |
| `PERMISSION_DENIED` | OAuth de Drive (`errors.go:23-25`); `forbidden` del servicio; cuenta en borrado (`auth.go:87-89`) |
| `UNAVAILABLE` | `ErrDriveUnavailable` o `DriveError` no 403/404/413; gate de cuenta no consultable (`auth.go:84-86`) |
| `NOT_FOUND` | `not_found`, `note_unavailable`, `ErrSourceAccessDenied`, `DriveError` 403/404 |
| `ABORTED` | `conflict` (versión, idempotencia) |
| `ALREADY_EXISTS` | `already_saved`, `already_liked` (sin RPC que hoy los produzca) |
| `RESOURCE_EXHAUSTED` | `rate_limited` (sin RPC que hoy lo produzca) |
| `INVALID_ARGUMENT` | `bad_request`, `invalid_*`, `file_too_large`, `payload_too_large`, `unsupported_media_type`; `DriveError` 413 |
| `INTERNAL` | `ErrInternalDatabase`, `ErrInternalServer`, errores no tipados |

- El status lleva solo el mensaje humano del servicio; no hay `details` estructurados ni `code` snake_case garantizado. El comentario de `notes.proto:14-28` dice que "el detalle snake_case viaja en el status", pero `mapError` solo pone el mensaje.
- Efectos en Postgres/Drive: idénticos a su equivalente HTTP (ver cada documento).

## Invocación (Front / Otros Servicios)

- **Frontend Flutter:** No se encontró invocación en el código revisado. El front no usa gRPC; consume las rutas HTTP equivalentes por `/api-notes`.
- **Otros microservicios:** No se encontró invocación en el código revisado de ninguno de estos 9 RPCs. Los únicos clientes de producción de `NotesService` usan los RPC internos: Social (`UnshareAll`) y Auth (`BeginAccountDeletion`, `AccountDeletionCleanup`).
- **Tests:** `back/notes/internal/handler/grpc/server_test.go`, `list_my_contract_test.go` y `back/notes/test/e2e_social_notes_test.go` (todos con bufconn).

## Flujo

1. `main.go:233-240`: `NewLogging` → `NewAuthRouter` → `NewJWTAuth` (valida token, gate de cuenta, publica `user_id`).
2. `NotesServer.<RPC>` (`server.go`): `caller(ctx)` → método de `NoteService` → `mapError` en caso de error → `toProto*`.
3. `NoteService` → stores PG (`back/notes/internal/service/store_pg.go`) → repositorios pgx → Drive (`back/notes/internal/drive/real_client.go`) y Social gRPC cuando aplica. Mismo recorrido que el HTTP equivalente.

Finalmente devuelve el mensaje de respuesta del RPC, y hoy solo los tests utilizan ese resultado para comprobar que la semántica gRPC coincide con la HTTP.

## Tests

| Archivo:línea | Prueba | Qué comprueba | Tipo |
| --- | --- | --- | --- |
| `back/notes/internal/handler/grpc/server_test.go:147,153,160` | `TestUserRPCWithoutAuthorizationIsUnauthenticated`, `TestUserRPCWithInvalidTokenIsUnauthenticated`, `TestUserRPCWithValidTokenPropagatesUserID` | Interceptor JWT | Unitaria gRPC (bufconn, stores memoria, Drive mock) |
| `server_test.go:223` | `TestCreateNotePersistsAndReturnsID` | `CreateNote` | Unitaria gRPC |
| `server_test.go:254` | `TestGetNoteForeignAndMissingAreNotFound` | `GetNote` → `NOT_FOUND` para ajena/inexistente | Unitaria gRPC |
| `server_test.go:271,298` | `TestUpdateNoteOptimisticVersionConflict`, `TestUpdateNoteByStrangerIsNotFound` | `UpdateNote` → `ABORTED` / `NOT_FOUND` | Unitaria gRPC |
| `server_test.go:313,330` | `TestDeleteNoteRemovesIt`, `TestDeleteNoteByStrangerIsNotFound` | `DeleteNote` | Unitaria gRPC |
| `server_test.go:345,382` | `TestShareNoteLinkAndRestrictedThenUnshare`, `TestShareNoteByNonMemberIsPermissionDenied` | `ShareNote`/`UnshareNote` | Unitaria gRPC |
| `server_test.go:399,461` | `TestListGroupNotesPaginationAndOrdering`, `TestListGroupNotesNonMemberIsPermissionDenied` | `ListGroupNotes` | Unitaria gRPC |
| `server_test.go:480` | `TestGetNoteAccessResolvesWithoutDownloading` | `GetNoteAccess` sin leer contenido | Unitaria gRPC |
| `back/notes/internal/handler/grpc/list_my_contract_test.go:9` | `TestListMyNotesPaginationAndIsolation` | `ListMyNotes` | Unitaria gRPC |
| `back/notes/test/e2e_social_notes_test.go:396,427` | `TestE2EShareFailClosedWhenSocialDown`, `TestE2EShareNotMemberIsPermissionDenied` | `ShareNote` contra un Social HTTP falso | E2E (bufconn) |

No existen archivos de test en `back/notes/internal/interceptor/` (el documento anterior los mencionaba). El enrutado del interceptor se prueba indirectamente en `server_test.go` y en el E2E.
