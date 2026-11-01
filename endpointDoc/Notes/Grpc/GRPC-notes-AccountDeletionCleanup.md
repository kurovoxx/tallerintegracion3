# gRPC notes.v1.NotesService/AccountDeletionCleanup

RPC **interno** que Auth llama al final del borrado de una cuenta, después de que Social limpió membresías. Retira los vínculos que quedan en las notas del usuario y deja convergidos sus permisos de Drive. **No borra** las notas ni sus archivos/adjuntos en Drive: el código lo deja explícito como política pendiente de definición. Puerto `:9082`, solo red interna.

## Req

- **RPC:** `notes.v1.NotesService/AccountDeletionCleanup` (`back/proto/notes/v1/notes.proto:106-109`).
- **Mensaje de entrada** `AccountDeletionCleanupRequest{user_id}` (`notes.proto:340`). `user_id` UUID obligatorio; inválido → `INTERNAL`.
- **Autenticación:** `x-internal-key` = `INTERNAL_API_KEY` vía `NewAuthRouter` → `NewInternalAuth` (`back/notes/internal/interceptor/auth.go:99-104,36-51`). No acepta JWT.
- **Registro:** `back/notes/cmd/server/main.go:233-240`. Implementación `back/notes/internal/handler/grpc/server.go:184-190`.
- Requiere Postgres (repositorio de borrado); sin él responde `INTERNAL`.

## Res

`AccountDeletionCleanupResponse{status}` (`notes.proto:341`):

| Resultado | Significado |
| --- | --- |
| `OK` + `ACCOUNT_CLEANUP_STATUS_COMPLETE` | Todas las notas del usuario quedaron sin vínculos propios pendientes y con permisos convergidos; el tombstone pasa a `complete`. También si ya estaba completo (`back/notes/internal/service/account_deletion.go:50-52,68-72`) |
| `OK` + `ACCOUNT_CLEANUP_STATUS_PENDING` | Alguna nota no convergió (Drive/Social fallaron) o se agotó el contexto; se puede repetir (`L59-66`) |
| `INTERNAL` "account cleanup could not be verified" | `ERROR`: `user_id` inválido, repositorio ausente, fallo al crear/leer el tombstone o al listar notas, o fallo al marcar `complete` (`server.go:185-188`; `account_deletion.go:41-57,68-71`) |
| `UNAUTHENTICATED` / `INTERNAL` "internal auth not configured" | Problemas con `x-internal-key` |

**Efectos** (`NoteService.AccountDeletionCleanup`, `account_deletion.go:40-73`), síncronos:

1. `Begin` (crea el tombstone si faltaba; ver [BeginAccountDeletion](GRPC-notes-BeginAccountDeletion.md)).
2. Lista **todas** las notas del usuario (`listOwnedNotes`, paginado de 100, `L166-180`).
3. Para cada nota, bajo `pg_advisory_xact_lock` (`cleanNoteShares(…, groupID="", false)`, `L182-261`): borra todos sus vínculos en `notes.shared_notes`, marca permisos `pending`, recalcula el estado deseado (queda solo la visibilidad: si la nota es `public`, `anyone` se mantiene) y aplica Drive con el token del autor; revoca los correos de grupos `restricted` y el `anyone` que venía solo de `link`. Exige que todo quede `in_sync`.
4. `UPDATE notes.account_deletion_users SET status='complete'` (`back/notes/internal/repository/account_deletion_repository.go:91-93`).

Se conservan: filas de `notes.notes`, adjuntos, `.md` y adjuntos en Drive, likes y guardados. Ver [COMUN §5](../COMUN-notes.md#5-ciclo-de-vida-de-los-permisos-de-drive).

## Invocación (Front / Otros Servicios)

- **Frontend Flutter:** No se encontró invocación directa. Se dispara desde `DELETE /auth/account` en Auth (`back/auth/cmd/server/main.go:123`).
- **Auth:** `AccountDeletionCoordinator.DeleteAccount` (`back/auth/internal/service/account_deletion.go:64-104`) llama `cleanup.CleanupNotes` (`L95`) **después** de `CleanupSocial` → `GRPCAccountCleanup.CleanupNotes` (`back/auth/internal/service/account_deletion_grpc.go:100-108`): `x-internal-key`, timeout 30 s. Solo continúa (y borra la cuenta en Identity) si recibe `COMPLETE` (`requireCleanupComplete`); si no, registra `notes_cleanup_incomplete` y deja el borrado pendiente para el worker de Auth (`RunPendingOnce`/`RunWorker`, `account_deletion.go:160,177`).

## Flujo

1. Auth → `notes:9082` con `x-internal-key`.
2. `NewLogging` → `NewAuthRouter` → `NewInternalAuth`.
3. `NotesServer.AccountDeletionCleanup` (`server.go:184`) → `NoteService.AccountDeletionCleanup` (`account_deletion.go:40`).
4. `AccountDeletionRepository.Begin` → `NoteStore.ListByUser` → por nota `cleanNoteShares` (Social `GetMembers` por gRPC + `identity.oauth_connections` + Drive) → `AccountDeletionRepository.Complete`.
5. Respuesta con `status`.

Finalmente devuelve `AccountDeletionCleanupResponse{status}`, y Auth utiliza ese resultado para decidir si puede borrar definitivamente la cuenta en Identity (`COMPLETE`) o reintentar más tarde.

## Tests

| Archivo:línea | Prueba | Qué comprueba | Tipo |
| --- | --- | --- | --- |
| `back/notes/internal/service/account_deletion_test.go:69` | `TestAccountDeletionRevokesSharesButPreservesNoteContentAndOtherSources` | Revoca vínculos y conserva contenido y otras fuentes de acceso | Unitaria |
| `account_deletion_test.go:151` | `TestAccountDeletionReturnsPendingWhenDriveRevocationFails` | Fallo de Drive → `PENDING` | Unitaria |
| `back/notes/internal/service/account_deletion_retry_test.go:47` | `TestBeginAccountDeletionLogsCauseAndRetryCompletesAfterMigration` | `Cleanup` → `COMPLETE` tras migrar | Unitaria |
| `back/notes/internal/service/account_deletion_integration_test.go:195,233` | `TestNotesIntegrationAccountDeletionRevokesSharesAndRetriesDriveFailure`, `TestNotesIntegrationAccountDeletionKeepsTombstonePendingUntilDriveRetry` | Contra Postgres real: reintento y tombstone `pending` | Integración (`NOTES_TEST_DATABASE_URL`) |
| `back/social/test/cross_service_e2e_test.go:242,376` | `TestCrossServiceAccountDeletionRetriesPartialDriveCleanup`, `TestCrossServiceAccountDeletionCompletesNormallyAndCanRepeat` | `DELETE /auth/account` de punta a punta (202 pendiente / 200 completo) | E2E (`-tags=crossservice`) |

No hay prueba directa en el código revisado que llame este RPC por gRPC (bufconn) dentro de `back/notes`.
