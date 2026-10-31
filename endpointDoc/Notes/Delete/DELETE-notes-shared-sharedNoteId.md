# DELETE /notes/shared/{sharedNoteId}

Retira un vínculo nota↔grupo (`notes.shared_notes`) y vuelve a calcular los permisos de Drive con los shares que quedan, revocando lo que sobra. Ruta interna del backend Notes (`:8082`). Desde el navegador: `DELETE https://ti3-brojas.dev.censei.cl/api-notes/notes/shared/<sharedNoteId>` ([prefijos](../COMUN-notes.md#1-rutas-internas-y-prefijos-externos)). Equivalente gRPC: `NotesService/UnshareNote`.

## Req

- **Registro:** `back/notes/cmd/server/main.go:211` → `protected.DELETE("/notes/shared/:sharedNoteId", h.Unshare)`. Handler `back/notes/internal/handler/http/note_handler.go:747-766` (`Unshare`).
- **Autenticación:** `RequireAuth` (`main.go:192`). Ver [COMUN §2](../COMUN-notes.md#2-autenticación-http).
- **Permiso:** autor de la nota, o admin del grupo del vínculo según Social `IsAdmin` (`back/notes/internal/service/note_service.go:2425-2433`).
- **Path:** `sharedNoteId` = `id` de `notes.shared_notes` (el `shared_note_id` que devolvió el share), **no** el id de la nota. No UUID → 404 `not_found` "compartición no encontrado" (`validateUUIDParam`, `note_handler.go:152-158`, `L758-760`).
- **Query / body:** no tiene.

## Res

**204 No Content**, sin body (`note_handler.go:765`). También 204 si la nota ya no existe (se limpia el vínculo huérfano, `note_service.go:2417-2422`) y aunque Drive falle.

**Errores**

| HTTP | `error.code` | Condición |
| --- | --- | --- |
| 403 | `forbidden` "Acceso denegado" | No es el autor ni admin del grupo (`note_service.go:2430-2432`) |
| 404 | `not_found` "compartición no encontrada" | El vínculo no existe (o ya se retiró) (`L2397-2399`, `L2410-2412`) |
| 500 | `internal_error` | Social no responde a `IsAdmin` (`L2426-2429`) o fallo de Postgres al leer/borrar (`L2394-2396`, `L2435-2437`) |
| 401 / 423 / 503 | ver [COMUN §2](../COMUN-notes.md#2-autenticación-http) | Middleware |

**Efectos antes de responder** (bajo `pg_advisory_xact_lock` por nota, `back/notes/internal/repository/shared_repository.go:43`):

1. `DELETE` de la fila en `notes.shared_notes` (`note_service.go:2435`; `shared_repository.go:176`).
2. Marca como `pending` los permisos administrados de la nota (`MarkManagedPermissionsPending`, `L2441`), para que el reconciliador tenga un disparador si lo siguiente falla.
3. Recalcula el estado deseado con los shares restantes (`ComputeDesiredDrivePermissions`, `L2445-2454`; para grupos `restricted` consulta Social `GetMembers` y `identity.oauth_connections`).
4. `syncDrivePermissions` (`L2455`) con el token del autor: revoca los correos que ya no tienen ninguna vía de acceso (`RevokePermission` por correo, `back/notes/internal/drive/real_client.go:497`) y el `anyone` si ya no hay `link` ni visibilidad `public` (`RevokePermissionByID`, `real_client.go:457`); un 404 de Drive cuenta como ya revocado; borra esas filas de `notes.drive_managed_permissions`. Los correos que siguen teniendo acceso por otro grupo se conservan.

Si el paso 3 o 4 falla, el vínculo queda borrado igualmente, los permisos quedan `pending`/`failed` y responde 204 (`L2445-2458`).

**Segundo plano:** el reconciliador (cada 30 s, `reconcileManagedPermissions` `note_service.go:2340`) termina de revocar lo pendiente. Ver [COMUN §5](../COMUN-notes.md#5-ciclo-de-vida-de-los-permisos-de-drive).

## Invocación (Front / Otros Servicios)

- **Pantalla:** `GroupNotesTab` (`front/lib/features/groups/group_notes_tab.dart`). Botón **Dejar de compartir** (`L412-418`) → diálogo "Dejar de compartir" / **Retirar** (`L182-190`) → `_unshare` (`L170-206`) → `NotesSharingService.unshare` (`L194`).
- **Cliente:** `front/lib/core/services/notes_sharing_service.dart:266-279`: `DELETE {notesApiBaseUrl}/notes/shared/<sharedNoteId>` con `Authorization: Bearer ${SessionManager.token}`, timeout 12 s, vía `AuthedHttp.run`. Acepta 204 y también 404 como éxito.
- **Limitación del front:** el `sharedNoteId` sale del mapa en memoria `_sharedByMe`, que solo se llena al compartir en la misma sesión (`group_notes_tab.dart:159`, `L171-178`). `GET /groups/:id/notes` no devuelve `shared_note_id` (`back/notes/internal/handler/http/note_handler.go:873-880`), así que tras recargar la app no se puede dejar de compartir desde la UI ("Solo puedes dejar de compartir las notas que compartiste en esta sesión.").
- **Respuesta:** borra la entrada de `_sharedByMe` y recarga la lista. No escribe en Drift.
- **Otros microservicios:** No se encontró invocación HTTP. Internamente `NoteService.UnshareAll` (usado por `POST /notes/unshare-all`) llama a `NoteService.Unshare` para cada vínculo propio (`note_service.go:2478-2483`).
- **Tests/scripts:** `scripts/test-notes-e2e.py:150-151`.

## Flujo

1. `main.go:211` → `RequireAuth` (`auth_middleware.go:91`).
2. `NoteHandler.Unshare` (`note_handler.go:747`): `validateUUIDParam`.
3. `NoteService.Unshare` (`note_service.go:2389`): `SharedStore.GetByID` (`service/store_pg.go:121` → `shared_repository.go:90`) → `WithNoteLock` → `unshareLocked` (`L2405`).
4. `GetByID` nota; si no es el autor, Social gRPC `IsAdmin` (`back/notes/internal/service/social_grpc_adapter.go:70`, síncrono, 5 s).
5. `SharedStore.Delete` → `MarkManagedPermissionsPending` → `ListByNote` → `ComputeDesiredDrivePermissions` → `syncDrivePermissions` (Drive síncrono).
6. Respuesta 204.

Finalmente devuelve `204`, y la pestaña de notas del grupo utiliza ese resultado para olvidar el vínculo y recargar la lista de notas compartidas.

## Tests

| Archivo:línea | Prueba | Qué comprueba | Tipo |
| --- | --- | --- | --- |
| `back/notes/internal/handler/http/note_handler_test.go:386` | `TestHandlerShareAndGroupNotes` | Comparte y luego retira por HTTP (DELETE en `L438`) | Unitaria (memoria) |
| `back/notes/internal/handler/http/audit_fixes_test.go:144` | `TestHandlerInvalidUUID404` | `/notes/shared/no-uuid` → 404 (`L160`) | Unitaria |
| `back/notes/internal/service/note_service_test.go:470` | `TestShareAndAccessControl` | Share y `Unshare` por el autor (`L519`) | Unitaria |
| `back/notes/internal/service/drive_permissions_test.go:38` | `TestUnsharePreservesOtherGroupPermissions` | Conserva permisos de otros grupos | Unitaria (Drive mock) |
| `drive_permissions_test.go:75` | `TestLinkShareLifecyclePreservesPublicAndOtherLinks` | `anyone` se mantiene si hay otra fuente | Unitaria |
| `drive_permissions_test.go:209,238` | `TestUnshareDatabaseFailurePreservesShare`, `TestUnshareDriveFailureConvergesLater` | Fallo de BD conserva el share; fallo de Drive converge después | Unitaria |
| `drive_permissions_test.go:467` | `TestUnshareRevokesNominalPermissionAndClearsRow` | Revoca por correo y borra la fila | Unitaria |
| `service/drive_emails_test.go:168` | `TestRestrictedUnshareRevokesGoogleEmail` | Revoca el correo de Google Drive | Unitaria |
| `service/permission_state_test.go:84` | `TestUnshareWithoutDirectoryConvergesWhenSocialRecovers` | Social caído → converge al volver | Unitaria |
| `back/notes/internal/handler/grpc/server_test.go:345` | `TestShareNoteLinkAndRestrictedThenUnshare` | Compartir y retirar vía gRPC (bufconn) | Unitaria gRPC |
| `back/notes/cmd/server/router_contract_test.go:15` | `TestHTTPRouterContract` | Ruta registrada; sin token → 401 | Unitaria |
| `front/test/group_notes_tab_test.dart:255` | `compartir (link) refresca y permite dejar de compartir` | Pulsa DEJAR DE COMPARTIR con `MockClient` | Widget |

No hay prueba directa en el código revisado del caso "admin del grupo retira el share de otro autor" ni del 403 por HTTP.
