# gRPC notes.v1.NotesService/BeginAccountDeletion

RPC **interno** que Auth (Identity) llama al empezar el borrado de una cuenta. Crea un tombstone durable en Notes y espera a que terminen las peticiones de ese usuario que ya estaban en curso. Desde ese momento los JWT del usuario dejan de servir en Notes (HTTP 423 / gRPC `PERMISSION_DENIED`). Puerto `:9082`, solo red interna.

## Req

- **RPC:** `notes.v1.NotesService/BeginAccountDeletion` (`back/proto/notes/v1/notes.proto:101-104`).
- **Mensaje de entrada** `BeginAccountDeletionRequest{user_id}` (`notes.proto:338`). `user_id` UUID obligatorio; inválido → `INTERNAL` (`back/notes/internal/service/account_deletion.go:22-25,148-151`).
- **Autenticación:** `x-internal-key` = `INTERNAL_API_KEY` vía `NewAuthRouter` → `NewInternalAuth` (`back/notes/internal/interceptor/auth.go:99-104,36-51`). No acepta JWT.
- **Registro:** `back/notes/cmd/server/main.go:233-240`. Implementación `back/notes/internal/handler/grpc/server.go:176-182`.
- Requiere Postgres (repositorio de borrado, `main.go:125,129`); sin él responde `INTERNAL`.

## Res

`BeginAccountDeletionResponse{status}` (`notes.proto:339`):

| Resultado | Significado |
| --- | --- |
| `OK` + `ACCOUNT_CLEANUP_STATUS_PENDING` | Tombstone creado (o ya existía en estado `pending`) (`account_deletion.go:34`) |
| `OK` + `ACCOUNT_CLEANUP_STATUS_COMPLETE` | El borrado de Notes ya estaba completo (`L31-33`) |
| `INTERNAL` "account deletion gate unavailable" | `ERROR`: `user_id` inválido, repositorio no configurado o fallo de Postgres (`server.go:177-180`) |
| `UNAUTHENTICATED` / `INTERNAL` "internal auth not configured" | Problemas con `x-internal-key` (`auth.go:38-48`) |

**Efectos** (`AccountDeletionRepository.Begin`, `back/notes/internal/repository/account_deletion_repository.go:68-89`):

1. Toma `pg_advisory_lock` exclusivo con la clave del usuario (`L77`). Cada petición autenticada de ese usuario sostiene el mismo lock en modo compartido mientras se ejecuta (`AcquireRequest`, `L42-66`, usado por el middleware HTTP y el interceptor JWT), así que `Begin` espera a que terminen las escrituras en curso.
2. `INSERT INTO notes.account_deletion_users(user_id,status) VALUES(…,'pending') ON CONFLICT DO NOTHING` (`L82`).
3. Devuelve si `status='complete'` (`L87`).

A partir de aquí, `AcquireRequest` ve el tombstone y el usuario recibe **423** `account_deletion_in_progress` en HTTP (`back/notes/internal/middleware/auth_middleware.go:128-131`) o `PERMISSION_DENIED` "account deletion in progress" en gRPC (`auth.go:87-89`).

No toca notas, vínculos ni Drive.

## Invocación (Front / Otros Servicios)

- **Frontend Flutter:** No se encontró invocación directa. El usuario inicia el borrado con `DELETE /auth/account` en Auth (`back/auth/cmd/server/main.go:123`); el front lo llama desde `front/lib/core/services/account_deletion_service.dart`.
- **Auth:** `AccountDeletionCoordinator.DeleteAccount` (`back/auth/internal/service/account_deletion.go:64-104`) llama primero `cleanup.BeginNotes` (`L83`) → `GRPCAccountCleanup.BeginNotes` (`back/auth/internal/service/account_deletion_grpc.go:52-60`): `x-internal-key`, timeout 30 s (`L123-129`). Acepta `PENDING` o `COMPLETE` (`requireCleanupAccepted`); cualquier otra cosa registra `notes_begin_incomplete` y corta (`L83-86`). El cliente se crea en `back/auth/cmd/server/main.go:69` con la dirección gRPC de Notes.
- Orden completo en Auth: `BeginNotes` → `BeginSocial` → `CleanupSocial` → `CleanupNotes` → borrar Identity (`account_deletion.go:83-102`).

## Flujo

1. Auth → `notes:9082` con `x-internal-key`.
2. `NewLogging` → `NewAuthRouter` → `NewInternalAuth`.
3. `NotesServer.BeginAccountDeletion` (`server.go:176`) → `NoteService.BeginAccountDeletion` (`account_deletion.go:21-35`) → `AccountDeletionRepository.Begin`.
4. Respuesta con `status`.

Finalmente devuelve `BeginAccountDeletionResponse{status}`, y Auth utiliza ese resultado para continuar el borrado (con `PENDING`/`COMPLETE`) o dejarlo pendiente para reintentar.

## Tests

| Archivo:línea | Prueba | Qué comprueba | Tipo |
| --- | --- | --- | --- |
| `back/notes/internal/service/account_deletion_retry_test.go:47` | `TestBeginAccountDeletionLogsCauseAndRetryCompletesAfterMigration` | `ERROR` sin tabla migrada; tras migrar, `PENDING` y luego `COMPLETE` | Unitaria |
| `account_deletion_retry_test.go:92` | `TestAccountDeletionLogsInvalidInputWithoutPersonalData` | `user_id` inválido → `ERROR` sin datos personales en el log | Unitaria |
| `back/notes/internal/service/account_deletion_integration_test.go:40` | `TestNotesIntegrationDeletionFenceDrainsInflightRequestAndRejectsLaterWrites` | Espera la petición en curso y bloquea las posteriores | Integración (`NOTES_TEST_DATABASE_URL`) |
| `account_deletion_integration_test.go:78` | `TestNotesIntegrationAccountGateDoesNotConsumeBusinessPool` | El gate usa un pool propio | Integración |
| `back/notes/internal/repository/account_deletion_schema_test.go:41,67` | `TestVerifyAccountDeletionSchema_*` | Notes no arranca sin las tablas (`main.go:62-64`) | Unitaria / Integración |
| `back/social/test/cross_service_e2e_test.go:242,376` | `TestCrossServiceAccountDeletion*` | Flujo Auth → Notes/Social entre procesos | E2E (`-tags=crossservice`) |

No hay prueba directa en el código revisado que llame este RPC por gRPC (bufconn) dentro de `back/notes`; las pruebas de Notes llaman al servicio.
