# gRPC `notes.v1.NotesService/BeginAccountDeletion`


## Req

- RPC `notes.v1.NotesService/BeginAccountDeletion`: `back/proto/notes/v1/notes.proto:101-104`, `request` en :338.
- Request user_id UUID. Metadata `x-internal-key`, no JWT de usuario; NewAuthRouter enruta explícitamente este RPC al interceptor interno en `back/notes/internal/interceptor/auth.go:99-107`.
- Escribe el tombstone antes de Social cleanup, bloquea nuevas operaciones normales y espera solicitudes en vuelo según el gate.

## Res

- OK con status COMPLETE si ya terminó, PENDING si se registra el tombstone; ERROR se convierte en gRPC `INTERNAL`.
- INVALID UUID/precondición o falta de estado durable resulta en error; clave ausente/mala produce `UNAUTHENTICATED`.
- No borra contenido ni credenciales OAuth.
- La ruta no expone una respuesta HTTP equivalente desde Flutter.

## Invocación (Front / Otros Servicios)

- Identity llama por gRPC con `x-internal-key`: `back/auth/internal/service/account_deletion_grpc.go:52`, `BeginNotes`.
- No hay consumidor frontend.

## Flujo

1. `main()` registra `NotesService` con logging y `NewAuthRouter`; `BeginAccountDeletion` se enruta a `x-internal-key` (`back/notes/cmd/server/main.go:231-240`; `back/notes/internal/interceptor/auth.go:96-107`).
2. `NotesServer.BeginAccountDeletion` pasa el UUID al servicio y convierte ERROR en gRPC `INTERNAL` (`back/notes/internal/handler/grpc/server.go:176-182`).
3. `NoteService.BeginAccountDeletion` valida precondiciones y llama `AccountDeletionRepository.Begin`, que persiste el tombstone en `notes.account_deletion_users` (`back/notes/internal/service/account_deletion.go:21-35`; `back/notes/internal/repository/account_deletion_repository.go:68-89`).
4. El RPC responde COMPLETE si ya existe un borrado completo o PENDING al abrir el gate. Identity trata PENDING solo como inicio del gate y exige completar las limpiezas antes del borrado local (`back/notes/internal/handler/grpc/server.go:176-182`; `back/auth/internal/service/account_deletion_grpc.go:52-79`).

## Tests

- `back/notes/internal/service/account_deletion_retry_test.go:47`, `tombstone` y retry tras configurar persistencia.
- `back/notes/internal/service/account_deletion_integration_test.go:233`, `tombstone` queda pendiente hasta retry de Drive.
- `back/notes/internal/handler/grpc/server_test.go:181-220` prueba autenticación interna de `UnshareAll`; no se encontró un test wire-level dedicado para `BeginAccountDeletion`.
