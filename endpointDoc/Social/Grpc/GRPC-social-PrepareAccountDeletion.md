# gRPC `social.v1.SocialService/PrepareAccountDeletion`


## Req

- RPC `social.v1.SocialService/PrepareAccountDeletion`, proto `back/proto/social/v1/social.proto:43-52`.
- Request: user_id UUID. No JWT; metadata `x-internal-key` de servicio a servicio. Social aplica la clave a todos sus RPC.
- Lo invoca Identity antes de comenzar la limpieza que retira membresías.

## Res

- OK con status ACCOUNT_CLEANUP_STATUS_PENDING al abrir el tombstone durable; COMPLETE si ya terminó; ERROR no autoriza a Identity.
- `INVALID_ARGUMENT` con UUID malformado; `UNAUTHENTICATED` por clave ausente/mala; `UNIMPLEMENTED` si no está configurado el coordinador durable; otros errores se mapean a status gRPC.
- El gate impide nuevas acciones de la cuenta durante y después de la limpieza.
- La operación puede repetirse: Begin/Prepare persiste una sola operación.

## Invocación (Front / Otros Servicios)

- Identity llama desde `back/auth/internal/service/account_deletion_grpc.go:62`, `BeginSocial`.
- No hay llamada desde Flutter.

## Flujo

1. `startGRPC` registra `SocialService` con logging y `NewInternalAuth`; la clave `x-internal-key` se exige antes de ejecutar RPCs (`back/social/cmd/server/main.go:146-160`; `back/social/internal/interceptor/auth.go:30-47`).
2. `SocialServer.PrepareAccountDeletion` valida el UUID y llama al coordinador (`back/social/internal/handler/grpc/social_server.go:123-135`).
3. `AccountDeletionService.Prepare` persiste el gate con `AccountDeletionRepository.Begin` y consulta si ya figura completo (`back/social/internal/service/account_deletion.go:62-74`; `back/social/internal/repository/account_deletion_repository.go:97-127,215-222`). `Begin` crea el tombstone y captura las membresías antes de que puedan desaparecer (`back/social/internal/repository/account_deletion_repository.go:118-127`).
4. El handler responde el estado explícito. Identity acepta PENDING como gate inicial, pero no borra localmente al usuario hasta recibir COMPLETE de las limpiezas posteriores (`back/social/internal/handler/grpc/social_server.go:130-135`; `back/auth/internal/service/account_deletion_grpc.go:62-79`).

## Tests

- `back/social/internal/service/group_service_test.go:740-796` cubre idempotencia/operaciones de cuenta por grupos; `back/social/internal/service/account_deletion_stream_integration_test.go:14` cubre reinicio y recuperación de la operación.
- `back/auth/internal/service/account_deletion_test.go` cubre que begin pendiente no se confunde con complete.
- No se encontró una prueba dedicada del wire protocol `PrepareAccountDeletion`; la cobertura citada comprueba servicio y flujo de recuperación, no el contrato gRPC completo.
