# POST /notes/unshare-all

> **Estado: vigente pero DEPRECADO.** La ruta HTTP **sigue registrada** (`back/notes/cmd/server/main.go:196`) y responde. El transporte principal es el RPC interno [`notes.v1.NotesService/UnshareAll`](../Grpc/GRPC-notes-UnshareAll.md) (autenticado con `x-internal-key`). Esta ruta queda como respaldo mientras Social mantenga su fallback HTTP; cada uso escribe `[DEPRECATED] POST /notes/unshare-all vía HTTP…` en el log (`back/notes/internal/handler/http/note_handler.go:781`). No se ha retirado, por eso este documento no se marca como "reemplazado".

Retira todos los vínculos que `user_id` compartió en `group_id` y vuelve a calcular los permisos de Drive de las notas que siguen compartidas con ese grupo. Ruta interna del backend Notes (`:8082`). Como nginx no bloquea `/api-notes/`, también es alcanzable desde fuera: `POST https://ti3-brojas.dev.censei.cl/api-notes/notes/unshare-all` ([prefijos](../COMUN-notes.md#1-rutas-internas-y-prefijos-externos)).

## Req

- **Registro:** `main.go:196` → `protected.POST("/notes/unshare-all", h.UnshareAll)`. Handler `note_handler.go:779-811` (`UnshareAll`).
- **Autenticación:** `RequireAuth` (`main.go:192`) con **JWT de usuario** (no acepta `X-Internal-Key`). Ver [COMUN §2](../COMUN-notes.md#2-autenticación-http).
- **Autorización** (`note_handler.go:802-805`): el llamante (`user_id` del token) debe ser el propio `user_id` del body **o** admin de `group_id` según Social `IsAdmin` (`back/notes/internal/service/note_service.go:2779-2782`). Si Social falla, `IsGroupAdmin` devuelve `false` y la respuesta es 403.
- **Body JSON** (`unshareAllRequest`, `note_handler.go:769-772`), límite 1 MiB:

| Campo | Tipo | Obligatorio | Validación |
| --- | --- | --- | --- |
| `user_id` | string | sí | Binding `required,uuid`. Es el usuario **objetivo** de la limpieza, no el llamante |
| `group_id` | string | sí | Binding `required,uuid` |

```json
{"user_id":"22222222-2222-4222-8222-222222222222","group_id":"55555555-5555-4555-8555-555555555555"}
```

A diferencia del RPC, no tiene `group_deleted`.

## Res

**204 No Content**, sin body (`note_handler.go:810`). Idempotente: repetirlo sin vínculos también da 204. Responde 204 aunque Drive falle (la reconvergencia es best-effort).

**Errores**

| HTTP | `error.code` | Condición |
| --- | --- | --- |
| 400 | `bad_request` "user_id y group_id requeridos" | Body inválido o ids no UUID (`note_handler.go:794-801`) |
| 403 | `forbidden` "no autorizado" | El llamante no es `user_id` ni admin del grupo (`L802-805`) |
| 403 | `forbidden` "Acceso denegado" | Al retirar un vínculo, `Unshare` exige ser autor o admin; como solo recorre notas de `user_id`, en la práctica no ocurre |
| 413 | `file_too_large` | Body > 1 MiB |
| 500 | `internal_error` | Fallo de Postgres al listar notas o shares (`note_service.go:2469-2477`), o fallo propagado por `Unshare` |
| 401 / 423 / 503 | ver [COMUN §2](../COMUN-notes.md#2-autenticación-http) | Middleware |

**Efectos antes de responder** (`NoteService.UnshareAll`, `note_service.go:2461-2500`, todo síncrono):

1. Pagina las notas **propias** de `user_id` (100 por página, `ListByUser`) y, para cada vínculo con `group_id`, llama `NoteService.Unshare` (`L2465-2490`): borra la fila de `notes.shared_notes` y reconverge los permisos de esa nota (ver [DELETE /notes/shared/{id}](../Delete/DELETE-notes-shared-sharedNoteId.md)).
2. `reconcileGroupNotePermissions(groupID)` (`L2498`, `L2504-2525`): recorre **todas** las notas que siguen compartidas con el grupo (también de otros autores) y, bajo el lock de cada nota, recalcula y aplica permisos (`reconcileNotePermissions`, `L2530-2552`). Como Social ya no devuelve al saliente como miembro, se revoca su correo de Google Drive en esas notas, salvo que siga teniendo acceso por otro grupo. No toca `anyone`/`link`. Los fallos solo se registran.

Ver [COMUN §5](../COMUN-notes.md#5-ciclo-de-vida-de-los-permisos-de-drive).

**Segundo plano:** lo que quede `failed` lo reintenta el reconciliador cada 30 s.

## Invocación (Front / Otros Servicios)

- **Frontend Flutter:** No se encontró invocación en el código revisado. El front no llama a esta ruta; la casilla "limpiar notas compartidas" (`front/lib/features/groups/cleanup_shared_notes_checkbox.dart`) envía `cleanup_shared_notes` a **Social** (ver abajo).
- **Social (fallback HTTP):** `GroupService.unshareNotesHTTP` (`back/social/internal/service/group_service.go:619-639`) hace `POST {notesBaseURL}/notes/unshare-all` con `{user_id, group_id}` y reenvía el JWT del usuario que hizo la acción en Social, timeout 5 s. Solo se usa si falla el gRPC: `unshareNotes` (`L602-613`) intenta primero `notesGRPCUnsharer.UnshareAll` (`back/social/internal/service/notes_grpc.go:64-82`) y cae al HTTP si devuelve error (`L610-612`). Es best-effort: no revisa el código de respuesta (`L633-638`).
  - `notesBaseURL` sale de `NOTES_URL` o, si no existe, de `http://localhost:8082` (`group_service.go:100,158-160`). `NOTES_URL` **no** está definida en `docker-compose.yml` ni en `k8s/manifests.yaml`, así que en esos despliegues el respaldo apunta a `localhost` del contenedor de Social y no llega a Notes. En la práctica, el único camino que funciona en Compose/k8s es el gRPC.
- **Cuándo lo dispara Social** (en goroutine, `go s.unshareNotes(...)`), si el body trae `cleanup_shared_notes: true`:
  - salir del grupo: `LeaveGroup` (`group_service.go:578-590`), desde `POST /groups/:id/leave` (`back/social/internal/handler/http/group_handler.go:285`); front `SocialService.leaveGroup` (`front/lib/core/services/social_service.dart:824`), diálogo con `CleanupSharedNotesCheckbox` (`front/lib/features/groups/groups_screen.dart:590`);
  - expulsar: `KickMember` (`group_service.go:525-536`), `group_handler.go:200`, front `kickMember` (`social_service.dart:213`), diálogo en `front/lib/features/groups/group_members_tab.dart:200`;
  - banear: `BanMember` (`group_service.go:543-554`), `group_handler.go:219`, front `banMember` (`social_service.dart:239`);
  - `HandleAccountDeletion` (`group_service.go:648-664`).
- **Detalle del fallback en kick/ban:** el JWT reenviado es el del admin y `user_id` es el expulsado, por lo que Notes autoriza por la rama "admin del grupo". Como la llamada ocurre **después** de que Social ya actualizó la membresía, si el admin dejó de serlo la llamada HTTP daría 403 (no se observó un caso así en el código).
- **Tests/scripts:** `scripts/test-notes-e2e.py:153-155` (403 para un tercero, 204 para el propio usuario).

## Flujo

1. `main.go:196` → `RequireAuth` (`auth_middleware.go:91`).
2. `NoteHandler.UnshareAll` (`note_handler.go:779`): log `[DEPRECATED]`, bind, autorización (propio o `IsGroupAdmin` → Social gRPC `IsAdmin`, `back/notes/internal/service/social_grpc_adapter.go:70`).
3. `NoteService.UnshareAll` (`note_service.go:2461`): `ListByUser` (`service/store_pg.go:28`) → `ListByNote` → `Unshare` por vínculo (lock por nota, Drive síncrono).
4. `reconcileGroupNotePermissions` → `ListByGroup` (`store_pg.go:127` → `repository/shared_repository.go:127`) → `reconcileNotePermissions` por nota (Social `GetMembers`, `identity.oauth_connections`, Drive).
5. Respuesta 204.

Finalmente devuelve `204`, y Social (único consumidor, solo como respaldo) no utiliza el resultado: registra un error en el log únicamente si la petición HTTP no pudo enviarse.

## Tests

| Archivo:línea | Prueba | Qué comprueba | Tipo |
| --- | --- | --- | --- |
| `back/notes/internal/handler/http/audit_fixes_test.go:26` | `TestUnshareAllIDORForbidden` | Un tercero no puede limpiar a otro usuario → 403 | Unitaria |
| `audit_fixes_test.go:76` | `TestUnshareAllSelfAllowedAndAdminAllowed` | Propio usuario y admin → 204 | Unitaria |
| `audit_fixes_test.go:111,124` | `TestUnshareAllUnauthorized401`, `TestUnshareAllInvalidUUID400` | 401 sin token; 400 con ids inválidos | Unitaria |
| `note_handler_test.go:446` | `TestHandlerUnshareAll` | Limpieza por HTTP | Unitaria (memoria) |
| `back/notes/internal/service/note_service_test.go:581` | `TestUnshareAll` | Servicio | Unitaria |
| `service/drive_permissions_test.go:118` | `TestUnshareAllRevokesOnlyOwnerGroupAcrossPages` | Solo vínculos del usuario en ese grupo, con paginación | Unitaria |
| `service/unshare_all_group_test.go:48,115,157,192,215` | `TestUnshareAllRevokesLeaverOnOtherAuthorsNote`, `TestUnshareAllKeepsLeaverIfSharedByAnotherGroup`, `TestUnshareAllKeepsLinkAnyone`, `TestUnshareAllIdempotent`, `TestUnshareAllDriveRevokeFailureDeferred` | Revocación en notas de otros autores, conservación por otro grupo, `anyone` intacto, idempotencia, fallo de Drive diferido | Unitaria |
| `back/notes/cmd/server/router_contract_test.go:15` | `TestHTTPRouterContract` | La ruta sigue registrada; sin token → 401 | Unitaria |

Los tests de Social que ejercitan el fallback HTTP no se revisaron en detalle; `back/social/internal/service/group_service.go:175-179` (`SetNotesUnsharer(nil)`) es el gancho que usan para forzarlo.
