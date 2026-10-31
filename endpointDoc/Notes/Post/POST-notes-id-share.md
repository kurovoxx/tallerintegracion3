# POST /notes/{id}/share

Comparte una nota propia con un grupo de Social en modo `link` o `restricted`. Primero guarda la intención en Postgres (`notes.shared_notes` + estado deseado en `notes.drive_managed_permissions`) y luego aplica los permisos en Drive. Ruta interna del backend Notes (`:8082`). Desde el navegador: `POST https://ti3-brojas.dev.censei.cl/api-notes/notes/<id>/share` ([prefijos](../COMUN-notes.md#1-rutas-internas-y-prefijos-externos)). Equivalente gRPC: `NotesService/ShareNote`.

## Req

- **Registro:** `back/notes/cmd/server/main.go:207` → `protected.POST("/notes/:id/share", h.Share)`. Handler `back/notes/internal/handler/http/note_handler.go:717-744` (`Share`).
- **Autenticación:** `RequireAuth` (`main.go:192`). Ver [COMUN §2](../COMUN-notes.md#2-autenticación-http).
- **Permiso:** autor de la nota **y** miembro del grupo según Social (`back/notes/internal/service/note_service.go:1986-1996`).
- **Path:** `id` UUID de la nota (no UUID → 404 `not_found`).
- **Body JSON** (`shareRequest`, `note_handler.go:712-715`), límite 1 MiB:

| Campo | Tipo | Obligatorio | Validación |
| --- | --- | --- | --- |
| `group_id` | string | sí | Binding `required,uuid`. No se comprueba que el grupo exista más allá de la membresía |
| `access_mode` | string | sí | Binding `oneof=link restricted` |

```json
{"group_id":"55555555-5555-4555-8555-555555555555","access_mode":"restricted"}
```

- `link`: cualquier usuario autenticado de la app puede leer la nota; en Drive se otorga `anyone`/`reader`.
- `restricted`: solo miembros del grupo leen en la app; en Drive se otorga `reader` por correo a cada miembro con Drive conectado.

## Res

**201 Created** (`note_handler.go:743`)

```json
{"shared_note_id":"66666666-6666-4666-8666-666666666666"}
```

- `shared_note_id`: UUID de la fila en `notes.shared_notes`. Es el identificador que exige [DELETE /notes/shared/{id}](../Delete/DELETE-notes-shared-sharedNoteId.md).
- Se responde **201 aunque Drive falle**: el share queda guardado con `permission_sync_status=failed` (`note_service.go:2020-2026`). El HTTP no informa ese estado; puede consultarse con `GET /notes/:id/access` (`drive_sync_status`).
- Compartir dos veces la misma nota con el mismo grupo **no se rechaza**: el servicio no lo comprueba y `notes.shared_notes` no tiene índice único `(note_id, group_id)` (`docker/postgres/init.sql:274-283,310-318`). Se crea otra fila con otro `shared_note_id`.

**Errores**

| HTTP | `error.code` | Condición |
| --- | --- | --- |
| 400 | `bad_request` "group_id y access_mode requeridos" | Body inválido o fuera del binding (`note_handler.go:730-737`) |
| 403 | `forbidden` "Acceso denegado" | No es el autor (`note_service.go:1986-1988`) o no es miembro del grupo (`L1994-1996`) |
| 404 | `not_found` "nota no encontrada" | `id` no UUID o nota inexistente (`L1983-1985`) |
| 413 | `file_too_large` | Body > 1 MiB |
| 500 | `internal_error` | Social no responde al comprobar membresía (`L1990-1993`), no se pudo calcular el estado deseado (directorio de miembros o correos caído; `restricted` falla cerrado, `L2008-2013`, `L2048-2051`), o fallo de Postgres |
| 401 / 423 / 503 | ver [COMUN §2](../COMUN-notes.md#2-autenticación-http) | Middleware |

**Efectos antes de responder** (bajo `pg_advisory_xact_lock` por nota, `WithNoteLock`, `back/notes/internal/repository/shared_repository.go:43`):

1. Consulta a Social (gRPC `social:9083`, `x-internal-key`): `IsMember`, `IsAdmin` (para `is_admin_note`); `GetFollowersCount` devuelve siempre 0 en el adaptador gRPC (`back/notes/internal/service/social_grpc_adapter.go:57-87`).
2. Calcula el estado deseado = visibilidad + todos los shares (incluido el nuevo) (`ComputeDesiredDrivePermissions`, `note_service.go:2037-2056`). Para cada grupo `restricted`:
   - user_id de los miembros con Social `GetMembers` (`social_grpc_adapter.go:92-109`);
   - correo de la **cuenta Google Drive conectada** de cada uno, leído directamente de `identity.oauth_connections.external_account_email` (`provider='google_drive'`, `revoked_at IS NULL`) (`back/notes/internal/service/drive_email_directory.go:50-78,110-117`; cableado en `main.go:136-138`);
   - miembros sin Drive se omiten y se registra un aviso; no se usa su correo de registro (`note_service.go:2072-2080`);
   - correos en minúsculas, válidos y sin duplicados (`back/notes/internal/service/member_directory.go:31-46`). El autor, si es miembro, también se incluye.
3. INSERT en `notes.shared_notes` (`access_mode`, `is_admin_note`, `author_followers_snapshot`) (`note_service.go:2016`; `shared_repository.go:72`).
4. `syncDrivePermissions` (`note_service.go:2128-2334`) con el token del autor:
   - persiste cada permiso deseado como `pending` en `notes.drive_managed_permissions`;
   - `restricted`: `Permissions.Create` `type=user`, `role=reader`, `sendNotificationEmail=false` por correo, guardando el ID de permiso devuelto (`back/notes/internal/drive/real_client.go:416-437`). Un 409 de Drive (ya existe) cuenta como éxito;
   - `link`: `Permissions.Create` `type=anyone`, `role=reader`, `allowFileDiscovery=false` (`real_client.go:439-455`);
   - marca cada fila `in_sync` o `failed` y actualiza `permission_sync_status` de los shares de la nota.

Ciclo completo de permisos: [COMUN §5](../COMUN-notes.md#5-ciclo-de-vida-de-los-permisos-de-drive).

**Segundo plano:** si algún permiso quedó `failed`, el reconciliador (cada 30 s) vuelve a calcular y aplicar el estado deseado (`reconcileManagedPermissions`, `note_service.go:2340-2381`). Los miembros que se unan al grupo **después** no reciben permiso hasta que algo vuelva a sincronizar esa nota (no se encontró disparador por alta de miembro).

## Invocación (Front / Otros Servicios)

- **Pantalla:** `GroupNotesTab` (`front/lib/features/groups/group_notes_tab.dart`). Botón **Compartir una nota** (`L326-330`) → `_shareFlow` (`L127`): carga `GET /notes/me` (`L135`), abre `_ShareNoteDialog` con selector **Link** / **Restricted** (por defecto `link`, `L588`, `L683-707`) → `NotesSharingService.shareNote` (`L154`).
- **Cliente:** `front/lib/core/services/notes_sharing_service.dart:224-263`: valida modo, `POST {notesApiBaseUrl}/notes/<noteId>/share` con `Authorization: Bearer ${SessionManager.token}` y `Content-Type: application/json` (`L113-126`), body `{group_id, access_mode}`, timeout 15 s, vía `AuthedHttp.run`.
- **Respuesta:** exige 201 y `shared_note_id`; lo guarda en el mapa en memoria `_sharedByMe[noteId]` (`group_notes_tab.dart:159`) y recarga la lista. Errores: 403 → "No tienes permiso (403). Debes ser miembro del grupo." (`notes_sharing_service.dart:150-153`). No escribe en Drift.
- `NotesApi.shareNote` (`front/lib/features/notes/notes_api.dart:180-213`): solo la usa `front/test/cross_service_e2e_test.dart`.
- **Otros microservicios:** No se encontró invocación en el código revisado.
- **Tests/scripts:** `back/social/test/cross_service_e2e_test.go:731` (tag `crossservice`, procesos reales Auth/Notes/Social); `scripts/test-notes-e2e.py:145-146,152`.

## Flujo

1. `main.go:207` → `RequireAuth` (`auth_middleware.go:91`).
2. `NoteHandler.Share` (`note_handler.go:717`): `validateNoteID`, bind del body.
3. `NoteService.Share` (`note_service.go:1959`) → `WithNoteLock` (`service/store_pg.go:180`) → `shareLocked` (`L1975`).
4. `GetByID`, autor; Social gRPC `IsMember`/`IsAdmin` (síncrono, 5 s).
5. `ListByNote` + `ComputeDesiredDrivePermissions` → para `restricted`: Social gRPC `GetMembers` + SELECT en `identity.oauth_connections`.
6. `SharedStore.Create` (`store_pg.go:118` → `shared_repository.go:72`).
7. `syncDrivePermissions` → Drive `GrantPermission`/`GrantLinkPermission` (síncrono, token del autor vía Identity gRPC `GetOAuthToken`) → `UpsertManagedPermission` (`store_pg.go:162` → `shared_repository.go:322`).
8. Respuesta 201.

Finalmente devuelve `201 {shared_note_id}`, y la pestaña de notas del grupo utiliza ese resultado para recordar (solo en memoria) qué vínculo creó el usuario, permitir "Dejar de compartir" y recargar la lista del grupo.

## Tests

| Archivo:línea | Prueba | Qué comprueba | Tipo |
| --- | --- | --- | --- |
| `back/notes/internal/handler/http/note_handler_test.go:386` | `TestHandlerShareAndGroupNotes` | Share 201 y aparece en la lista del grupo | Unitaria (memoria) |
| `note_handler_test.go:680` | `TestAccessModeLinkVsRestricted` | Diferencia de lectura link/restricted | Unitaria |
| `note_handler_test.go:1063` | `TestHandlerShareRestrictedGrantsDrive` | Restricted otorga permisos en Drive mock | Unitaria |
| `audit_fixes_test.go:172` | `TestHandlerOneofValidation400` | `access_mode` inválido → 400 | Unitaria |
| `back/notes/internal/service/note_service_test.go:470,529,548,564` | `TestShareAndAccessControl`, `TestShareForbiddenIfNotAuthor`, `TestShareForbiddenIfNotMember`, `TestShareInvalidAccessMode` | Reglas de servicio | Unitaria |
| `note_service_test.go:1323-1518` | `TestShareRestrictedGrants`, `TestShareRestrictedDedup`, `TestShareRestrictedSkipsBlankEmails`, `TestShareRestrictedMemberError`, `TestShareRestrictedPartialFailureConvergesLater`, `TestShareRestrictedEmptyGroup`, `TestShareRestrictedAuthorOnce`, `TestShareLinkNoGrants`, `TestNormalizeEmails` | Permisos por correo | Unitaria |
| `service/drive_emails_test.go:29,74,102,124,197,238` | `TestRestrictedUsesGoogleEmailsNotAppEmails`, `TestRestrictedMemberWithoutDriveIsSkipped`, `TestRestrictedDedupGoogleEmails`, `TestLinkThenRestrictedKeepsAnyoneAndGrantsGoogle`, `TestRestrictedDirectoryErrorFailsClosed`, `TestDriveEmailDirectoryComposite` | Usa el correo de Google Drive, omite miembros sin Drive, falla cerrado | Unitaria |
| `service/permission_state_test.go:14,58` | `TestSharePersistsDesiredStatePermissions`, `TestShareLinkPersistsAnyonePermissionID` | Estado deseado en BD | Unitaria |
| `service/drive_permissions_test.go:14,186,399,436,537` | `TestComputeDesiredDrivePermissions`, `TestShareDatabaseFailureLeavesDriveUntouched`, `TestRestrictedSharePersistsEmailPermissionIDs`, `TestRestrictedShareFiltersMalformedEmails`, `TestShareCancellationPersistsIntentionAndPreservesExistingGroup` | Cálculo y persistencia de permisos | Unitaria |
| `back/notes/internal/handler/grpc/server_test.go:345,382` | `TestShareNoteLinkAndRestrictedThenUnshare`, `TestShareNoteByNonMemberIsPermissionDenied` | Equivalente gRPC (bufconn) | Unitaria gRPC |
| `back/notes/test/e2e_social_notes_test.go:306,396,427` | `TestE2EShareRestrictedThenUnshareAllCleansUp`, `TestE2EShareFailClosedWhenSocialDown`, `TestE2EShareNotMemberIsPermissionDenied` | Share vía gRPC contra un Social **HTTP falso** (`SocialHTTPAdapter`), Drive mock, stores en memoria | E2E (bufconn) |
| `back/social/test/cross_service_e2e_test.go:125` | `TestCrossServiceMembershipLifecycle` | Share `restricted` entre procesos reales con Postgres aislado | E2E (`-tags=crossservice`) |
| `front/test/group_notes_tab_test.dart:255` | `compartir (link) refresca y permite dejar de compartir` | Flujo de UI con `MockClient` | Widget |
