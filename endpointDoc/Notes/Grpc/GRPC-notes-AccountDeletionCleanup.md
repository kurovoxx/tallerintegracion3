# gRPC `notes.v1.NotesService/AccountDeletionCleanup`


## Req

- `notes.v1.NotesService/AccountDeletionCleanup`: `back/proto/notes/v1/notes.proto:106-109`; request user_id en :340-341.
- Metadata `x-internal-key`. NewAuthRouter aplica el interceptor interno a este método en `back/notes/internal/interceptor/auth.go:99-107`. No usa el JWT que caduca del usuario.
- Lo invoca Identity después de que Notes y Social confirmen sus gates y que Social haya limpiado los grupos.

## Res

- OK: status COMPLETE solamente cuando los shares propios requeridos y permisos administrados están retirados; PENDING conserva trabajo que requiera retry; ERROR se mapea a gRPC `INTERNAL`.
- INVALID UUID/estado inválido y clave ausente/mala son errores gRPC.
- Los apuntes propios y sus adjuntos se conservan. La operación retira accesos compartidos de la cuenta y no borra el contenido de Drive.
- Repetible; OAuth del usuario debe seguir almacenado mientras se consultan o revocan permisos Drive.

## Invocación (Front / Otros Servicios)

- Identity invoca `back/auth/internal/service/account_deletion_grpc.go:100`, `CleanupNotes`.
- Social invoca el RPC distinto `UnshareAll` por grupo desde `AccountDeletionService.Cleanup` (`back/social/internal/service/account_deletion.go:101-116`), con el adaptador `notesGRPCUnsharer.UnshareAll` (`back/social/internal/service/notes_grpc.go:62-82`); no debe confundirse con esta limpieza general de shares.
- No hay llamada Flutter.

## Flujo

1. `main()` registra `NotesService` con un interceptor que enruta este RPC a `x-internal-key` (`back/notes/cmd/server/main.go:231-240`; `back/notes/internal/interceptor/auth.go:96-107`).
2. `NotesServer.AccountDeletionCleanup` recibe `user_id` y convierte ERROR en gRPC `INTERNAL` (`back/notes/internal/handler/grpc/server.go:184-190`).
3. `NoteService.AccountDeletionCleanup` asegura el tombstone, lista apuntes propios, recalcula y revoca permisos requeridos, y termina solo cuando cada permiso queda limpio (`back/notes/internal/service/account_deletion.go:37-73`).
4. `AccountDeletionRepository.Begin` y `Complete` conservan el estado de la operación; para limpiezas por grupo, `SaveGroupWork`/`PendingGroupWork`/`CompleteGroupWork` registran IDs pendientes de Drive incluso si las filas de share ya se borraron (`back/notes/internal/repository/account_deletion_repository.go:68-94,102-140`). El RPC devuelve COMPLETE/PENDING; ante fallo Identity conserva usuario y OAuth.

## Tests

- `back/notes/internal/service/account_deletion_test.go:69`, `TestAccountDeletionRevokesSharesButPreservesNoteContentAndOtherSources`.
- `back/notes/internal/service/account_deletion_integration_test.go:195`, `TestNotesIntegrationAccountDeletionRevokesSharesAndRetriesDriveFailure`; :233, tombstone pendiente hasta retry.
- `back/auth/internal/service/account_deletion_integration_test.go:48` valida el gate de borrado local hasta COMPLETE.
