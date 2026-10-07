# Documentación — Héctor Lepio / Notes

Microservicio Notes: núcleo de apuntes (CRUD, adjuntos, guardar, copiar, likes,
compartir con grupos) con contenido almacenado en Google Drive del autor y
metadata en PostgreSQL (`notes.*`). Patrón Handler → Service → Repository en
`back/notes/`.

## 0. Alcance y estado actual

Notes persiste metadata en PostgreSQL y contenido Markdown en el Google Drive
del autor (`notes.notes.external_file_id`). Sin `DATABASE_URL` levanta con
stores en memoria; con `STORAGE_MODE=drive` usa la API real de Drive con tokens
por usuario desde `identity.oauth_connections`
(`back/notes/cmd/server/main.go:31-68`).

Rutas registradas (todas protegidas con JWT, `main.go:130-152`):

- `POST /notes`, `GET /notes/me`, `GET /notes/:id`, `PATCH /notes/:id`,
  `DELETE /notes/:id`
- `POST /notes/:id/attachments`, `DELETE /notes/:id/attachments/:attachmentId`,
  `GET /notes/:id/attachments/:attachmentId/content`, `POST /notes/upload`
- `POST /notes/:id/save`, `POST /notes/:id/copy`, `POST /notes/:id/like`,
  `DELETE /notes/:id/like`
- `POST /notes/:id/share`, `DELETE /notes/shared/:sharedNoteId`,
  `POST /notes/unshare-all`, `GET /groups/:id/notes`
- `GET /notes/:id/access`, `POST /notes/reconcile`, `GET /health`,
  `GET /health/drive`

No existe gRPC en Notes: `back/proto/notes/` contiene solo `.gitkeep`. La
comunicación Notes → Social es HTTP (`SocialHTTPAdapter`,
`back/notes/internal/service/social_adapter.go:30-57`). `Unsave` existe en el
servicio (`note_service.go:1615-1617`) pero no tiene ruta registrada.

Mapa canónico por operación: ROUTE (`cmd/server/main.go`) → HANDLER
(`internal/handler/http/note_handler.go`, `attachment_content.go`) → SERVICE
(`internal/service/note_service.go`, `reconcile_drive.go`,
`attachment_content.go`) → REPOSITORY (`internal/repository/`,
`internal/service/store_pg.go`) → DB (`docker/postgres/init.sql:155-298`,
migraciones `back/notes/db/migrations/`) → DRIVE (`internal/drive/`) →
DEPENDENCIA (Social HTTP, Identity OAuth).

## 1. Arquitectura de Notes

### 1.1 Wiring y configuración

`runServer` construye stores PG, adaptadores Social HTTP e inyecta el
directorio de miembros (`main.go:70-82`). Sin `DATABASE_URL` usa stores en
memoria (`main.go:84-96`). `newSocialAdapters` devuelve siempre el adaptador
HTTP real contra `SOCIAL_SERVICE_URL`; los resolvers en memoria solo existen en
tests (`main.go:102-106`, `internal/service/store_memory.go`).

Config (`internal/config/config.go:14-36`): `DATABASE_URL`, `JWT_SECRET`
(fallback `dev-jwt-secret-change-me`, `:96-102`), `PORT`/`NOTES_PORT` (default
`8082`, `:90-95`), `STORAGE_MODE` (`mock` por defecto, `:103-105`),
`GOOGLE_CLIENT_ID/SECRET` (`:66-67`), `SOCIAL_SERVICE_URL` (default
`http://social:8083`, `:73-75`), `SOCIAL_TIMEOUT` (default `5s`, `:76-82`),
`INTERNAL_API_KEY` (`:71`), `NOTES_RECONCILE_INTERVAL` (default `30s`,
`:83-89`).

El reconciliador corre en background con ticker (`main.go:163-177`) e invoca
`svc.ReconcilePendingNotes`.

### 1.2 Autenticación

`SimpleHS256Validator` valida HS256 con `iss=apuntes-auth`,
`aud=apuntes-client` (`internal/middleware/auth_middleware.go:38-72`).
`RequireAuth` exige `Authorization: Bearer {token}`, distingue expirado
(`token_expired`) de inválido (`invalid_token`) e inyecta `user_id` (+ `role`)
en `gin.Context` y `request.Context` (`auth_middleware.go:82-119`).
`GetUserID` lee ambas fuentes (`auth_middleware.go:121-133`). Sin token se
responde 401 en el middleware, antes de llegar al handler.

Errores de dominio: `ServiceError{Code, Message}`
(`note_service.go:26-31`); centinelas `ErrInternalDatabase`,
`ErrDriveUnavailable` (→ 502), `ErrSourceAccessDenied`, `ErrInternalServer`
(`note_service.go:36-49`). El handler traduce exclusivamente con
`errors.Is`/`errors.As`, sin inspeccionar mensajes ni filtrar trazas SQL
(`note_handler.go:884-936`). Códigos y mapeo a status en
`internal/utils/response.go:28-109` (`StatusForCode`: `already_saved`/
`already_liked`/`conflict` → 409, `note_unavailable`/`not_found` → 404,
`file_too_large` → 413, `unsupported_media_type` → 415, `rate_limited` → 429).

Validadores (`internal/utils/validator.go:9-32`): título 1-255 tras
`TrimSpace`, visibilidad `public|private`, `access_mode` `link|restricted`,
UUID con `uuid.Parse`.

## 2. `POST /notes`

Contrato: `agentApiContract2.md §3` → `201 {note_id}`, `400` visibilidad
inválida; crea metadata y dispara creación async del archivo en Drive.

Ruta: `main.go:133` → `NoteHandler.Create` (`note_handler.go:169-196`) →
`NoteService.Create` (`note_service.go:652-877`).

### 2.1 Request y validaciones

- Forma: `createRequest{title, subject_id, visibility, content}`
  (`note_handler.go:134-139`). `title` requerido `max=300`,
  `subject_id` opcional con formato UUID, `visibility` requerida
  `oneof=public private`, `content` opcional (compatibilidad con spec TAREA).
- Body JSON acotado a 1 MB vía `http.MaxBytesReader`; exceso → 413
  (`note_handler.go:177-183`).
- Servicio: título 1-255 tras `TrimSpace` (`note_service.go:653-660`,
  `validateTitle` en `:596-601`); `subject_id` con `TrimSpace`, vacío → `nil`,
  no-UUID → `invalid_subject_id` (`note_service.go:685-692`,
  `validateSubjectID` en `:608-620`); contenido > 1 MB (`maxNoteContentLength`,
  `:280`) → `file_too_large` (`note_service.go:662-664`); contenido se
  sanitiza contra XSS (`sanitizeMarkdown`, `:665-667`, regex `:59-76`,
  reemplazo `:86-104`).
- Normalización: título con `TrimSpace` y `/` `\` → `_`
  (`note_service.go:670-672`); visibilidad vacía → `private` (`:676-678`).
  Contenido por defecto `# {title}\n\n` cuando no se envía (`:694-699`).

**Divergencia con Contract 2:** el contrato no incluye `content` en el body;
el código lo acepta y lo persiste en Drive. El handler admite `max=300` en
título pero el servicio rechaza 256-300 con `invalid_title` (límite efectivo
255). La visibilidad por defecto `private` del servicio es inalcanzable por
HTTP (el binding la exige), solo aplica a llamadas internas.

### 2.2 Autenticación y autorización

`user_id` desde JWT (`note_handler.go:171-175`). El autor es el creador; no se
requiere membresía.

### 2.3 Flujo de negocio

1. Reclama idempotencia durable si hay `X-Idempotency-Key` (clave
   preasignada `noteID`, hash del payload; replay devuelve la nota,
   hash distinto → 409) (`note_service.go:705-741`,
   `beginIdempotentOperation` en `:172-227`).
2. Inserta metadata en PG con `sync_status='pending_drive'` y `noteID`
   preasignado (`observedNotesCreate`, `:754`). Si el intento previo murió
   tras insertar, reutiliza la fila (`:747-766`).
3. Crea el archivo `.md` en Drive del autor con timeout 15 s
   (`withDriveTimeout`, `:287-289`) y 3 intentos con backoff
   (`retryDriveOperation`, `:308-334`; solo reintenta 5xx/red, `:339-361`),
   indexado con `appProperties.notes_note_id` (`:788-810`,
   `drive/client.go:160-166`). En retry de crash busca primero el huérfano
   (`findOrphanDriveFile`, `:367-379`).
4. Si visibilidad `public` y archivo recién creado, otorga permiso link de
   lectura y lo persiste como managed permission `in_sync`
   (`note_service.go:815-838`).
5. Actualiza `external_file_id` + `sync_status='synced'`
   (`UpdateExternalFileID`, `repository/note_repository.go:208-220`); si PG
   falla tras Drive OK, compensa borrando el huérfano o encolando
   `delete_file` en la outbox (`note_service.go:841-863`).
6. Publica la clave de idempotencia (`:870-875`). Responde `201 {note_id}`
   (`note_handler.go:195`).

Fallo de Drive → fila `failed_sync` reconciliable (`:796-807`); OAuth
ausente/revocado → 403 (`:798-801`); 413 de Drive → `file_too_large`
(`:803-806`); resto → 502 `ErrDriveUnavailable`.

### 2.4 Persistencia

`notes.notes (user_id, subject_id, title, external_file_id, visibility,
forked_from_note_id=NULL, sync_status)` vía `NoteRepository.Create`
(`note_repository.go:39-70`). `likes_count` default 0, `version` default 1
(`init.sql:162-177`).

### 2.5 Google Drive

`CreateFile(ctx, userID, noteID, title+".md", mdContent)`; mock genera
`drive_<uuid>` (`drive/client.go:251-263`). Real usa
`identity.oauth_connections` vía `PGOAuthTokenStore`
(`drive/oauth.go:37-80`).

### 2.6 Respuesta y errores

`201 {note_id}`; `400 invalid_title|invalid_subject_id|bad_request`;
`403` OAuth (`Conecte o renueve su Google Drive`); `409` conflicto de
idempotencia; `413 file_too_large`; `502` Drive no disponible; `500` fallo PG.

### 2.7 Tests

`note_service_test.go`: `TestCreateSuccess`, `TestCreateValidations`,
`TestCreateRollbackOnDBFailure`; `drive_outbox_test.go`,
`idempotency_recoverable_test.go`; handler `TestHandlerCreateSuccess`,
`TestHandlerCreateValidation400`, `TestHandlerCreateUnauthorized401`
(`internal/handler/http/note_handler_test.go`). Correr desde `back/notes`:
`go test ./internal/service/ ./internal/handler/http/ -count=1`.

### 2.8 Estado

Implementado. **Divergencia con Contract 2:** acepta `content` opcional no
contratado; la creación en Drive es sincrónica dentro del request (con
reintentos), no fire-and-forget.

## 3. `PATCH /notes/{id}`

Contrato: `agentApiContract2.md §3` → `{title?, visibility?}` (`200`),
`403` no eres el autor. El contenido "se edita directo en Drive".

Ruta: `main.go:147` → `NoteHandler.Patch` (`note_handler.go:308-354`) →
`UpdateWithExpectedVersion` (`note_service.go:976-1006`) →
`updateInner` (`:1014-1028`) → `updateInnerLocked` (`:1030-1263`).

### 3.1 Request y validaciones

- `patchRequest{title?, visibility?, content?, version?}`
  (`note_handler.go:299-306`): título `min=1,max=300`, visibilidad
  `oneof=public private`, `content` para sync Drive, `version` precondición de
  versionado optimista (409 si no coincide).
- Body acotado 1 MB (`:320`); sin ningún campo → 400 `nada que actualizar`
  (`:329-332`); `id` no-UUID → 404 (`validateNoteID`, `:141-150`).
- Idempotencia opcional con `X-Idempotency-Key` (`:333-340`,
  `note_service.go:980-1005`).

**Divergencia con Contract 2:** el contrato excluye el contenido de este
endpoint; el código SÍ sincroniza `content` a Drive (con snapshot previo
reversible) y acepta `version`. El contrato fija `403` para no-autor; el
código responde 404 zero-knowledge (`note_service.go:1038-1040`).

### 3.2 Autenticación y autorización

Solo autor (`note.UserID != userID` → 404 idéntico al inexistente,
`note_service.go:1038-1040`). Ocultamiento de existencia deliberado.

### 3.3 Flujo de negocio

Todo bajo lock distribuido por nota (`WithNoteLock`,
`repository/shared_repository.go:43-70`):

1. Lee nota, verifica autor y precondición `expectedVersion`
   (`note_service.go:1031-1045`).
2. Sanitiza/normaliza igual que Create (`:1047-1066`).
3. Drive primero, PG después: con archivo existente y `content`, toma
   snapshot previo (obligatorio; si falla aborta 502/403/404,
   `:1084-1101`), aplica `UpdateFile`, y si PG falla revierte con
   `compensateDriveUpdate` (`:1103-1124`, `:1270-1279`). Sin
   `external_file_id` (legacy/offline) hace self-healing creando el `.md`
   (`:1125-1163`). Solo-renombre cuando solo cambia el título
   (`:1164-1186`).
4. Si cambia visibilidad, calcula permisos deseados y converge tras el
   UPDATE de metadata (fallo diferido, no revierte) (`:1187-1261`).
5. UPDATE de metadata con versionado optimista: `expectedVersion` del
   cliente o versión leída; `UPDATE ... WHERE id AND version` +
   `version+1`; conflicto → 409 con revert de Drive (`:1219-1236`,
   `repository/note_repository.go:136-181`).

### 3.4 Persistencia

`NoteRepository.Update` dinámico (`title`, `visibility`, siempre
`version=version+1, updated_at=now()`) con `ErrConflict` si la fila existe
pero cambió la versión (`note_repository.go:136-181`, migración
`db/migrations/002_note_version_optimistic_lock.sql:9-10`).

### 3.5 Respuesta y errores

`200 {id, title, visibility, version, updated_at}`
(`note_handler.go:347-353`); `400` validación; `404` inexistente/no-autor/
Drive 403-404 (`note_unavailable`); `403` OAuth; `409` versión o idempotencia;
`413` contenido; `502` Drive.

### 3.6 Tests

`TestUpdateOnlyAuthor`, `TestDriveUpdateSyncsContent`,
`TestDriveUpdateRecreatesMissingFile`,
`TestDriveUpdateNotFoundMapsNoteUnavailable`
(`internal/service/note_service_test.go`); handler `TestHandlerPatchSyncDrive`,
`TestHandlerPatchVersionConflict`, `note_postgres_edit_test.go`,
`note_version_lock_test.go`, `note_repository_version_test.go`.

### 3.7 Estado

Implementado (extendido respecto al contrato: contenido + versionado
optimista).

## 4. `DELETE /notes/{id}`

Contrato: `204`, borra metadata y archivo en Drive; `403` no eres el autor.

Ruta: `main.go:148` → `NoteHandler.Delete` (`note_handler.go:357-373`) →
`NoteService.Delete` (`note_service.go:1291-1355`).

### 4.1 Flujo de negocio

Orden canónico PG-primero (documentado `:1281-1290`):

1. Verifica autor; no-autor/inexistente → 404 (`:1295-1304`).
2. `DeleteWithDriveCleanup` en una transacción: `SELECT ... FOR UPDATE`,
   captura `external_file_id` + adjuntos, encola `delete_file` y
   `delete_attachment` en `drive_reconciliation_queue`, borra dependientes
   (`note_attachments`, `saved_notes`, `note_likes`, `shared_notes`),
   limpia `drive_managed_permissions` y borra la nota
   (`repository/note_repository.go:489-573`).
3. Limpia managed permissions en memoria y ejecuta borrados Drive
   best-effort con reintento; 404 idempotente; si Drive falla tras PG OK,
   retorna éxito igual (la outbox conserva el trabajo)
   (`note_service.go:1325-1353`, `executeDriveOperation` en `:2763-2799`).

Segundo `DELETE` → 404 (idempotencia a nivel API).

### 4.2 Tests

`TestDeleteOnlyAuthor`, `TestDriveDeleteCascadesDriveAndPostgres`;
`delete_cleanup_test.go`; `drive_outbox_integration_test.go`
(`TestDriveCleanupTransactions`,
`TestDeleteNoteQueuesAllAttachmentsBeforeCascade`);
`simple_protocol_delete_test.go`; handler
`TestHandlerDeleteNoteAndAllFiles`. Integración PG requiere DB
(`outboxTestPool`, `drive_outbox_integration_test.go:18`).

### 4.3 Estado

Implementado. **Divergencia con Contract 2:** no-autor responde 404, no 403.

## 5. `GET /notes/{id}`

Contrato: `200 {title, content, likes_count, ...}`, `404`/`410` no disponible.

Ruta: `main.go:149` → `NoteHandler.Get` (`note_handler.go:199-251`) →
`NoteService.Get` (`note_service.go:880-934`).

### 5.1 Flujo de negocio

1. Valida UUID, lee metadata (`:881-890`).
2. `hasReadAccess`: autor o `public` pasan; privada exige share `link`
   vigente o membresía en algún grupo con share `restricted`; sin acceso →
   404 idéntico al inexistente (`:568-593`, `:894-900`).
3. Sin `external_file_id` → `note_unavailable`
   (`:903-905`). Descarga contenido con timeout+reintento
   (`:906-913`); OAuth/403/404 de Drive → `note_unavailable`
   (`:914-929`); resto → 502 (`:930`).

Respuesta real (`note_handler.go:216-250`): `{id, user_id, title,
visibility, likes_count, version, created_at, updated_at, subject_id?,
external_file_id?, drive_url?, forked_from_note_id?, content?, attachments[]}`
(`attachments` vía `ListAttachments`, `[]` cuando vacío, `:240-249`).

### 5.2 Tests

`TestGetWithContent`, `TestGetDriveNotFound`, `TestGetDriveForbidden`,
`TestDriveGetMaps403And404ToNoteUnavailable`;
`TestHandlerGetNotFound404`, `TestHandlerGetWithDriveResilienceAlternative`,
`TestHandlerPrivateAccessNotFound`.

### 5.3 Estado

Implementado. **Divergencias con Contract 2:** incluye `attachments`,
`version`, `drive_url`; no existe 410 (todo es 404 `note_unavailable`).

## 6. `GET /notes/me`

Contrato: query `cursor`, `limit` → `200 {items: [...], next_cursor}`.

Ruta: `main.go:134` → `NoteHandler.ListMy` (`note_handler.go:254-296`) →
`NoteService.ListMy` (`note_service.go:937-956`) →
`NoteRepository.ListByUser` (`note_repository.go:88-129`).

Límite default 20, tope 100 (`note_service.go:938-943`); cursor debe ser UUID
o → 400 (`:945-947`); paginación por `(created_at, id)` con `limit+1`
(`note_repository.go:97-129`). Respuesta real: `200 {notes: [...],
next_cursor}` (`note_handler.go:295`), cada ítem con `id, user_id, title,
visibility, likes_count, version, created_at, updated_at, subject_id?,
external_file_id?` (`:277-294`).

**Divergencia con Contract 2:** la clave es `notes`, no `items`.

Tests: `TestListMyPagination` (service y handler:
`TestHandlerListMyPagination`). Estado: Implementado.

## 7. Adjuntos

### 7.1 `POST /notes/{id}/attachments`

Contrato: `{file}` o `{external_file_id}` → `201 {attachment_id,
is_inline}`, `413` archivo muy grande.

Ruta: `main.go:139` → `NoteHandler.UploadAttachment`
(`note_handler.go:415-528`) → `AddAttachment` (`note_service.go:1358-1419`)
o `AddAttachmentExternal` (`:1475-1520`).

- JSON (`Content-Type: application/json`): exige `external_file_id`
  (`note_handler.go:429-450`); valida whitelist por extensión/tipo
  (`:467-470`, `drive/mime.go:96-100`); registra sin binario tras verificar
  acceso al archivo en Drive (`VerifyFileAccess`, `note_service.go:1494-1507`).
- Multipart `file`: body total 11 MB, archivo 10 MB (`note_handler.go:482`,
  `:501-504`); vacío → 400 (`:505-508`); `is_inline` por query o form
  (`:510`); sniffing estricto de los primeros 512 bytes:
  contenido fuera de whitelist → 415, MIME declarado contradictorio → 400
  (`:517-521`, `drive/mime.go:125-135`).
- Servicio: solo autor (no-autor → 403, `:1369-1371`); `fileName` vacío →
  `attachment`; MIME preservado si es específico, si no resuelto por
  extensión/sniffing (`DetectMimeType`, `drive/mime.go:142-158`); subida a
  Drive con timeout+reintento (`note_service.go:1385-1402`); si PG falla tras
  subir, compensa borrando el huérfano o encolando `delete_attachment`
  (`:1404-1417`).
- Whitelist efectiva: `image/jpeg`, `image/png`, `application/pdf`
  (`drive/mime.go:82-86`). El repositorio guarda `note_id,
  external_file_id, file_url, file_type, file_name, file_size_bytes,
  is_inline` (`repository/attachment_repository.go:21`).

Respuesta real `201 {attachment_id, is_inline, file_url, external_file_id}`
(`note_handler.go:476`, `:527`). Errores: `400` (vacío, MIME mismatch,
JSON inválido), `403` no-autor/OAuth, `404` nota inexistente o archivo
externo inaccesible, `413`, `415`, `502`.

No existe motor matemático ni parsing de code/math: el Markdown se conserva
tal cual (salvo sanitización XSS); `is_inline` solo marca si el adjunto está
embebido en el Markdown.

### 7.2 `DELETE /notes/{id}/attachments/{attachmentId}`

Ruta: `main.go:141` → `NoteHandler.DeleteAttachment`
(`note_handler.go:594-617`) → `RemoveAttachment`
(`note_service.go:1421-1468`).

Solo autor (403 si no); adjunto de otra nota o inexistente → 404
(`:1425-1441`). Orden PG-primero: `DeleteAttachmentWithDriveCleanup`
(transacción que verifica autor y captura `fileID`,
`repository/attachment_repository.go:91`) y luego borrado Drive best-effort
sin revertir el éxito (`note_service.go:1459-1466`). Responde 204.

### 7.3 Rutas complementarias reales (fuera de Contract 2)

- `POST /notes/upload` (`main.go:137` → `UploadFile`,
  `note_handler.go:535-591` → `UploadToDrive`,
  `note_service.go:1536-1572`): sube binario al Drive del usuario sin nota
  destino; devuelve `201 {external_file_id, file_url, file_name, file_type,
  file_size_bytes, is_inline:false}`. Mismas validaciones 10 MB/MIME.
- `GET /notes/:id/attachments/:attachmentId/content` (`main.go:140` →
  `AttachmentContent`,
  `handler/http/attachment_content.go:11-35` → `GetAttachmentContent`,
  `service/attachment_content.go:12-60`): exige la misma política de lectura
  que `Get` (zero-knowledge), descarga con el OAuth del dueño
  (`drive/attachment_content.go:12-59`, tope `MaxAttachmentBytes` 10 MB,
  `:9`); `Content-Disposition: inline` para imagen/pdf y `nosniff`
  (`attachment_content.go:23-34`); sin acceso → 404; adjunto ilegible → 404
  `note_unavailable`.

### 7.4 Tests

`TestAttachmentUploadAndDelete`, `TestAttachmentTooLarge`;
`attachment_content_test.go` (service, drive y handler);
`mime_test.go`; `note_handler_upload_test.go`
(`TestHandlerExternalAttachment`, `TestHandlerAttachment`).

### 7.5 Estado

Implementado.

## 8. `POST /notes/{id}/save`

Contrato: `201`, `409` ya guardado.

Ruta: `main.go:142` → `NoteHandler.Save` (`note_handler.go:620-637`) →
`NoteService.Save` (`note_service.go:1575-1613`).

Referencia, no copia: exige lectura vigente (`hasReadAccess`; privada sin
acceso → 404, `:1588-1594`); pre-consulta `Exists` y, ante carrera en el
insert, revalida para distinguir duplicado de error interno
(`:1598-1611`). `UNIQUE (user_id, note_id)` en `notes.saved_notes`
(`init.sql:202-208`); duplicado → `already_saved` → 409
(`utils/response.go:99`). Si el original se borra, la fila desaparece por
`CASCADE` sin lógica especial. No hay revalidación posterior de acceso: el
bookmark persiste aunque el acceso se pierda. Handler responde `201` sin body
(`note_handler.go:636`).

`Unsave` existe en el servicio (`note_service.go:1615-1617`) pero **no tiene
ruta registrada** (no hay `DELETE /notes/{id}/save` en `main.go:130-152`).

Tests: `TestSaveAndAlreadySaved`, `TestHandlerSaveConflict`. Estado:
Implementado (lectura de guardados por HTTP no expuesta; `Unsave` sin ruta).

## 9. `POST /notes/{id}/copy`

Contrato: `201 {note_id}` con `forked_from_note_id`, `404` original no
disponible.

Ruta: `main.go:143` → `NoteHandler.Copy` (`note_handler.go:640-671`) →
`NoteService.Copy` (`note_service.go:1631-1822`).

Rate limit en memoria por handler: 10/min por usuario y 100/min global
(`note_handler.go:20-34`, `allow` en `:57-122`); exceso → 429 `rate_limited`
(`:655-663`). Idempotencia durable igual que Create (`:1666-1694`).

Flujo: original inexistente/inaccesible/sin archivo → 404 zero-knowledge
(`:1643-1655`); inserta clon en PG (`user_id` del copiador, `subject_id=nil`
siempre, `visibility="private"` siempre, `forked_from=orig.ID`,
`sync_status='pending_drive'`, `:1712`); copia el archivo en Drive del
copiador con su OAuth (`CopyFile(srcUserID, srcFileID, dstUserID, newNoteID,
title)`, `:1745-1772`, `drive/client.go:392-418` — solo exige OAuth del
destino); defensa `fileID` distinto (`:1773-1786`); persiste
`external_file_id`/`synced` con compensación idéntica a Create
(`:1788-1809`); publica idempotencia (`:1816-1820`). Responde
`201 {note_id, forked_from_note_id}` (`note_handler.go:670`).

Fallo OAuth del copiador → 403; origen ilegible (404/403 Drive) → 404;
transitorio → 502 (`:1752-1769`). El clon es independiente: no hereda
`subject_id` (se resetea con log, `:1701-1703`) ni visibilidad (siempre
privado), y su `external_file_id` es un archivo distinto.

**Divergencia con Contract 2:** respuesta añade `forked_from_note_id`;
visibilidad del clon siempre `private` aunque el original sea `public`.

Tests: `TestCopyClonesFile`, `TestCopyPrivateWithoutAccessNotFound`,
`TestDriveCopyCreatesIndependentFile`, `TestDriveCopyMissingSourceMapsNotFound`,
`TestHandlerLikeAndCopy`, `note_handler_ratelimit_test.go`. Estado:
Implementado.

## 10. Likes

Contrato: `POST → 201` (`409` ya likeado); `DELETE → 204`.

Rutas: `main.go:144-145` → `NoteHandler.Like` (`note_handler.go:674-690`) /
`Unlike` (`:693-709`) → `NoteService.Like` (`:1827-1862`) / `Unlike`
(`:1864-1873`).

- `Like`: UUID válido, nota existente, `hasReadAccess` previo (privada sin
  acceso → 404, `:1839-1845`); `Exists` previo → `already_liked`
  (`:1846-1852`); `LikeAtomic` transaccional (INSERT + `likes_count+1` en una
  Tx; `23505` → `already_liked`,
  `repository/like_repository.go:69-95`); ante fallo revalida `Exists` para
  distinguir carrera de error interno (`note_service.go:1853-1860`).
- `Unlike`: `UnlikeAtomic` (DELETE; si afectó fila, `likes_count =
  GREATEST(likes_count-1,0)` en la misma Tx; idempotente si no había like,
  `like_repository.go:99-120`); responde 204 aunque no existiera el like.

Un like por usuario impuesto por `UNIQUE (note_id, user_id)`
(`init.sql:214-220`, migración `001:56-77`). `likes_count` desnormalizado en
`notes.notes` (`init.sql:169`).

Tests: `TestLikeUnlikeTransactional`, `TestConcurrentLikes` (service y
handler), `like_read_contract_test.go`. Estado: Implementado.

## 11. Compartir con grupo

### 11.1 `POST /notes/{id}/share`

Contrato: `{group_id, access_mode}` → `201`; resuelve `is_admin_note` vía
gRPC a Social; `403` no eres miembro.

Ruta: `main.go:146` → `NoteHandler.Share` (`note_handler.go:717-744`) →
`Share` (`note_service.go:1883-1897`) → `shareLocked` (`:1899-1951`).

- `shareRequest{group_id: uuid requerido, access_mode: link|restricted}`
  (`note_handler.go:712-715`); body 1 MB; inválido → 400.
- `shareLocked` bajo `WithNoteLock`: valida `access_mode` (`:1900-1902`),
  solo autor (403 si no, `:1910-1912`), `IsMember` del autor (fail-closed:
  error de Social → 500, no-miembro → 403, `:1914-1920`), resuelve
  `isAdmin` y `followersCount` (best-effort, `_`, `:1921-1922`).
- Calcula permisos deseados ANTES de persistir (si el directorio falla, no
  crea el share, `:1932-1937`): `ComputeDesiredDrivePermissions`
  (`:1961-1983`) — `Anyone = visibility==public || algún share link`;
  `Emails` = unión normalizada (`NormalizeEmails`,
  `member_directory.go:28-43`) de `ListMemberEmails` por cada grupo
  restricted.
- BD primero: inserta `shared_notes` con `permission_sync_status='pending'`
  (`repository/shared_repository.go:72-88`); converge ACL de inmediato vía
  `syncDrivePermissions` (`note_service.go:2025-2220`); si Drive falla, el
  share persiste y queda `failed` para el reconciliador (`:1945-1949`).
- Responde `201 {shared_note_id}` (`note_handler.go:743`).

**Divergencia con Contract 2:** la resolución es HTTP
(`SocialHTTPAdapter.IsMember/IsAdmin/GetFollowersCount`,
`social_adapter.go:74-115` con timeout y `X-Internal-Key`, `:175-209`), no
gRPC. No hay `.proto` de Notes (`back/proto/notes/.gitkeep`).

### 11.2 `DELETE /notes/shared/{sharedNoteId}`

Contrato: `204`; `403` no eres autor ni admin del grupo.

Ruta: `main.go:150` → `NoteHandler.Unshare` (`note_handler.go:747-766`) →
`Unshare` (`:2275-2289`) → `unshareLocked` (`:2291-2345`), bajo lock por nota.

Autorización: autor de la nota o admin del grupo (`IsAdmin`; error de Social
→ 500, `:2311-2319`). BD primero (borra el share), marca managed permissions
como `pending` y converge el estado restante best-effort
(`:2320-2344`). Si la nota ya no existe, borra el share huérfano (`:2303-2308`).
Responde 204.

### 11.3 `POST /notes/unshare-all`

Contrato: interno, gRPC desde Social, `{user_id, group_id}` → `204`.

Ruta real: `main.go:135` → `NoteHandler.UnshareAll`
(`note_handler.go:774-805`) → `NoteService.UnshareAll`
(`note_service.go:2347-2377`). Es HTTP con JWT de usuario, no gRPC: el
llamante debe ser el propio `user_id` o admin del grupo (`IsGroupAdmin`,
`note_service.go:2586-2589`), si no → 403 fix IDOR documentado en
(`note_handler.go:776-796`). Pagina las notas del `user_id` (100 por página)
y retira cada share del `group_id` vía `Unshare` (`:2351-2376`).

**Divergencia con Contract 2:** expuesto como HTTP autenticado, no como RPC
interno sin superficie pública.

### 11.4 `GET /groups/{id}/notes`

Contrato: query `cursor`, `limit` → `200 {items, next_cursor}` ordenado por
admin, likes, seguidores, fecha; `403` no-miembro. La lógica vive en Notes
(nota de propiedad de servicio, Contract 2 §3).

Ruta: `main.go:151` → `NoteHandler.ListGroupNotes`
(`note_handler.go:842-877`) → `NoteService.ListGroupNotes`
(`:2538-2582`).

Exige membresía del solicitante (`IsMember`; no-miembro → 403,
`:2543-2549`); lista `shared_notes` del grupo (índice
`(group_id, is_admin_note DESC)`, `init.sql:248-249`; `ListByGroup`,
`shared_repository.go:127-167`); resuelve cada nota, ordena en memoria
admin → `likes_count` DESC → `shared_at` DESC (`note_service.go:2567-2576`);
responde `200 {notes, next_cursor}` (`note_handler.go:876`).

**Divergencias con Contract 2:** clave `notes` (no `items`);
`author_followers_snapshot` se almacena al compartir pero NO participa del
orden (solo admin, likes, fecha).

### 11.5 Tests

`TestShareAndAccessControl`, `TestShareForbiddenIfNotAuthor`,
`TestShareForbiddenIfNotMember`, `TestShareInvalidAccessMode`,
`TestUnshareAll`, `TestListGroupNotes`, `TestListGroupNotesOrdering`,
`TestAccessModeLinkVsRestricted`, `TestShareRestrictedGrants`,
`TestShareRestrictedDedup`, `TestShareRestrictedSkipsBlankEmails`,
`TestShareRestrictedMemberError`,
`TestShareRestrictedPartialFailureConvergesLater`,
`TestShareRestrictedEmptyGroup`, `TestShareRestrictedAuthorOnce`,
`TestShareLinkNoGrants`; handler `TestHandlerShareAndGroupNotes`,
`TestHandlerUnshareAll`, `TestHandlerAccessModeLinkVsRestricted`,
`TestListGroupNotesOrdering`, `TestHandlerShareRestrictedGrantsDrive`;
`social_adapter_test.go`, `drive_permissions_test.go`,
`permission_state_test.go`.

### 11.6 Estado

`link`: Implementado. `restricted`: Implementado parcialmente (ver §16).

## 12. `GET /notes/{id}/access`

Contract 2 §2 la asigna a Identity/Agustín, pero está implementada en Notes:
`main.go:138` → `NoteHandler.GetAccess` (`note_handler.go:822-839`) →
`NoteService.GetAccess` (`:2379-2458`).

Autorización de aplicación sin tocar Drive: `owner` (autor), `public`
(visible), `link` (algún share link), `restricted` (miembro de algún grupo
con share); sin acceso → 404 zero-knowledge (`:2390-2433`). Sin
`external_file_id` → `note_unavailable` (`:2436-2438`). Responde
`NoteAccessResponse{can_read, can_write, visibility, access_mode,
drive_sync_status, drive_access_verified, drive_connection_required,
drive_url}` (`internal/model/note.go:6-15`):

- `drive_sync_status` desde `drive_managed_permissions` (failed > pending >
  synced, `GetAccessInfo`, `:2512-2518`,
  `DriveSyncStatusFromManagedPermissions`, `:2522-2536`).
- `drive_access_verified`/`drive_connection_required` solo para
  `owner` sobre nota no-pública (verifica con `VerifyFileAccess`;
  OAuth → conexión requerida; resto → banderas en falso, nunca falla por
  Drive, `driveAccessFlags`, `:2477-2494`; documentado en
  `note_handler.go:807-821`).

Tests: `access_contract_test.go` (service y handler),
`TestGetAccess*`, `TestHandlerGetAccess*`. Estado: Implementado en Notes
(contrato la atribuye a Identity).

## 13. `POST /notes/reconcile`

Fuera de Contract 2. `main.go:136` → `NoteHandler.Reconcile`
(`note_handler.go:379-397`) → `ReconcileDriveDeletions`
(`service/reconcile_drive.go:26-58`).

Lista una vez los archivos activos del usuario (`ListAppFileIDs`,
`drive/client.go:642-662` mock / real con `trashed=false`) y confirma cada
candidato ausente con `FileGone` (404 o papelera → `true`; OAuth/red/403/429/
5xx → error, nunca eliminación, `drive/client.go:667-680`,
`reconcile_drive.go:185-198`). Solo el missing confirmado borra vía flujos
normales `Delete`/`DeleteAttachmentWithDriveCleanup`; al borrar un adjunto
limpia su referencia `attachment:<id>` del Markdown
(`reconcileMissingAttachment`, `:144-180`; `stripAttachmentRefs`, `:213-218`).
Responde `{removed_notes, removed_attachments, pending, removed_note_ids}`
(`note_handler.go:391-396`, `model/note.go:147-152`).

Tests: `reconcile_drive_test.go`, `TestHandlerReconcileDriveDeletions`.
Estado: Implementado (extensión manual Drive→App, distinta del
reconciliador automático del §14).

## 14. Google Drive y almacenamiento

Metadata en PostgreSQL, contenido en Drive (`agentMain.md §2.2`, ver §17):

- Nota: archivo Markdown del autor, `notes.notes.external_file_id`
  (`init.sql:167`); nulo brevemente entre creación local y sync
  (`init.sql:158-161`).
- Adjuntos: binarios del autor, `notes.note_attachments.external_file_id`
  + `file_url` (`init.sql:186-196`); `is_inline` distingue embebido en el
  Markdown de adjunto suelto.
- Cliente real: `drive/v3` con `PGOAuthTokenStore` leyendo
  `identity.oauth_connections(provider='google_drive')`, con refresh vía
  `GOOGLE_CLIENT_ID/SECRET`, mutex por usuario y errores tipados
  (`drive/oauth.go:37-80`, `ProviderGoogleDrive` en `:19`; servicio
  espejo en `back/auth`, `cmd/server/main.go:89-91`,
  `internal/repository/oauth_repository.go:50-201`).

Timeout 15 s por llamada (`note_service.go:274-289`), 3 intentos con backoff
50 ms lineal solo ante 5xx/red (`:294-361`); 400/401/403/404/413 y
`OAuthError` nunca se reintentan. `OAuthError` → 403 genérico sin filtrar
detalles (`note_handler.go:886-891`); 403/404 Drive en lectura → 404
`note_unavailable` (`:917-931`).

Permisos nominales (desired-state, `note_service.go:1875-1881`,
`syncDrivePermissions` en `:2011-2220`): la BD
(`drive_managed_permissions`, `init.sql:286-298`; `permission_sync_status`
en `shared_notes`, `init.sql:238`) es fuente de verdad; cada transición se
persiste `pending` antes de tocar Drive y luego `in_sync`/`failed`; bajas
revocadas idempotentes ante 404; 409 al otorgar cuenta como convergencia
(`isPermissionAlreadyExists`, `:2006-2009`); visibilidad `public` y shares
`link` convergen a permiso `anyone`, `restricted` a permisos por email.
OAuth ausente al converger no revierte la operación: queda `failed`.

## 15. Sincronización, Outbox y reconciliación

PROBLEMA → MECANISMO → GARANTÍA → LIMITACIÓN:

- Creación/actualización con crash entre PG y Drive → fila
  `pending_drive`/`failed_sync` + `appProperties.notes_note_id` como índice
  de huérfanos (`drive/client.go:160-166`, `FindFileByNoteID` `:425-447`).
  El reconciliador (`ReconcilePendingNotes`, `note_service.go:2611-2718`)
  adopta el huérfano si aparece; si no puede probar ausencia (OAuth/red),
  preserva la fila. Ventana de gracia 15 min (`reconcilePendingMinAge`,
  `:2599`) superior al lease de idempotencia (2 min) para no compensar
  operaciones en vuelo; presupuesto 30 s por pasada (`:2604`); solo
  `pending_drive` antiguo sin archivo se elimina (vía `Delete`, que encola
  cleanup); `failed_sync` nunca se borra por falta de evidencia
  (`:2679-2715`).
- Borrados con Drive caído → outbox transaccional
  `drive_reconciliation_queue` (`init.sql:253-272`, migración `001:7-26`):
  `DeleteWithDriveCleanup` encola `delete_file`/`delete_attachment` en la
  misma transacción del borrado (`note_repository.go:489-573`); el worker
  reclama de a 1 con `FOR UPDATE SKIP LOCKED` y lease (`ClaimDriveOperations`,
  `:431-461`), ejecuta (`executeDriveOperation`, `note_service.go:2763-2799`;
  404 = éxito idempotente), completa o reprograma con backoff exponencial
  30 s→1 h (`reconcileDriveOperations`, `:2720-2761`). Hasta 100 ops por tick.
- Convergencia de ACL → `reconcileManagedPermissions` (`:2226-2267`):
  re-deriva el estado deseado (visibilidad + shares) para notas con sync
  pendiente y converge idempotentemente bajo lock por nota.
- Compensaciones con DLQ: si PG falla tras Drive OK y el borrado compensatorio
  también falla, se encola outbox y se loguea `[CRITICAL_UNRECONCILED]`
  (`note_service.go:856-860`, `:1157-1160`, `:1275-1278`, `:1412-1415`).

Tests: `reconciliation_test.go`, `reconcile_drive_test.go`,
`drive_outbox_test.go`, `drive_outbox_integration_test.go`,
`rollback_test.go`, `remediation_v3_test.go`, `delete_cleanup_test.go`,
`spec95_test.go`, `note_handler_spec95_test.go`.

## 16. Idempotencia y concurrencia

- Idempotencia durable `notes.idempotency_keys` `UNIQUE (user_id, operation,
  idempotency_key)` (`init.sql:274-284`, migración `001:28-38`): claim
  `in_progress` con `resource_id` preasignado (crash recovery lógico) y lease
  2 min (`idemClaimTTL`, `note_service.go:127-130`); `completed` permite replay
  si coincide `request_hash` (sha256 del payload, `:136-143`); `recoverable`
  tras fallo post-inserción permite retry inmediato sin 409 falso
  (`:247-258`, `MarkIdempotencyRecoverable` en
  `repository/note_repository.go:359`); hash distinto o solicitud en progreso
  → 409 (`beginIdempotentOperation`, `note_service.go:172-227`).
  Clave > 128 chars o vacía = sin idempotencia (`idemKey`, `:114-119`).
  Aplica a `create`/`copy`/`update` (`:711-741`, `:1666-1694`, `:980-1005`).
- Concurrencia: `version` optimista en `notes.notes` (migración `002:9-10`;
  `UPDATE ... WHERE version`, `note_repository.go:136-181`; `PATCH version`
  opcional, `note_service.go:1041-1045`; conflicto → 409 con revert de Drive,
  `:1226-1236`); lock distribuido por nota
  `pg_advisory_xact_lock(hashtextextended(noteID,0))` en transacción para
  Share/Unshare/Update/reconciliador (`shared_repository.go:43-70`); el cron
  serializa solo sus propias ejecuciones (`reconcileMu`,
  `note_service.go:517`).

Tests: `idempotency_recoverable_test.go`,
`idempotency_integration_test.go`, `note_version_lock_test.go`,
`note_repository_version_test.go`.

## 17. Modelo de datos

| Tabla | Propósito | Reglas/constraints importantes |
|---|---|---|
| `notes.notes` | Metadata de apuntes | `visibility public/private`; `likes_count` default 0; `forked_from_note_id → notes(id) ON DELETE SET NULL`; `sync_status` (`pending_drive/failed_sync/synced`); `version` optimista; índices `user_id`, `subject_id`, `visibility` (`init.sql:162-181`) |
| `notes.note_attachments` | Adjuntos en Drive del autor | `external_file_id NOT NULL`; `is_inline`; `CASCADE` por nota (`init.sql:186-198`) |
| `notes.saved_notes` | Bookmarks por referencia | `UNIQUE (user_id, note_id)`; `CASCADE` por nota (`init.sql:202-210`) |
| `notes.note_likes` | Un like por usuario | `UNIQUE (note_id, user_id)`; `CASCADE` por nota (`init.sql:214-222`) |
| `notes.shared_notes` | Vínculos nota↔grupo | `access_mode link/restricted`; `is_admin_note`; `author_followers_snapshot`; `permission_sync_status`; índice `(group_id, is_admin_note DESC)` (`init.sql:231-249`) |
| `notes.drive_reconciliation_queue` | Outbox de cleanup Drive | `status pending/failed/processing/completed`; `attempts`, `next_attempt_at`, `payload jsonb`; índice parcial pending/failed (`init.sql:253-272`) |
| `notes.idempotency_keys` | Idempotencia durable | `UNIQUE (user_id, operation, idempotency_key)`; `status in_progress/completed/recoverable` (`init.sql:274-284`) |
| `notes.drive_managed_permissions` | ACL deseado/observado por nota | `UNIQUE (note_id, principal_type, principal_key)`; `principal_type anyone/email`; `sync_status` (`init.sql:286-298`) |

`db/queries/notes.sql` (sqlc de referencia: `CreateNote :1-4`,
`GetNoteByID :6-8`, `ListNotesByUser :10-12`, `UpdateNote :14-17`,
`Like/Unlike`, `SaveNote :41-42`, `CreateSharedNote :59-62`) no es el código
ejecutado en todas las rutas: el runtime usa el SQL inline de
`internal/repository/` (p. ej. `note_repository.go`, `shared_repository.go`).

## 18. Integración con Identity y Social

- Notes → Identity: sin llamadas directas; lee tokens Drive desde
  `identity.oauth_connections` vía `PGOAuthTokenStore`
  (`drive/oauth.go:37-80`) y valida JWT localmente con el mismo secreto/
  issuer/audience (`middleware/auth_middleware.go:38-72`,
  `config.go:96-102`). Conexión OAuth la gestiona Auth
  (`POST /auth/google-drive/connect`, `back/auth/cmd/server/main.go:89-91`).
- Notes → Social (HTTP, no gRPC): `SocialHTTPAdapter` contra
  `SOCIAL_SERVICE_URL` con timeout y `X-Internal-Key`
  (`service/social_adapter.go:175-209`); consume
  `GET /internal/groups/{groupID}/members`,
  `GET /internal/groups/{groupID}/member-emails`,
  `GET /internal/users/{userID}/followers-count` (`:19-29`). `IsMember`
  fail-closed (error → 500), `GetFollowersCount` best-effort (`shareLocked`,
  `note_service.go:1914-1922`).
- Social → Notes: `POST /notes/unshare-all` HTTP con JWT (ver §11.3), no
  gRPC. No hay RPC `AccountDeletionCleanup` consumido por Notes.

**Dependencia pendiente:** Social no registra ninguna ruta `/internal/*`
(solo `/groups/*` públicas en
`back/social/internal/handler/http/group_handler.go:25-38` y resto en
`back/social/cmd/server/main.go`); tampoco existe tabla/endpooint de
follows/`followers-count` en el código actual de Social. Por tanto, con un
Social real, `IsMember` falla (fail-closed → `Share`/`ListGroupNotes`
responden 500) y `ListMemberEmails`/`GetFollowersCount` no resuelven: el modo
`restricted` end-to-end (Notes → members → emails → Drive) no es verificable
contra Social actual. **Implementado actualmente:** persistencia del share,
cálculo del estado deseado, grants/revokes nominales y reconciliación
(probado con `MemoryMemberDirectory`/`MemorySocialResolver` en tests).
**Dependencia pendiente:** endpoints internos de Social
(`member-emails`, `followers-count`, `members` servicio-a-servicio).

## 19. Drift / búsqueda offline

Vive en Flutter, no en el backend. `LocalNotes{id, title, content,
visibility, updated_at, version?, ownerUserId?}` más tabla virtual FTS5
`local_notes_fts(title, content)`
(`front/lib/core/database/app_database.dart:10-50`, tokenize
`porter unicode61`); triggers `local_notes_ai/ad/au` + `rebuild`
(`:110-134`); `ftsAvailable` con fallback LIKE (`:61-64`,
`local_notes_repository.dart:56-68`, `:106-120`).
`LocalNotesRepository.searchNotesFts` filtra por dueño, tokeniza por prefijo
y rankea con `bm25`; vacío → todas (`local_notes_repository.dart:37-121`);
`upsertNote` (`:14-27`), `getAllNotes` (`:130-134`), `deleteNote`
(`:138-140`), `clearOwner` (`:149-153`). `all_notes_screen.dart` sincroniza
`GET /notes/me` (`:367`), `POST /notes` (`:737`), `GET/PATCH/DELETE
/notes/:id` (`:942`, `:986`, `:1040`), `POST /notes/:id/copy` (`:1236`),
`POST /notes/upload` + registro de adjuntos (`:4521`, `:888`, `:4641`),
contenido de adjuntos (`:5586`) y `POST /notes/reconcile` (`:673`).
Likes/saved de la pantalla son estado local demo (`:230-233`), sin llamadas a
`/like`/`/save`; no hay llamadas a `/share` ni `/groups/{id}/notes` desde
Flutter.

Estado: Implementado en Flutter (backend no participa). Sin sync bidireccional
automático: la pantalla empuja/lee explícitamente.

## 20. Pruebas de Markdown, code, math, imágenes y PDF

El backend no parsea Markdown: lo conserva tal cual (solo sanitización XSS de
§2.1) y delega imágenes/PDF a adjuntos. Evidencia:

- `attachment_content_test.go` (service/drive/handler), `mime_test.go`,
  `note_handler_upload_test.go`.
- Pantalla Flutter con verificación técnica H4 de imagen local y PDF
  (`all_notes_screen.dart:214`) y utilidades `attachment:<uuid>` /
  `attachment-pending://` (`attachment_resources.dart`,
  `authenticated_attachment_image.dart`, `note_file_picker.dart`
  restringe a `application/pdf` en `:31-33, :78-85`).

Code fences y math funcionan porque el contenido se almacena sin transformar
esas construcciones; no hay motor matemático en el backend. Estado: Parcial
(almacenamiento y adjuntos implementados; renderizado(code/math) es
responsabilidad del cliente).

## 21. Reglas de negocio consolidadas

N-01. Solo el autor modifica metadata o elimina la nota; terceros reciben 404
idéntico al inexistente (`note_service.go:1038-1040`, `:1302-1304`).
N-02. La visibilidad válida se limita a `public` y `private`
(`utils/validator.go:14-16`, `note_service.go:602-607`).
N-03. El título válido tiene 1-255 caracteres tras recorte; `/` y `\` se
normalizan a `_` (`validator.go:9-12`, `note_service.go:670-674`).
N-04. El contenido Markdown se limita a 1 MB y se sanitiza contra XSS sin
eliminar el texto (`note_service.go:280`, `:86-104`).
N-05. La lectura de nota privada ajena exige share `link` vigente o membresía
en un grupo con share `restricted`; si no, 404 (`note_service.go:568-593`).
N-06. Sin `external_file_id` o con Drive 403/404, la lectura es 404
`note_unavailable`, nunca 403/410 (`note_service.go:903-930`).
N-07. La creación persiste `pending_drive` antes de Drive y `synced` después;
fallo de Drive deja `failed_sync` reconciliable (`note_service.go:743-807`).
N-08. El borrado es PG-primero con outbox durable; fallo Drive posterior no
revierte el éxito (`note_service.go:1291-1355`,
`note_repository.go:489-573`).
N-09. Adjuntos permitidos: `image/jpeg`, `image/png`, `application/pdf` con
sniffing estricto (mismatch → 400) y tope 10 MB (`drive/mime.go:82-135`,
`note_handler.go:500-521`).
N-10. Solo el autor registra o elimina adjuntos de su nota
(`note_service.go:1369-1371`, `:1432-1434`).
N-11. Guardar es por referencia con `UNIQUE (user_id, note_id)`; duplicado →
409; el borrado del original arrastra el bookmark (`note_service.go:1575-1613`,
`init.sql:202-208`).
N-12. Un usuario mantiene como máximo un like por nota; el contador se
actualiza en la misma transacción (`like_repository.go:69-120`,
`init.sql:214-220`).
N-13. Quitar un like inexistente es éxito idempotente sin decrementar bajo
cero (`like_repository.go:99-120`).
N-14. El clon es nota nueva privada sin `subject_id` heredado, con
`forked_from_note_id` al original e independencia posterior de archivos
(`note_service.go:1696-1723`).
N-15. Compartir exige autoría y membresía del autor (fail-closed ante Social
caído) (`note_service.go:1910-1920`).
N-16. `access_mode` válido es `link` o `restricted`
(`validator.go:18-20`).
N-17. El share persiste en BD antes de converger Drive; fallo Drive queda
`failed` para el reconciliador (`note_service.go:1939-1949`).
N-18. Retirar un share lo puede hacer el autor o un admin del grupo
(`note_service.go:2311-2319`).
N-19. `unshare-all` lo invoca el propio usuario o un admin del grupo
(`note_handler.go:796-799`).
N-20. `GET /groups/{id}/notes` exige membresía y ordena admin → likes →
fecha (`note_service.go:2543-2576`).
N-21. PATCH concurrente con versión desactualizada responde 409 sin pisar
(`note_repository.go:136-181`, `note_service.go:1041-1045`).
N-22. La misma `X-Idempotency-Key` con distinto payload responde 409; el
replay idéntico no duplica (`note_service.go:172-227`).
N-23. Clonar está limitado a 10/min por usuario y 100/min global (429)
(`note_handler.go:20-34`, `:57-122`).
N-24. `restricted` otorga permisos nominales por email normalizado; `link` y
`public` otorgan `anyone` (`note_service.go:1961-1983`,
`member_directory.go:28-43`).

## 22. Limitaciones / dependencias pendientes

- `restricted` end-to-end pendiente de endpoints internos de Social (§18).
- `author_followers_snapshot` se almacena pero no ordena `ListGroupNotes`
  (§11.4).
- `Unsave` sin ruta HTTP (§8); no hay listado de guardados por HTTP.
- Sin gRPC Notes↔Social (HTTP en su lugar) ni `AccountDeletionCleanup`
  hacia Notes; `back/proto/notes/` vacío.
- `subject_id` sin validación de existencia (solo formato UUID).
- `social_adapter_test.go` cubre el adaptador con servidor falso; contra
  Social real las rutas `/internal/*` no existen.
- Conceptos antiguos (`student`/`teacher`, rol global, `teacher_class`,
  `study`, Supabase Storage para Notes, profesores administrando cursos) no
  aparecen en el código vigente de Notes; solo sobreviven como comentarios
  históricos (p. ej. `init.sql:224-225` sobre el rol `teacher` eliminado).

## 23. Resumen de implementación

| Método | Ruta | Propósito | Estado |
|---|---|---|---|
| POST | `/notes` | Crear nota (metadata + `.md` en Drive) | Completo |
| PATCH | `/notes/{id}` | Metadata + contenido + versión optimista | Completo |
| DELETE | `/notes/{id}` | Borrado PG-primero + cleanup Drive | Completo |
| GET | `/notes/{id}` | Metadata + contenido Drive + adjuntos | Completo |
| GET | `/notes/me` | Listado propio paginado | Completo |
| POST | `/notes/{id}/attachments` | Subir o registrar adjunto | Completo |
| DELETE | `/notes/{id}/attachments/{attachmentId}` | Eliminar adjunto | Completo |
| GET | `/notes/{id}/attachments/{attachmentId}/content` | Descargar adjunto | Completo |
| POST | `/notes/upload` | Subida directa a Drive sin nota | Completo |
| POST | `/notes/{id}/save` | Bookmark por referencia | Completo |
| POST | `/notes/{id}/copy` | Fork independiente en Drive propio | Completo |
| POST | `/notes/{id}/like` | Like transaccional | Completo |
| DELETE | `/notes/{id}/like` | Unlike idempotente | Completo |
| POST | `/notes/{id}/share` | Compartir `link` / `restricted` | Parcial |
| DELETE | `/notes/shared/{sharedNoteId}` | Retirar share | Completo |
| POST | `/notes/unshare-all` | Limpieza al salir de grupo | Completo |
| GET | `/groups/{id}/notes` | Notas del grupo ordenadas | Completo |
| GET | `/notes/{id}/access` | Contrato de acceso + estado Drive | Completo |
| POST | `/notes/reconcile` | Reconciliación manual Drive→App | Completo |
| GET | `/health`, `/health/drive` | Salud y modo storage | Completo |

Comandos verificados (desde `back/notes`):
`go test ./internal/service/ -run "TestCreateSuccess|TestLikeUnlikeTransactional|TestShareAndAccessControl" -count=1` → ok;
`go test ./internal/handler/http/ -run "TestHandlerCreateSuccess|TestHandlerLikeAndCopy|TestHandlerShareAndGroupNotes" -count=1` → ok.
