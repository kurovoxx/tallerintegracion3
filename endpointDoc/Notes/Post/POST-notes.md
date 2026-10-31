# POST /notes

Crea un apunte: metadata en `notes.notes` y archivo Markdown en el Drive del autor. Ruta interna del backend Notes (`:8082`). Desde el navegador: `POST https://ti3-brojas.dev.censei.cl/api-notes/notes` (ver [prefijos](../COMUN-notes.md#1-rutas-internas-y-prefijos-externos)). Equivalente gRPC: `notes.v1.NotesService/CreateNote` ([doc](../Grpc/GRPC-notes-NotesService.md)).

## Req

- **Registro:** `back/notes/cmd/server/main.go:194` → `protected.POST("/notes", h.Create)`. Handler `back/notes/internal/handler/http/note_handler.go:169-196` (`Create`).
- **Autenticación:** `RequireAuth` (`main.go:192`; `back/notes/internal/middleware/auth_middleware.go:91-142`). JWT HS256 en `Authorization: Bearer <token>`. El autor es siempre el `user_id` del token. Sin roles. Detalle de 401/423/503 en [COMUN §2](../COMUN-notes.md#2-autenticación-http).
- **Parámetros de ruta/query:** ninguno.
- **Headers opcionales:** `X-Idempotency-Key` (`note_handler.go:187`). Vacía o de más de 128 caracteres = sin idempotencia (`back/notes/internal/service/note_service.go:109-119`).
- **Body JSON** (`createRequest`, `note_handler.go:134-139`), límite 1 MiB (`L178`):

| Campo | Tipo | Obligatorio | Validación |
| --- | --- | --- | --- |
| `title` | string | sí | Binding `required,max=300` (runas). Luego el servicio recorta espacios, cambia `/` y `\` por `_` y exige 1–255 **bytes** (`len`) (`note_service.go:705,719-724`; `utils/validator.go:9-12`) |
| `visibility` | string | sí | Binding `oneof=public private` |
| `subject_id` | string \| null | no | Binding `omitempty,uuid`. `""` se trata como sin materia (`note_service.go:734-741`). Sin FK: no se comprueba que la materia exista |
| `content` | string \| null | no | Máx. 1 048 576 bytes (`maxNoteContentLength`, `note_service.go:280,711-713`). Se sanitiza contra XSS (`sanitizeMarkdown`, `L86-104`, `L714-716`). Si falta, se usa `"# {title}\n\n"` (`L743-748`) |

Ejemplo:

```http
POST /notes
Authorization: Bearer <jwt>
X-Idempotency-Key: flutter-note-create-1730380000000000
Content-Type: application/json

{"title":"Álgebra","visibility":"private","content":"# Matrices\n","subject_id":"6b0c1f0e-2f4a-4c1e-9d7a-3a1b2c3d4e5f"}
```

**Visibilidad public/private:** se fija aquí. `public` da lectura a cualquier usuario autenticado de la app y además crea un permiso Drive `anyone`/`reader`. `private` solo deja leer al autor hasta que se comparta con un grupo. Se cambia después con [PATCH /notes/:id](../Patch/PATCH-notes-id.md).

## Res

**201 Created**

```json
{"note_id":"11111111-1111-4111-8111-111111111111"}
```

- `note_id`: UUID de la nota creada (`note_handler.go:195`). Es el único campo. No devuelve `version`, `title` ni `external_file_id`.
- Un replay con la misma `X-Idempotency-Key` y el mismo payload devuelve el mismo `note_id` con 201, sin tocar Postgres ni Drive (`note_service.go:760-769`).

**Errores**

| HTTP | `error.code` | Condición |
| --- | --- | --- |
| 400 | `bad_request` "Request inválido" | JSON mal formado, falta `title`/`visibility`, `title` > 300 runas, `visibility` fuera de `public`/`private`, `subject_id` no UUID (`note_handler.go:179-186`) |
| 400 | `invalid_title` "Título requerido (1-255 caracteres)" | Título vacío tras recortar o > 255 bytes (`note_service.go:705-707,722-724`) |
| 403 | `forbidden` "Conecte o renueve su Google Drive" | El autor no tiene Drive conectado o el token fue revocado (`L874-877`) |
| 409 | `conflict` "Conflicto" / "solicitud en progreso" / "solicitud en recuperación" | Misma clave con otro payload, o la misma clave aún en curso (`beginIdempotentOperation`, `L172-231`) |
| 413 | `file_too_large` "Archivo muy grande" | Body > 1 MiB (`note_handler.go:180-182`), `content` > 1 MiB (`note_service.go:711-713`) o Drive responde 413 (`L879-882`) |
| 500 | `internal_error` "Error interno" | Fallo de Postgres (`ErrInternalDatabase`) |
| 502 | `internal_error` "Servicio de almacenamiento no disponible" | Drive caído, timeout, o fallo al otorgar el enlace público (`L883`, `L895-903`) |
| 401 / 423 / 503 | ver [COMUN §2](../COMUN-notes.md#2-autenticación-http) | Middleware |

**Efectos antes de responder 201** (todo síncrono):

1. Si hay clave: fila en `notes.idempotency_keys` reclamada con el `noteID` preasignado (`L754-790`).
2. Fila en `notes.notes` con `sync_status='pending_drive'` (`L802-807`).
3. Archivo `{title}.md` en la carpeta `Apuntes TI3` del Drive del autor (`L836-886`; `drive/real_client.go:127-150`).
4. Solo si `visibility=public`: permiso Drive `anyone`/`reader` (`allowFileDiscovery=false`) y fila `anyone` `in_sync` en `notes.drive_managed_permissions` (`note_service.go:891-914`). No se crean permisos por correo al crear.
5. `external_file_id` y `sync_status='synced'` en `notes.notes` (`L917`; `repository/note_repository.go:208-220`).
6. Clave de idempotencia marcada `completed` (`note_service.go:946-951`).

**Fallos parciales y trabajo en segundo plano:**

- Rechazo definitivo de Drive (OAuth, 4xx, 413): se borra la fila recién creada y se libera la clave (`L857-868`). No queda nota.
- Fallo ambiguo (timeout, red, 5xx): la fila queda `failed_sync` y la clave `recoverable` (`L869-873`, `L780-789`). Un reintento con la misma clave retoma la misma nota. El reconciliador (cada 30 s) busca el archivo huérfano por `appProperties` pasados 15 min (`L2840-2878`).
- Fallo de Postgres tras crear el archivo: se intenta borrar el archivo y, si falla, se encola `delete_file` en `notes.drive_reconciliation_queue` (`L917-939`).

## Invocación (Front / Otros Servicios)

- **Pantalla:** `AllNotesScreen` (`front/lib/features/notes/all_notes_screen.dart:138`). Botón **"Nueva nota"** en desktop (`L2915-2922`, `onPressed: _showCreateForSubject`); en móvil el comentario de `L2911-2913` indica que lo aporta el FAB del shell. Abre `_showCreateDialog` (`L2579`) con `_CreateNoteDialog` (`L4970`), selector **PÚBLICO/PRIVADO** (`L5173`, `L5181`; por defecto `private`, `L4993`) y botón **GUARDAR** (`L5084`).
- **Acción → petición:** `onCreate` (`L2584`) guarda primero en Drift (`_repo!.upsertNote`, `L2619`) y luego `_completeRemoteCreation` (`L2662`, `L2682`) → `_createLocalNoteIdempotent` (`L1331`) → `_createNoteOnBackend` (`L1145-1179`, deduplica peticiones en vuelo por clave) → `_postCreate` (`L1181-1242`).
- **Petición:** `POST $notesBaseUrl/notes` (`L1188`) con `_authHeaders()` (`L1540-1550`: `Authorization: Bearer ${SessionManager.token}`), `X-Idempotency-Key: flutter-note-create-<tempId>` (`L1206-1210`, `L1246`) y body `{title, content, visibility, subject_id?}` (`L1189-1195`). Timeout 45 s (`L85`). `AuthedHttp.run` reintenta una vez tras refrescar el token en un 401 (`front/lib/core/services/authed_client.dart:77-89`).
- **Respuesta:** con 201/200 valida que `note_id` sea UUID y lo añade a `_ownedNoteIds` (`L1217-1229`); lee `data['version']`, que el backend no envía, y usa 1 por defecto (`L1226`). Luego promueve la fila temporal de Drift al UUID real (`_replaceLocalId` `L1354`, `promoteTempNote` `L1371`; `front/lib/core/database/local_notes_repository.dart:80`). Con 409 no crea otra nota: reintenta más tarde con la misma clave (`L1231-1237`).
- **Otros disparadores del mismo POST:** `_resolvePendingCreates` durante la sincronización (`L689`, llamado en `L592`) y `_settleDeletedPendingCreate` (`L1522-1538`).
- **`NotesApi.createNote`** (`front/lib/features/notes/notes_api.dart:152-177`): solo lo usa `front/test/cross_service_e2e_test.dart:149`. No tiene consumidor de producción.
- **Otros microservicios:** No se encontró invocación en el código revisado.
- **Tests/scripts:** `back/social/test/cross_service_e2e_test.go:722` (tag `crossservice`); `scripts/test-notes-e2e.py:93,97`.

## Flujo

1. `main.go:194` registra la ruta → `RequireAuth` (`auth_middleware.go:91`) valida JWT y gate de cuenta.
2. `NoteHandler.Create` (`note_handler.go:169`): lee `user_id`, aplica límite de 1 MiB, bind de `createRequest`, lee `X-Idempotency-Key`.
3. `NoteService.Create` (`note_service.go:701`): valida título/materia/tamaño, sanitiza, normaliza título, contenido por defecto.
4. Idempotencia: `beginIdempotentOperation` (`L172`) → `PGNoteStore.GetOrClaimIdempotencyKey` (`service/store_pg.go:215`) → `NoteRepository.GetOrClaimIdempotencyKey` (`repository/note_repository.go:265`).
5. `PGNoteStore.Create` (`store_pg.go:22`) → `NoteRepository.Create` (`note_repository.go:39`): INSERT `pending_drive`.
6. Drive (síncrono): `RealDriveClient.CreateFile` (`drive/real_client.go:127`) con token del autor pedido a Identity (`drive/oauth_grpc.go:62`).
7. Si es pública: `GrantLinkPermission` (`real_client.go:439`) + `persistManagedPermission` (`note_service.go:2090`).
8. `UpdateExternalFileID` (`note_repository.go:208`) → `synced`; relectura `GetByID`; `completeIdempotentOperation` (`note_service.go:263`).
9. `handleServiceError` (`note_handler.go:890`) traduce errores; en éxito `c.JSON(201, {"note_id"})` (`L195`).

Finalmente devuelve `201 {"note_id"}`, y la pantalla `AllNotesScreen` utiliza ese resultado para reemplazar el id temporal de la nota en Drift por el UUID del servidor y marcarla como propia.

## Tests

| Archivo:línea | Prueba | Qué comprueba | Tipo |
| --- | --- | --- | --- |
| `back/notes/cmd/server/router_contract_test.go:15` | `TestHTTPRouterContract` | La ruta existe y sin token responde 401 | Unitaria (router) |
| `back/notes/internal/handler/http/note_handler_test.go:86` | `TestHandlerCreateSuccess` | 201 con `note_id` | Unitaria (stores memoria + Drive mock) |
| `note_handler_test.go:108` | `TestHandlerCreateValidation400` | Validación de body → 400 | Unitaria |
| `note_handler_test.go:131` | `TestHandlerCreateUnauthorized401` | Sin token → 401 | Unitaria |
| `note_handler_spec95_test.go:166` | `TestHandlerCreateIdempotencyReplayAndConflict` | Replay misma clave y 409 con otro payload | Unitaria |
| `audit_fixes_test.go:309` | `TestHandlerCreateInverseOrphanCompensation` | Compensación del archivo huérfano si falla PG | Unitaria |
| `audit_fixes_test.go:362` | `TestHandlerJSONPayloadTooLarge413` | Body > 1 MiB → 413 | Unitaria |
| `audit_fixes_test.go:379` | `TestHandlerOAuthErrorNoLeak` | 403 OAuth sin filtrar detalle | Unitaria |
| `back/notes/internal/service/note_service_test.go:30,60,93` | `TestCreateSuccess`, `TestCreateValidations`, `TestCreateRollbackOnDBFailure` | Reglas de servicio | Unitaria |
| `note_service_test.go:946` | `TestDriveCreateSyncsPostgresAndDrive` | Fila `synced` + archivo en Drive mock | Unitaria |
| `service/phantom_note_test.go:57,108` | `TestCreateDefinitiveDriveFailureDoesNotLeavePhantomNote`, `TestCreateAmbiguousDriveFailureKeepsRecoverableRow` | Fallo definitivo borra la fila; ambiguo la deja `failed_sync` | Unitaria |
| `service/create_replay_expiry_test.go:18,58` | `TestCreateRetryAfterReplayWindowAdoptsExistingNote`, `TestCreateConcurrentRetriesSameKeyCreateOneNote` | Idempotencia tras expirar y concurrencia | Unitaria |
| `service/rollback_test.go:532,565,591,621,738,777` | `TestIdempotencyCreate*` | Replay, hash distinto, claim en curso/expirado, recuperación tras crash | Unitaria |
| `rollback_test.go:926` | `TestCreateAndUpdateSanitizeContent` | Sanitización XSS | Unitaria |
| `service/drive_permissions_test.go:303` | `TestPublicCreateAndLinkShareFailWhenGrantFails` | Pública: si falla el permiso `anyone`, falla la creación | Unitaria |
| `back/notes/internal/repository/idempotency_integration_test.go:16,77` | `TestIdempotencyKeyClaimCompleteAndReclaim`, `TestIdempotencyPreassignedResourceID` | Claves en Postgres real | Integración (`NOTES_TEST_DATABASE_URL`) |
| `back/notes/internal/handler/grpc/server_test.go:223` | `TestCreateNotePersistsAndReturnsID` | Mismo servicio vía gRPC (bufconn) | Unitaria gRPC |
| `front/test/notes_create_sync_test.dart:211,249,323,350` | tests 7, 8–14, 13, 15 | Flujo de creación y sincronización en la pantalla (HTTP falso) | Widget |
| `front/test/notes_sync_duplicates_regression_test.dart:240` | `cuentas con igual clave no comparten el POST en vuelo` | Deduplicación de POST en vuelo | Widget |
