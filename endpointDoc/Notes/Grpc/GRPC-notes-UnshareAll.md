# gRPC notes.v1.NotesService/UnshareAll

RPC **interno** (servicio a servicio) que retira los vínculos de un usuario en un grupo y deja convergidos los permisos de Drive afectados. Es el transporte principal de la limpieza al salir, ser expulsado o baneado de un grupo y durante el borrado de cuenta. Reemplaza funcionalmente a [POST /notes/unshare-all](../Post/POST-notes-unshare-all.md), que sigue registrado como respaldo deprecado. Puerto `:9082`, solo red interna ([prefijos](../COMUN-notes.md#1-rutas-internas-y-prefijos-externos)).

## Req

- **RPC:** `notes.v1.NotesService/UnshareAll` (`back/proto/notes/v1/notes.proto:99`).
- **Mensaje de entrada** `UnshareAllRequest` (`notes.proto:324-331`):

| Campo | Tipo | Obligatorio | Validación |
| --- | --- | --- | --- |
| `user_id` | string | sí | UUID. Usuario **objetivo** (quien sale), no el llamante. Inválido → `INTERNAL` (`back/notes/internal/service/account_deletion.go:148-151`) |
| `group_id` | string | sí | UUID. Inválido → `INTERNAL` (`account_deletion.go:81-87`) |
| `group_deleted` | bool | no | `true` cuando Social borró el grupo (último miembro): se retiran **todos** los vínculos de ese grupo, de cualquier autor |

- **Autenticación:** `NewAuthRouter` envía este método a `NewInternalAuth` (`back/notes/internal/interceptor/auth.go:99-104`). Exige metadata `x-internal-key` igual a `INTERNAL_API_KEY` (comparación de tiempo constante, `auth.go:36-51`). Un JWT de usuario **no** sirve. No pasa por el gate de borrado de cuenta.
- **Registro:** `back/notes/cmd/server/main.go:233-240`. Implementación `back/notes/internal/handler/grpc/server.go:168-174`.
- **Requisito de despliegue:** necesita el repositorio de borrado de cuenta (`SetAccountDeletionRepository`, `main.go:129`), que solo existe con Postgres. En modo memoria (`DATABASE_URL` vacío) `accountDeletionPrecondition` falla y el RPC responde siempre `INTERNAL` (`account_deletion.go:152-154`).

```sh
grpcurl -plaintext -import-path back/proto/notes -proto v1/notes.proto \
  -H "x-internal-key: $INTERNAL_API_KEY" \
  -d '{"userId":"<uuid>","groupId":"<uuid>"}' \
  localhost:9082 notes.v1.NotesService/UnshareAll
# {"status":"ACCOUNT_CLEANUP_STATUS_COMPLETE"}
```

## Res

`UnshareAllResponse{status}` (`notes.proto:333-336`), enum `AccountCleanupStatus` (`L112-117`):

| Resultado | Significado |
| --- | --- |
| `OK` + `ACCOUNT_CLEANUP_STATUS_COMPLETE` | Todas las notas afectadas convergieron: sin shares ni permisos administrados en `pending`/`failed` (`account_deletion.go:145`, `L241-258`) |
| `OK` + `ACCOUNT_CLEANUP_STATUS_PENDING` | Quedó trabajo: Drive o Social fallaron en alguna nota, o se agotó el contexto. El trabajo pendiente queda guardado y se puede repetir la llamada (`L131-143`) |
| `INTERNAL` "group share cleanup failed" | Estado `ERROR`: ids inválidos, repositorio no configurado o fallo de Postgres al leer/guardar el trabajo (`server.go:169-172`; `account_deletion.go:81-130`) |
| `INTERNAL` "internal auth not configured" | `INTERNAL_API_KEY` vacía en Notes (`auth.go:38-40`) |
| `UNAUTHENTICATED` | Sin metadata, sin `x-internal-key` o clave incorrecta (`auth.go:41-48`) |

**Efectos** (`NoteService.UnshareAccountGroup`, `account_deletion.go:79-146`), síncronos:

1. Junta las notas a procesar: el trabajo pendiente previo de `notes.account_deletion_group_notes` más todas las notas que hoy tienen un vínculo con `group_id` (`ListByGroup`, paginado de 100) (`L91-121`).
2. Guarda ese trabajo de forma durable antes de borrar nada (`SaveGroupWork`, `L122-125`; `back/notes/internal/repository/account_deletion_repository.go:102`). Así, si un borrado posterior falla, la nota no se pierde aunque su vínculo ya no aparezca en `ListByGroup`.
3. Para cada nota pendiente, bajo `pg_advisory_xact_lock` por nota (`cleanNoteShares`, `account_deletion.go:182-261`):
   - borra de `notes.shared_notes` los vínculos de ese grupo cuya nota es de `user_id`; si `group_deleted=true`, todos los vínculos de ese grupo (`L205-224`);
   - marca permisos `pending`, recalcula el estado deseado con los vínculos restantes y aplica Drive con el token del autor (`L225-240`). Así se revoca el correo de Google Drive del saliente en notas de **otros** autores que siguen compartidas con el grupo (Social ya no lo devuelve como miembro);
   - exige que no quede nada en `pending`/`failed` (`L241-258`); si queda, devuelve `PENDING`;
   - marca la nota completada (`CompleteGroupWork`, `L140`).
4. Los vínculos con otros grupos, `link`/`anyone` y visibilidad `public` se conservan.

Ver [COMUN §5](../COMUN-notes.md#5-ciclo-de-vida-de-los-permisos-de-drive).

**Diferencias con el HTTP deprecado:** el HTTP recorre solo las notas propias del usuario y luego reconverge el grupo sin trabajo durable, no tiene `group_deleted` y siempre responde 204; el RPC guarda trabajo por nota y devuelve `COMPLETE`/`PENDING`.

## Invocación (Front / Otros Servicios)

- **Frontend Flutter:** No se encontró invocación en el código revisado (el front no usa gRPC).
- **Social — salir / expulsar / banear** (best-effort, en goroutine):
  - `GroupService.unshareNotes` (`back/social/internal/service/group_service.go:602-613`) → `notesGRPCUnsharer.UnshareAll` (`back/social/internal/service/notes_grpc.go:64-82`): conecta a `NOTES_GRPC_URL` (por defecto `notes:9082`; `k8s/manifests.yaml:118`, `docker-compose.yml:109`), añade `x-internal-key`, envía `{user_id, group_id}` **sin** `group_deleted`, timeout 5 s (`notes_grpc.go:22-25`).
  - Solo mira si hubo error de transporte/estado gRPC; un `PENDING` cuenta como éxito y no se reintenta. Si hay error, cae al HTTP `POST /notes/unshare-all` (`group_service.go:610-612`).
  - Disparadores: `LeaveGroup` (`L578-590`), `KickMember` (`L525-536`), `BanMember` (`L543-554`) cuando el body de Social trae `cleanup_shared_notes: true`, y `HandleAccountDeletion` (`L648-664`). En el front, la casilla `CleanupSharedNotesCheckbox` de los diálogos de salir (`front/lib/features/groups/groups_screen.dart:590`) y expulsar/banear (`front/lib/features/groups/group_members_tab.dart:200`) envía ese flag a Social (`front/lib/core/services/social_service.dart:213,239,824`).
- **Social — borrado de cuenta** (durable):
  - `AccountDeletionService.Cleanup` (`back/social/internal/service/account_deletion.go:76-130`) llama `UnshareAll` por cada grupo pendiente con `group_deleted` real y timeout 30 s (`L101-106`). Cliente creado en `L41-49` con `NOTES_GRPC_ADDR` (`back/social/cmd/server/main.go:81`).
  - Solo da el grupo por terminado con `COMPLETE`; `PENDING` o `UNSPECIFIED` dejan el grupo pendiente para otra pasada; `ERROR` corta con error (`L110-117`).
  - Lo invoca `SocialServer.AccountDeletionCleanup` (`back/social/internal/handler/grpc/social_server.go:90-95`), que a su vez llama Auth durante `DELETE /auth/account` (ver [AccountDeletionCleanup](GRPC-notes-AccountDeletionCleanup.md)).

## Flujo

1. Social → `notes:9082` con `x-internal-key`.
2. `NewLogging` → `NewAuthRouter` → `NewInternalAuth` (`auth.go:36`).
3. `NotesServer.UnshareAll` (`server.go:168`) → `NoteService.UnshareAccountGroup` (`account_deletion.go:79`).
4. `AccountDeletionRepository.PendingGroupWork`/`SaveGroupWork` (`account_deletion_repository.go:102,118`) → `SharedStore.ListByGroup` (`back/notes/internal/service/store_pg.go:127`).
5. Por nota: `WithNoteLock` → `shared.Delete` → `MarkManagedPermissionsPending` → `ComputeDesiredDrivePermissions` (Social `GetMembers` por gRPC + `identity.oauth_connections`) → `syncDrivePermissions` (Drive síncrono) → verificación → `CompleteGroupWork` (`account_deletion_repository.go:137`).
6. Respuesta con `status`.

Finalmente devuelve `UnshareAllResponse{status}`, y Social utiliza ese resultado para dar por cerrada la tarea del grupo (borrado de cuenta, solo con `COMPLETE`) o, en salir/expulsar/banear, únicamente para decidir si debe caer al respaldo HTTP cuando hubo error.

## Tests

| Archivo:línea | Prueba | Qué comprueba | Tipo |
| --- | --- | --- | --- |
| `back/notes/internal/handler/grpc/server_test.go:181,190,201` | `TestUnshareAllWithoutInternalKeyIsUnauthenticated`, `TestUnshareAllWithUserJWTIsUnauthenticated`, `TestUnshareAllWithInvalidInternalKeyIsUnauthenticated` | Sin clave, con JWT o con clave errónea → `UNAUTHENTICATED` | Unitaria gRPC (bufconn) |
| `server_test.go:210` | `TestUnshareAllWithCorrectInternalKeySucceeds` | Clave correcta → OK | Unitaria gRPC |
| `back/notes/test/e2e_social_notes_test.go:306` | `TestE2EShareRestrictedThenUnshareAllCleansUp` | Share `restricted` y luego `UnshareAll` revoca permisos | E2E (bufconn, Social HTTP falso, Drive mock, repo de borrado en memoria) |
| `e2e_social_notes_test.go:365` | `TestE2EShareLinkThenUnshareAllRevokesLink` | Revoca `anyone` por ID | E2E |
| `e2e_social_notes_test.go:450` | `TestE2EUnshareAllIdempotentWithoutShares` | Idempotente sin vínculos | E2E |
| `e2e_social_notes_test.go:470` | `TestE2EUnshareAllOnlyAffectsTargetUser` | Solo afecta al usuario objetivo | E2E |
| `back/notes/internal/service/account_deletion_test.go:120,189` | `TestDeletedLastMemberGroupRemovesOnlyThatGroupShares`, `TestSurvivingGroupCleanupRevokesDepartingMemberFromOtherAuthorsNote` | `group_deleted` y revocación en notas de otros autores | Unitaria |
| `back/notes/internal/service/account_deletion_integration_test.go:117` | `TestNotesIntegrationDeletedGroupDriveFailureSurvivesRestartAndRetries` | Trabajo durable sobrevive a reinicio | Integración (`NOTES_TEST_DATABASE_URL`) |
| `back/social/test/cross_service_e2e_test.go:242,376` | `TestCrossServiceAccountDeletionRetriesPartialDriveCleanup`, `TestCrossServiceAccountDeletionCompletesNormallyAndCanRepeat` | Social → Notes `UnshareAll` entre procesos reales | E2E (`-tags=crossservice`) |
