# PATCH /notes/{id}

Edita título, visibilidad y/o contenido de una nota propia con bloqueo optimista por versión. El contenido se escribe en Drive; título y visibilidad en Postgres. Ruta interna del backend Notes (`:8082`). Desde el navegador: `PATCH https://ti3-brojas.dev.censei.cl/api-notes/notes/<id>` ([prefijos](../COMUN-notes.md#1-rutas-internas-y-prefijos-externos)). Equivalente gRPC: `NotesService/UpdateNote`.

## Req

- **Registro:** `back/notes/cmd/server/main.go:208` → `protected.PATCH("/notes/:id", h.Patch)`. Handler `back/notes/internal/handler/http/note_handler.go:308-354` (`Patch`).
- **Autenticación:** `RequireAuth` (`main.go:192`). Solo el autor; para otro usuario la nota "no existe" (404) (`back/notes/internal/service/note_service.go:1114-1116`). Ver [COMUN §2](../COMUN-notes.md#2-autenticación-http).
- **Path:** `id` UUID obligatorio (no UUID → 404 `not_found`).
- **Header opcional:** `X-Idempotency-Key` (`note_handler.go:336`). Mismo body y misma clave dentro de 24 h → devuelve el resultado ya aplicado; misma clave con otro body → 409 (`note_service.go:1056-1081`).
- **Body JSON** (`patchRequest`, `note_handler.go:299-306`), límite 1 MiB. Debe venir al menos uno de `title`, `visibility`, `content` (`L329-332`).

| Campo | Tipo | Obligatorio | Validación |
| --- | --- | --- | --- |
| `title` | string | no | Binding `min=1,max=300`; luego recorte, `/`/`\` → `_`, 1–255 bytes (`note_service.go:1127-1135`) |
| `visibility` | string | no | Binding `oneof=public private` |
| `content` | string | no | Máx. 1 048 576 bytes (`L1053-1055`); se sanitiza (`L1123-1125`) |
| `version` | entero | no | Binding `min=1`. Si viene, la nota debe estar en esa versión (`L1119-1121`). Si no viene, se usa la versión leída bajo el lock (`L1295-1301`) |

```json
{"title":"Álgebra lineal","content":"# Matrices\n\nDeterminantes","version":3}
```

**Visibilidad public/private en edición:** cambiar `visibility` actualiza `notes.notes.visibility` y recalcula los permisos de Drive: pasar a `public` otorga `anyone`/`reader`; pasar a `private` revoca `anyone` salvo que algún share `link` lo siga necesitando. Los permisos por correo de grupos `restricted` se conservan (`L1263-1291`, `L1333-1337`; ver [COMUN §5](../COMUN-notes.md#5-ciclo-de-vida-de-los-permisos-de-drive)).

## Res

**200 OK** (`note_handler.go:347-353`)

```json
{"id":"11111111-1111-4111-8111-111111111111","title":"Álgebra lineal","visibility":"private","version":4,"updated_at":"2026-10-23T09:30:00Z"}
```

- `id`, `title`, `visibility`: valores tras el cambio.
- `version`: nueva versión (se incrementa en el UPDATE, `back/notes/internal/repository/note_repository.go:131-182`). El cliente debe enviarla en el siguiente PATCH.
- `updated_at`: RFC3339 UTC.
- No devuelve `content` ni `likes_count`.

**Errores**

| HTTP | `error.code` | Condición |
| --- | --- | --- |
| 400 | `bad_request` "Request inválido" | Body mal formado o fuera del binding (`note_handler.go:321-328`) |
| 400 | `bad_request` "nada que actualizar" | Sin `title`, `visibility` ni `content` (`L329-332`) |
| 400 | `invalid_title` / `invalid_visibility` | Título vacío o > 255 bytes tras normalizar; visibilidad inválida (`note_service.go:1131-1140`) |
| 403 | `forbidden` "Conecte o renueve su Google Drive" | OAuth del autor ausente o revocado al leer, escribir o crear el archivo (`L1170-1172`, `L1192-1194`, `L1215-1217`, `L1253-1255`) |
| 404 | `not_found` "nota no encontrada" | No existe, no es del usuario, o se borró en paralelo (`L1111-1116`, `L1325-1329`) |
| 404 | `note_unavailable` | Drive responde 404/403 sobre el archivo existente (`L1173-1175`, `L1195-1197`, `L1256-1258`) |
| 409 | `conflict` "Conflicto" | `version` distinta de la actual, o otro escritor ganó la carrera (`L1119-1121`, `L1304-1312`); también por idempotencia |
| 413 | `file_too_large` | Body o `content` > 1 MiB, o Drive 413 |
| 500 | `internal_error` | Fallo de Postgres; también si no se pudo resolver el directorio de Social al recalcular permisos (`L1281-1290`) |
| 502 | `internal_error` "Servicio de almacenamiento no disponible" | Drive caído/timeout, o no se pudo tomar el snapshot previo del contenido (`L1160-1177`) |
| 401 / 423 / 503 | ver [COMUN §2](../COMUN-notes.md#2-autenticación-http) | Middleware |

**Efectos antes de responder** (bajo `pg_advisory_xact_lock` por nota, `WithNoteLock`, `back/notes/internal/repository/shared_repository.go:43`):

1. Si cambia `content` y hay archivo: lee el contenido actual (snapshot para revertir) y actualiza el `.md` (y el nombre si cambia `title`) en Drive (`note_service.go:1153-1200`).
2. Si cambia `content` y **no** hay `external_file_id`: crea el archivo y guarda su ID (self-healing, `L1201-1239`).
3. Si solo cambia `title`: renombra el archivo en Drive (`L1240-1262`).
4. UPDATE condicional por versión en `notes.notes` (`L1302`).
5. Si cambió la visibilidad o se creó el archivo: sincroniza permisos de Drive y `notes.drive_managed_permissions` (`L1333-1337`). Un fallo aquí **no** revierte el PATCH: queda `failed` para el reconciliador.

**Compensaciones:** si el UPDATE falla tras modificar Drive, se revierte el archivo al snapshot (`compensateDriveUpdate`, `L1346-1355`) y la nota queda `failed_sync`; si la reversión también falla se encola `update_file` en `notes.drive_reconciliation_queue`.

## Invocación (Front / Otros Servicios)

- **Pantalla:** detalle de nota (`_NoteDetailSheet`, `front/lib/features/notes/all_notes_screen.dart:3802`). Botón **Editar** (`L4946-4950`) → editor; botón **GUARDAR** (`L4845-4852`) → `_confirmEdit` (`L4047`) → `widget.onSaveEdit` (`L4068`) → `_saveEditFlow` (`L1914-1962`) → `_patchNoteRemote` (`L1678-1733`).
- **Petición:** `PATCH $notesBaseUrl/notes/<id>` (`L1683`) con `_authHeaders()` (Bearer `SessionManager.token`) y `X-Idempotency-Key: note-<id>-<microsegundos>` (una clave nueva por intento, `L1695-1696`, `L1706`). Body `{title, content, version}` (`L1694`). Timeout 30 s. Si no conoce la versión, antes hace `GET /notes/<id>` (`L1684-1689`); sin versión válida no envía nada (`missing_version`, `L1690-1693`).
- **El front nunca envía `visibility` en el PATCH.** La visibilidad solo se elige al crear. Cambiar visibilidad por HTTP no tiene consumidor en el front.
- **Respuesta:** 200 → guarda la nueva `version` (`L1715-1716`) y escribe la nota en Drift con `needsSync: false` (`L1934-1946`). 409 → "La nota cambió en otra sesión…"; 403 `forbidden` → "Solo el autor puede editar esta nota."; sin red → conserva el contenido local (`L4072-4096`).
- **Otros disparadores:** `_patchNoteRemote` también se llama tras crear con adjuntos pendientes (`L765`, `L796`, `L2732`) y desde **REFRESCAR / SINCRONIZAR DRIVE** para notas con cambios pendientes (`L961`).
- **Otros microservicios:** No se encontró invocación en el código revisado.
- **Tests/scripts:** `scripts/test-notes-e2e.py:109-112`.

## Flujo

1. `main.go:208` → `RequireAuth` (`auth_middleware.go:91`).
2. `NoteHandler.Patch` (`note_handler.go:308`): valida id y body, lee `X-Idempotency-Key` y `version`.
3. `NoteService.UpdateWithExpectedVersion` (`note_service.go:1052`): tamaño, idempotencia (`beginIdempotentOperation` `L172`).
4. `updateInner` (`L1090`) → `SharedStore.WithNoteLock` (`service/store_pg.go:180`) → `updateInnerLocked` (`L1106`).
5. `GetByID` → comprueba autor y versión → Drive (`GetFileContent`/`UpdateFile`/`CreateFile`, `drive/real_client.go:192,209,127`, síncrono, token del autor).
6. Si cambia visibilidad: `ListByNote` + `ComputeDesiredDrivePermissions` (`L2037`; para grupos `restricted` consulta Social `GetMembers` por gRPC y correos en `identity.oauth_connections`).
7. `PGNoteStore.Update` (`store_pg.go:31`) → `NoteRepository.Update` (`note_repository.go:136`).
8. `syncDrivePermissions` (`L2128`) si aplica; respuesta 200.

Finalmente devuelve `200 {id, title, visibility, version, updated_at}`, y la pantalla de detalle utiliza ese resultado para guardar la nueva versión, marcar la nota como sincronizada en Drift y cerrar el modo edición.

## Tests

| Archivo:línea | Prueba | Qué comprueba | Tipo |
| --- | --- | --- | --- |
| `back/notes/internal/handler/http/note_handler_test.go:550` | `TestHandlerPatchSyncDrive` | El contenido llega a Drive | Unitaria (Drive mock) |
| `note_handler_test.go:591` | `TestHandlerPatchVersionConflict` | `version` desfasada → 409 | Unitaria |
| `note_handler_spec95_test.go:31,86,114` | `TestHandlerPatchIdempotencyReplay`, `TestHandlerPatchWithoutIdempotencyKeyAppliesEachChange`, `TestHandlerPatchIdempotencyDistinctKeysAndErrorCleanup` | Idempotencia de PATCH | Unitaria |
| `note_postgres_edit_test.go:23` | `TestPostgresConsecutiveNoteEdits` | Ediciones consecutivas contra Postgres | Integración (`NOTES_TEST_DATABASE_URL`) |
| `back/notes/internal/service/note_service_test.go:217,660` | `TestUpdateOnlyAuthor`, `TestEditPrivateNoteForbidden` | Solo el autor edita | Unitaria |
| `note_service_test.go:970,993,1022` | `TestDriveUpdateSyncsContent`, `TestDriveUpdateRecreatesMissingFile`, `TestDriveUpdateNotFoundMapsNoteUnavailable` | Escritura, self-healing y `note_unavailable` | Unitaria |
| `service/drive_permissions_test.go:281,368` | `TestVisibilityUpdatesDriveLink`, `TestLegacyPublicNoteRevokesLinkOnVisibilityChange` | Cambio de visibilidad otorga/revoca `anyone` | Unitaria |
| `service/remediation_v3_test.go:18,63` | `TestUpdateSnapshotFailureAbortsWithoutMutation`, `TestUpdatePGFailureMarksFailedSync` | Snapshot y `failed_sync` | Unitaria |
| `service/rollback_test.go:184,217` | `TestRefactorUpdatePGFailureRevertsDrive`, `TestUpdateSelfHealingRollbackOnDBFailure` | Compensaciones | Unitaria |
| `service/spec95_test.go:99,171` | `TestUpdateIdempotencyReplayAndErrorCleanup`, `TestUpdateIdempotencyInFlightRejectsConcurrent` | Idempotencia en servicio | Unitaria |
| `service/note_version_lock_test.go:20,58,105` | `TestPermissionOptimisticVersionConflict`, `TestPermissionConcurrentUpdatesSingleWinner`, `TestPermissionWithNoteLockSerializes` | Versión y lock por nota | Unitaria |
| `back/notes/internal/repository/note_repository_version_test.go:47,79,103` | `TestNoteRepositoryUpdate*` | SQL del UPDATE con versión | Integración |
| `back/notes/internal/handler/grpc/server_test.go:271,298` | `TestUpdateNoteOptimisticVersionConflict`, `TestUpdateNoteByStrangerIsNotFound` | Equivalente gRPC (bufconn) | Unitaria gRPC |
| `front/test/notes_edit_lifecycle_test.dart:35` | `ediciones consecutivas, cache, JWT y adjuntos local=$local` | PATCH con versión y JWT desde la UI | Widget (HTTP falso) |
