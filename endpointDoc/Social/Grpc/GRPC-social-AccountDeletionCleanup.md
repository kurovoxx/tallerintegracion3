# gRPC `social.v1.SocialService/AccountDeletionCleanup`


## Req

- RPC: `social.v1.SocialService/AccountDeletionCleanup`, declarado en `back/proto/social/v1/social.proto:43-48`, 107-120.
- Request: user_id UUID. El proto mantiene user_token opcional por compatibilidad/constructor legacy; Identity de la versión revisada llama con user_id solamente (`back/auth/internal/service/account_deletion_grpc.go:81-85`).
- Metadata `x-internal-key`; no es endpoint HTTP ni llamada Flutter.
- RPC repetible. Es posterior a PrepareAccountDeletion.

## Res

- OK: groups[] contiene group_id y, si aplica, promoted_user_id o group_deleted; status solo autoriza avance si es ACCOUNT_CLEANUP_STATUS_COMPLETE.
- PENDING significa que quedan tareas por grupo; ERROR/status gRPC indica error; UNSPECIFIED de servidores antiguos nunca autoriza borrado local.
- El RPC conserva operación por grupo hasta confirmar membresía, revocación Notes y retirada Stream; el grupo del último miembro conserva su ID de limpieza aunque el grupo ya no exista.
- No hay transacción distribuida entre grupos/proveedores.

## Invocación (Front / Otros Servicios)

- Identity invoca `back/auth/internal/service/account_deletion_grpc.go:81`, `CleanupSocial`. No hay consumidor Flutter.
- `AccountDeletionService.Cleanup` llama a Notes `NotesService/UnshareAll` con `x-internal-key` por grupo (`back/social/internal/service/account_deletion.go:101-116`; `back/social/internal/service/notes_grpc.go:62-82`) y a `ChatMembership.RemoveMember` de Stream (`back/social/internal/service/account_deletion.go:118-130`; `back/social/internal/stream/client.go:314-337`). Son llamadas servicio-a-servicio, no el endpoint HTTP de usuario.

## Flujo

1. `startGRPC` registra el servicio con autenticación interna; `SocialServer.AccountDeletionCleanup` valida UUID y delega (`back/social/cmd/server/main.go:146-160`; `back/social/internal/handler/grpc/social_server.go:90-104`).
2. `AccountDeletionService.Cleanup` asegura el gate y lee grupos pendientes desde PostgreSQL (`back/social/internal/service/account_deletion.go:76-89`; `back/social/internal/repository/account_deletion_repository.go:97-127,155-170`). Para cada grupo retira membresía con sucesión y persiste ese avance (`back/social/internal/service/account_deletion.go:90-100`; `back/social/internal/repository/account_deletion_repository.go:172-179`).
3. El coordinador pide revocar los shares de Notes y espera `COMPLETE` (`back/social/internal/service/account_deletion.go:101-117`; `back/social/internal/service/notes_grpc.go:62-82`). Luego solicita retirar al usuario de Stream; la respuesta específica que confirma que el canal no existe se trata como ya retirado, mientras otros errores dejan la tarea pendiente (`back/social/internal/service/account_deletion.go:118-134`; `back/social/internal/stream/client.go:314-337`).
4. Solo después de persistir `stream_removed` completa cada grupo y, cuando no quedan pendientes, la operación (`back/social/internal/service/account_deletion.go:128-139`; `back/social/internal/repository/account_deletion_repository.go:181-212`). `SocialServer` devuelve el estado; Identity no borra la cuenta hasta obtener COMPLETE (`back/social/internal/handler/grpc/social_server.go:95-104`; `back/auth/internal/service/account_deletion_grpc.go:81-98`).

## Tests

- `back/social/internal/service/account_deletion_stream_integration_test.go:14`, `TestIntegrationAccountDeletionStreamFailureSurvivesServiceRestart`.
- `back/social/internal/service/group_chat_cleanup_test.go:213`, `TestDurableAccountDeletionConfirmsChatRemovalBeforeCompletion`; :236, retry tras reinicio.
- `back/social/internal/service/group_service_test.go:661`, `sucesi`ón; :712, último miembro; :740, idempotencia; :805, cleanup Notes.
- `back/auth/internal/service/account_deletion_integration_test.go:48` verifica que Identity espera COMPLETE.
