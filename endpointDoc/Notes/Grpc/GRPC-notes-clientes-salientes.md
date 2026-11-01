# Notes como cliente: llamadas a Social e Identity (Auth)

Este documento no describe un endpoint de Notes: describe las llamadas que Notes **hace** a otros servicios mientras atiende sus propios endpoints. Las llamadas que otros servicios hacen **a** Notes están en [UnshareAll](GRPC-notes-UnshareAll.md), [BeginAccountDeletion](GRPC-notes-BeginAccountDeletion.md) y [AccountDeletionCleanup](GRPC-notes-AccountDeletionCleanup.md).

Los clientes generados viven en `back/notes/internal/gen/identityv1/` y `back/notes/internal/gen/socialv1/`. `back/notes/internal/gen/notesv1/v1/` es el código del propio servidor Notes (lo usan el servidor y los tests, no es un cliente hacia otro servicio).

## Req

### Notes → Social

- **Selección de transporte** (`newSocialAdapters`, `back/notes/cmd/server/main.go:160-172`): si `SOCIAL_GRPC_ADDR` tiene valor usa `SocialGRPCAdapter`; si no, `SocialHTTPAdapter` contra `SOCIAL_SERVICE_URL` (por defecto `http://social:8083`, `back/notes/internal/config/config.go:88-90`). `SOCIAL_GRPC_ADDR=social:9083` está definido en k8s (`k8s/manifests.yaml:75`) y en Compose (`docker-compose.yml:76`), así que en despliegue se usa **gRPC**.
- **Auth:** metadata `x-internal-key` = `INTERNAL_API_KEY`, timeout por llamada `SOCIAL_TIMEOUT` (5 s por defecto) (`back/notes/internal/service/social_grpc_adapter.go:52-55`; `config.go:85,94-100`). Conexión sin TLS, perezosa (`L36-43`).

| Método del adaptador | RPC en Social (`back/proto/social/v1/social.proto`) | Lo usa Notes en | Semántica |
| --- | --- | --- | --- |
| `IsMember` (`social_grpc_adapter.go:57-68`) | `SocialService/IsMember` (`L28`) | Lectura de notas `restricted` (`hasReadAccess`, `note_service.go:632`), `GetAccess` (`L2595`), share (`L1990`), listar notas de grupo (`L2730`) | `NOT_FOUND` = no miembro; otro error → la operación de Notes falla con 500 (falla cerrado) |
| `IsAdmin` (`L70-81`) | `SocialService/IsAdmin` (`L31`) | Share (`is_admin_note`, `L1997`, error ignorado), unshare por no-autor (`L2426`, error → 500), autorización de `POST /notes/unshare-all` (`IsGroupAdmin` `L2779-2782`, error → `false`) | `NOT_FOUND` = no admin |
| `ListMemberUserIDs` (`L92-109`) | `SocialService/GetMembers` (`L36`) | Correos para permisos `restricted` (`driveEmailDirectory`, `back/notes/internal/service/drive_email_directory.go:54`) | `NOT_FOUND` = grupo sin miembros; otro error → falla cerrado |
| `ListMemberEmails` (`L111-122`) | `SocialService/GetMemberEmails` (`L41`) | Solo si no hay `DriveEmailDirectory` (`note_service.go:2082-2085`). Con Postgres siempre se instala uno (`main.go:136-138`), así que **en producción no se llama** | — |
| `GetFollowersCount` (`L85-87`) | Ninguno | Snapshot de seguidores al compartir | Devuelve siempre 0 |

Respaldo HTTP (`back/notes/internal/service/social_adapter.go`): `GET /internal/groups/{id}/members`, `GET /internal/groups/{id}/member-emails` (ambas registradas en Social, `back/social/cmd/server/main.go:302-307`) y `GET /internal/users/{id}/followers-count` (`social_adapter.go:106`), que **no** está registrada en Social.

### Notes → Identity (Auth)

- **Token de Drive:** `GRPCOAuthTokenProvider.GetValidAccessToken` (`back/notes/internal/drive/oauth_grpc.go:62-89`) llama `identity.v1.IdentityService/GetOAuthToken{user_id, provider=PROVIDER_GOOGLE_DRIVE}` a `IDENTITY_GRPC_ADDR` (por defecto `auth:9080`, `config.go:91-93`; k8s `manifests.yaml:77`), con `x-internal-key` y timeout 5 s (`L36-46`). Se usa en **cada** llamada a Drive con el usuario dueño del archivo (`RealDriveClient.serviceFor`, `back/notes/internal/drive/real_client.go:42`). Solo con `STORAGE_MODE=drive` (`main.go:74-103`).
- **Correo de la cuenta Google Drive de los miembros:** Notes **lee directamente** la tabla de Identity `identity.oauth_connections` (columna `external_account_email`, `provider='google_drive'`, `revoked_at IS NULL`) con su propio pool de Postgres (`PGDriveEmailStore.GoogleDriveEmails`, `drive_email_directory.go:91-133`). No pasa por la API de Auth. Solo lee esa columna, nunca tokens.
- **Identidad del autor:** sale del JWT validado localmente (HS256 con `JWT_SECRET`, sin llamar a Auth). No se encontró en Notes ninguna búsqueda de usuarios en Auth (`/internal/users/*` de Auth o RPC equivalente).

## Res

| Llamada | Respuesta usada | Traducción a errores de Notes |
| --- | --- | --- |
| `IsMember`/`IsAdmin` | `is_member` / `is_admin` | `NOT_FOUND` → `false`; otro error → `ErrInternalDatabase` (HTTP 500 / gRPC `INTERNAL`) |
| `GetMembers` | `members[].user_id` | Error → no se crea/converge el share (HTTP 500); en reconciliación queda `failed` |
| `GetOAuthToken` | `access_token` | `NOT_FOUND` o `UNAUTHENTICATED` con mensaje `drive_connection_invalid` → `*drive.OAuthError` (HTTP 403 "Conecte o renueve su Google Drive", salvo en lectura, donde se vuelve 404 `note_unavailable`); token vacío → `DriveError 403`; otro error → `DriveError 500` (HTTP 502) (`oauth_grpc.go:73-88`) |
| SELECT `identity.oauth_connections` | mapa `user_id → correo` | Error → falla cerrado (no se otorgan permisos) |

## Invocación (Front / Otros Servicios)

Estas llamadas las hace Notes; no tienen consumidor externo. Se disparan desde los endpoints de Notes indicados en la tabla anterior (lectura, acceso, share, unshare, unshare-all, listado de grupo, PATCH con cambio de visibilidad, `UnshareAll`/`AccountDeletionCleanup` por gRPC y el reconciliador).

## Flujo

Ejemplo (share `restricted`):

1. `POST /notes/:id/share` → `NoteService.shareLocked` (`note_service.go:1975`).
2. `SocialGRPCAdapter.IsMember` → `social:9083` (síncrono).
3. `ComputeDesiredDrivePermissions` → `driveEmailDirectory.ListMemberDriveEmails` → `SocialGRPCAdapter.ListMemberUserIDs` (`GetMembers`) → SELECT en `identity.oauth_connections`.
4. `syncDrivePermissions` → `RealDriveClient.GrantPermission` → `GRPCOAuthTokenProvider.GetValidAccessToken` → `auth:9080` → Google Drive API.

Finalmente cada llamada devuelve su dato (membresía, ids, correo o token), y el servicio de Notes lo utiliza para autorizar la operación del usuario o para calcular y aplicar los permisos de Drive.

## Tests

| Archivo:línea | Prueba | Qué comprueba | Tipo |
| --- | --- | --- | --- |
| `back/notes/internal/service/social_grpc_adapter_test.go:83` | `TestSocialGRPCAdapter` | Mapeo de `IsMember`/`IsAdmin`/`GetMembers`/`GetMemberEmails` contra un servidor Social falso | Unitaria (gRPC en memoria) |
| `social_grpc_adapter_test.go:112` | `TestSocialGRPCAdapterWrongKeyFailsClosed` | Clave interna incorrecta → error | Unitaria |
| `back/notes/internal/service/social_adapter_test.go:14,62,77,98` | `TestSocialHTTPAdapterContract`, `…NotFoundDegradesToEmpty`, `…ErrorsAreBounded`, `…MalformedPayloadFailsClosed` | Respaldo HTTP | Unitaria (`httptest`) |
| `back/notes/internal/service/drive_emails_test.go:29,74,197,238` | `TestRestrictedUsesGoogleEmailsNotAppEmails`, `TestRestrictedMemberWithoutDriveIsSkipped`, `TestRestrictedDirectoryErrorFailsClosed`, `TestDriveEmailDirectoryComposite` | Correo de Google Drive y falla cerrado | Unitaria (store falso) |
| `back/notes/internal/drive/oauth_grpc_test.go:87,100,144,197` | `TestGRPCOAuthTokenProvider_TokenValido`, `_Errores`, `_Recuperacion`, `_MockSinIdentity` | Token y mapeo de errores de Identity | Unitaria |
| `back/notes/test/e2e_social_notes_test.go:306-496` | `TestE2E*` | Notes → Social por el adaptador **HTTP** contra un Social falso | E2E (bufconn) |
| `back/social/test/cross_service_e2e_test.go:125` | `TestCrossServiceMembershipLifecycle` | Notes → Social real entre procesos | E2E (`-tags=crossservice`) |

No hay prueba directa en el código revisado de `PGDriveEmailStore.GoogleDriveEmails` contra Postgres real.
