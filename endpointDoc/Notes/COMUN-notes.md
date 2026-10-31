# Notes: referencia común

Información compartida por todos los documentos de `endpointDoc/Notes/`. Cada documento la enlaza en lugar de repetirla. Todo lo de aquí se verificó contra el código del repo (rama `Development`).

## 1. Rutas internas y prefijos externos

| Capa | Dirección | Fuente |
| --- | --- | --- |
| Backend HTTP (Gin) | `:8082` (`NOTES_PORT` → `PORT` → `8082`). Rutas **sin** prefijo: `/notes`, `/notes/:id`, … | `back/notes/internal/config/config.go:76,108-113`; `back/notes/cmd/server/main.go:177-215` |
| Backend gRPC | `:9082` (`NOTES_GRPC_PORT` → `GRPC_PORT` → `9082`) | `config.go:77,114-116`; `main.go:274-281` |
| Ingress público | `https://ti3-brojas.dev.censei.cl/` → Service `front:80` (nginx del front) | `k8s/ingress.yaml:13-19` |
| nginx del front | `location /api-notes/` reescribe `^/api-notes/(.*)$` → `/$1` y reenvía a `notes:8082` | `front/nginx.conf:36-40` |
| Base URL del front (web y desktop) | `NOTES_BASE_URL`, por defecto `https://ti3-brojas.dev.censei.cl/api-notes` | `front/lib/core/services/api_config.dart:10-13,27` |
| Base URL de `AllNotesScreen` en Android | `http://10.0.2.2:8082` (directo al backend, **sin** `/api-notes`) | `front/lib/features/notes/all_notes_screen.dart:30-35` |

- Ejemplo: la ruta backend `POST /notes/:id/share` se llama desde el navegador como `POST https://ti3-brojas.dev.censei.cl/api-notes/notes/<id>/share`.
- nginx solo bloquea `/api-auth/internal/` y `/api-academic/internal/` (`front/nginx.conf:11-12`). **No** hay bloqueo para `/api-notes/`: todas las rutas HTTP de Notes, incluida `POST /notes/unshare-all`, son alcanzables desde fuera con un JWT válido.
- El gRPC (`:9082`) no pasa por nginx ni por el Ingress. Solo se expone dentro del cluster (Service `notes`, puerto `grpc` 9082, `k8s/manifests.yaml:82-87`) y en Compose (`docker-compose.yml:61`, `9082:9082`).
- Notes **no registra rutas `/internal/*`** propias (`main.go:177-215`).

## 2. Autenticación HTTP

Todas las rutas salvo `/health` y `/health/drive` están en el grupo `protected` con `authMw.RequireAuth()` (`main.go:191-213`).

- Validador: `SimpleHS256Validator` con `JWT_SECRET` (fallback `JWT`, luego `dev-jwt-secret-change-me`), `Issuer: "apuntes-auth"`, `Audience: "apuntes-client"` (`main.go:223-227`; `config.go:117-123`; `back/notes/internal/middleware/auth_middleware.go:44-72`). Exige el claim `user_id` no vacío.
- Middleware `RequireAuth` (`auth_middleware.go:91-142`):

| HTTP | `error.code` | Condición |
| --- | --- | --- |
| 401 | `unauthorized` | Falta `Authorization`, no empieza por `Bearer ` o el token está vacío (`L94-108`) |
| 401 | `token_expired` | El error de validación contiene "expired" (`L113-114`) |
| 401 | `invalid_token` | Cualquier otro fallo de firma, issuer, audience o claim (`L115-116`) |
| 503 | `account_status_unavailable` | El gate de borrado de cuenta no pudo consultarse (`L123-126`) |
| 423 | `account_deletion_in_progress` | La cuenta tiene un tombstone de borrado activo (`L128-131`) |

- El gate de borrado (`deletionRepo`) solo existe con Postgres (`main.go:125,139`). En modo memoria (`DATABASE_URL` vacío) se pasa `nil` (`main.go:153`) y no hay 423/503.
- El `user_id` del token se publica en el contexto (`L135-139`); los handlers lo leen con `middleware.GetUserID` (`L144-156`). **Nunca** se toma el usuario del body para autorizar, salvo `unshare-all`, donde `user_id` es el usuario objetivo (ver su documento).
- No se valida rol: el claim `role` se guarda en el contexto pero ningún handler de Notes lo usa.

## 3. Formato de error HTTP

`{"error":{"code":"<snake_case>","message":"<texto>"}}` (`back/notes/internal/utils/response.go:9-22`).

`handleServiceError` (`back/notes/internal/handler/http/note_handler.go:890-942`) decide el código en este orden:

1. `*drive.OAuthError` → **403** `forbidden` "Conecte o renueve su Google Drive" (`L893-897`).
2. `*service.ServiceError`:
   - `ErrDriveUnavailable` → **502** `internal_error` "Servicio de almacenamiento no disponible" (`L903-905`).
   - `ErrSourceAccessDenied` → **404** `not_found` "Nota no encontrada" (`L906-912`).
   - `ErrInternalDatabase` / `ErrInternalServer` → **500** `internal_error` "Error interno" (`L913-916`).
   - Resto → `utils.StatusForCode(code)` con el mensaje del servicio (`L917-918`).
3. `*drive.DriveError` crudo: 403/404 → **404** `note_unavailable`; 413 → **413** `file_too_large`; resto → **502** (`L924-936`).
4. Cualquier otro error → **500** `internal_error` (`L940-941`).

`StatusForCode` (`utils/response.go:88-108`): `bad_request`/`invalid_*` → 400; `forbidden` → 403; `not_found`/`note_unavailable` → 404; `conflict`/`already_saved`/`already_liked` → 409; `file_too_large`/`payload_too_large` → 413; `unsupported_media_type` → 415; `rate_limited` → 429; resto → 500.

El HTTP no tiene un campo de error aparte del `code`: el código HTTP y `error.code` son lo único que distingue los casos (p. ej. dos 404 distintos: `not_found` vs `note_unavailable`).

Validación de path: un `:id` (o `:attachmentId`, `:sharedNoteId`, grupo) que no es UUID responde **404** `not_found` antes de llegar al servicio (`validateNoteID` `note_handler.go:144-150`, `validateUUIDParam` `L152-158`).

Límite de body JSON: 1 MiB (`http.MaxBytesReader`, p. ej. `L178`); excederlo da **413** `file_too_large` "Archivo muy grande".

## 4. Almacenamiento y Drive

- Postgres guarda solo metadata (`notes.notes`, `notes.note_attachments`, `notes.saved_notes`, `notes.note_likes`, `notes.shared_notes`, `notes.drive_managed_permissions`, `notes.idempotency_keys`, `notes.drive_reconciliation_queue`, `notes.account_deletion_*`). Esquema base: `docker/postgres/init.sql:205-355`; migraciones `back/notes/db/migrations/001..003`.
- **No hay sqlc en Notes.** `back/notes/internal/repository/sqlc/` solo contiene `.gitkeep`, no existe `sqlc.yaml` y `back/notes/db/queries/notes.sql` no genera código. Los repositorios son pgx escrito a mano (`back/notes/internal/repository/*.go`), envueltos por `service/store_pg.go`.
- El Markdown vive en el Drive del autor, en la carpeta `Apuntes TI3` (`back/notes/internal/drive/real_client.go:62,71-100`), con `appProperties` `notes_note_id`/`notes_owner_user_id` para recuperar huérfanos.
- `STORAGE_MODE=drive` en k8s (`k8s/manifests.yaml:73`) y por defecto en Compose (`docker-compose.yml:71`); el default del código es `mock` (`config.go:124-126`). Con `drive`, el access token se pide a Identity por gRPC (`identity.v1.IdentityService/GetOAuthToken`, `back/notes/internal/drive/oauth_grpc.go:62`; cliente creado en `main.go:89-103`).
- Cada llamada crítica a Drive usa timeout de 15 s (`driveCallTimeout`, `back/notes/internal/service/note_service.go:273`) y hasta 3 intentos (`driveRetryMaxAttempts`, `L296`).
- Trabajo en segundo plano: un ticker (por defecto cada 30 s, `NOTES_RECONCILE_INTERVAL`) llama `ReconcilePendingNotes` (`main.go:245-264`; `note_service.go:2804`), que (a) drena la outbox `notes.drive_reconciliation_queue` (`reconcileDriveOperations` `L2913`), (b) converge permisos `pending`/`failed` (`reconcileManagedPermissions` `L2340`) y (c) repara notas `pending_drive`/`failed_sync` con más de 15 min (`reconcilePendingMinAge` `L2792`).

## 5. Ciclo de vida de los permisos de Drive

Siempre se usa el token del **autor de la nota** (`ownerID := note.UserID`, `note_service.go:2139`). El estado deseado se guarda primero en `notes.drive_managed_permissions` (`pending`) y luego se aplica en Drive (`in_sync` o `failed`) (`syncDrivePermissions` `L2128-2334`).

| Evento | Qué pasa en Drive | Principal / correo | Rol | Código |
| --- | --- | --- | --- | --- |
| `POST /notes` (o `CreateNote`) con `visibility=public` | Permiso `anyone` con `allowFileDiscovery=false` | Cualquiera con el enlace | `reader` | `note_service.go:891-914`; `real_client.go:439-455` |
| `POST /notes` con `visibility=private` | Ninguno | — | — | `note_service.go:891` |
| `PATCH /notes/:id` que cambia `visibility` o crea el archivo (self-healing) | Recalcula visibilidad + shares y otorga/revoca `anyone` y correos | Igual que share | `reader` | `L1263-1337` |
| `POST /notes/:id/share` `link` | Otorga `anyone` | Cualquiera con el enlace | `reader` | `L1975-2027`, `L2301-2324` |
| `POST /notes/:id/share` `restricted` | Otorga un permiso `type=user` por correo, `sendNotificationEmail=false` | Correo de la **cuenta Google Drive conectada** de cada miembro del grupo (`identity.oauth_connections.external_account_email`, `provider='google_drive'`, `revoked_at IS NULL`). Miembros sin Drive conectado se omiten (no se usa su correo de registro). No se excluye al autor. Correos normalizados a minúsculas, validados y sin duplicados | `reader` | `L2071-2086`; `service/drive_email_directory.go:50-78,110-117`; `service/member_directory.go:31-46`; `real_client.go:416-437` |
| `DELETE /notes/shared/:id` (o `UnshareNote`) | Recalcula con los shares restantes y revoca lo que sobra (por ID para `anyone`, por correo para usuarios; 404 = ya revocado) | Los que ya no tienen vía de acceso | — | `L2405-2459`, `L2237-2270` |
| `POST /notes/unshare-all` (HTTP) | `Unshare` de cada share propio del usuario en el grupo + reconvergencia de todas las notas que siguen compartidas con el grupo (revoca al saliente en notas de otros autores) | Correo Drive del saliente | — | `L2461-2500`, `L2504-2552` |
| `UnshareAll` (gRPC) | Igual idea, con trabajo durable por nota (`notes.account_deletion_group_notes`); con `group_deleted=true` borra todos los shares del grupo | Correo Drive del saliente | — | `service/account_deletion.go:79-146,182-261` |
| `DELETE /notes/:id` | No revoca permisos uno a uno: borra las filas de `drive_managed_permissions` en la misma transacción y borra el archivo (los permisos mueren con él) | — | — | `repository/note_repository.go:566-569`; `note_service.go:1401-1429` |
| `AccountDeletionCleanup` (gRPC, desde Auth) | Retira todos los shares de notas del usuario y reconverge. **No** borra notas ni archivos | — | — | `account_deletion.go:40-73` |
| Reconciliador (cada 30 s) | Reintenta notas con permisos `pending`/`failed` (hasta 100 por pasada) | — | — | `note_service.go:2340-2381` |

Si Drive falla al otorgar o revocar, la operación del usuario **no** se revierte: el share queda con `permission_sync_status=failed` y el reconciliador lo reintenta.

No existe un RPC ni endpoint dedicado a "permisos Drive por email": se aplican dentro de las operaciones de la tabla.

## 6. Dependencias de Notes hacia otros servicios

Ver [GRPC-notes-clientes-salientes.md](Grpc/GRPC-notes-clientes-salientes.md).
