# DELETE /auth/account

Borrado de la cuenta propia, orquestado por Identity (Auth) hacia Notes y Social por gRPC. Público vía nginx como `/api-auth/auth/account`.

## Req
- Método: `DELETE`, ruta: `/auth/account` (`back/auth/cmd/server/main.go:120`, middleware `RequireIdentity`).
- Headers requeridos: `Authorization: Bearer <access_token>`. Sin body ni query: el usuario sale **solo** del JWT.
- No pide contraseña ni token extra; la confirmación explícita es responsabilidad del cliente (el frontend exige escribir `ELIMINAR`).
- `RequireIdentity` valida firma y expiración pero **no** exige cuenta activa: el mismo JWT sirve para reanudar un borrado ya iniciado.

## Res
- `200`: `{"status":"complete"}`. Notes, Social e Identity confirmaron; la fila `identity.users` ya no existe (cascada: perfil, refresh tokens, password resets y OAuth). Repetir la llamada con el mismo JWT devuelve `200` otra vez (idempotente).
- `202`: `{"status":"pending"}`. Hay un tombstone durable en `identity.account_deletions`: la cuenta quedó bloqueada y un worker (`RunWorker`, cada 15 s) reintenta las etapas sin necesitar el JWT. Corresponde a `ErrAccountCleanupPending`, `ErrAccountCleanupFailed` o `ErrInvalidCleanupResult` (servicio antiguo que responde `UNSPECIFIED`).
- `404`: `{"error":"account_not_found"}`. No hay usuario ni tombstone para ese `user_id`.
- `503`: `{"error":"account_deletion_unavailable"}`. Fallo de lock/estado o del commit local final. **Puede haber comenzado** (p. ej. Social y Notes limpios y falla `CompleteAndDelete`): consultar `GET /auth/account-deletion` antes de reintentar.
- `401`: `{"error":{"code":"unauthorized|invalid_token|token_expired","message":"..."}}` (middleware).
- No devuelve `403` ni `409`.

## Efectos mientras está pendiente o completo
- Rutas con `RequireAuth` en Auth y Notes: `423 {"error":{"code":"account_deletion_in_progress",...}}`.
- `POST /auth/refresh` → `401 invalid_token`; `POST /auth/login` → `401 invalid_credentials`. No hay forma de cancelar el borrado.

## Flujo
`AccountDeletionHandler.Delete` (`internal/handler/http/account_deletion_handler.go`) → `AccountDeletionCoordinator.DeleteAccount` (`internal/service/account_deletion.go`), bajo lock exclusivo por usuario:
1. `Begin`: tombstone `pending` (espera a que terminen las peticiones en curso del usuario: lock compartido en `AcquireRequest`).
2. Notes `BeginAccountDeletion` (gRPC `notes:9082`): tombstone Notes, bloquea escrituras.
3. Social `PrepareAccountDeletion` (gRPC `social:9083`): tombstone y snapshot de membresías.
4. Social `AccountDeletionCleanup`: sale de cada grupo (sucesión de admin; si era el último miembro el grupo se borra), borra baneos, llama a Notes `UnshareAll` por grupo y quita al usuario del canal Stream.
5. Notes `AccountDeletionCleanup`: retira los shares restantes y reconcilia permisos de Drive. **Conserva** las notas, adjuntos y archivos Markdown en el Drive del autor (sin regla de producto que autorice borrarlos).
6. `CompleteAndDelete`: marca `complete` y borra `identity.users` en una transacción. El tombstone queda para invalidar JWT ya emitidos.

Llamadas gRPC con `x-internal-key` y 30 s de timeout cada una (`account_deletion_grpc.go`). Solo `COMPLETE` autoriza avanzar.

## Requisito de esquema
Necesita `identity.account_deletions` (Auth `004_account_deletion.sql`), `notes.account_deletion_users` y `notes.account_deletion_group_notes` (Notes `003_account_deletion.sql`), `social.account_deletion_users` y `social.account_deletion_groups` (Social `002_account_deletion.sql`) con `stream_removed` (Social `003_account_deletion_stream.sql`). `init.sql` solo corre con un volumen nuevo: en bases existentes hay que aplicar esas migraciones. Auth, Notes y Social verifican estas tablas al arrancar (`repository.VerifyAccountDeletionSchema`) y no arrancan si faltan. Síntoma si faltan las de Notes en un proceso ya iniciado: `202 pending` permanente con `last_error = notes_begin_incomplete` y, en Notes, `borrado de cuenta [begin] ... relation "notes.account_deletion_users" does not exist`.

## Tests
- `back/auth/internal/handler/http/account_deletion_contract_test.go`: códigos/cuerpos 200 (idempotente), 202 por etapa pendiente/error/`UNSPECIFIED`, 404, 503 (incluido el 503 con borrado ya iniciado), 401 y 423 posterior en `RequireAuth`.
- `back/auth/internal/service/account_deletion_test.go`: confirmación explícita en cada etapa, reintento tras fallo del commit local, compatibilidad con servidores antiguos.
- `back/auth/internal/service/account_deletion_integration_test.go` (`AUTH_TEST_DATABASE_URL`): cascadas, bloqueo de login/refresh, gate de peticiones en vuelo.
- `back/social/test/cross_service_e2e_test.go` (`-tags=crossservice`): `TestCrossServiceAccountDeletionRetriesPartialDriveCleanup`, `TestCrossServiceAccountDeletionCompletesNormallyAndCanRepeat`.
- Frontend: `front/test/account_deletion_service_test.dart`, `front/test/account_deletion_flow_test.dart`.
