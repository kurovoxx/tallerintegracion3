# Notes: matriz de contrato y cobertura

Inventario del router de producción `back/notes/cmd/server/main.go`, revisión Sprint 3.
**21 rutas HTTP = 19 con JWT + 2 públicas de salud.** No existe ruta HTTP Unsave.
Los 10 RPC se documentan en [NotesService](../endpointDoc/GRPC-notes-NotesService.md).
Cada documento enlazado sigue Req / Res / Flujo / Tests. La autenticación común,
errores, despliegue y consistencia están en [docHector](docHector.md).

Las columnas distinguen pruebas de handler con stores en memoria (H), dominio
unitario (U), integración PostgreSQL (I) y proceso HTTP real en Docker (E).
Un doble de Drive comprueba el contrato y los fallos inyectados, no certifica
OAuth ni permisos en Google real. La cobertura de rutas tampoco equivale a
cobertura exhaustiva de ramas/errores.

## Contrato HTTP

| Método | Ruta / documento | Handler | Servicio | Autenticación y permiso | Entrada | Salida | Errores específicos |
| --- | --- | --- | --- | --- | --- | --- | --- |
| POST | [/notes](../endpointDoc/POST-notes.md) | Create | Create | JWT; Autor = JWT | JSON title, visibility, subject_id?, content?; X-Idempotency-Key opcional | 201 {note_id} | 400 validación; 403 OAuth; 409 clave/payload; 413 tamaño; 502 Drive |
| GET | [/notes/me](../endpointDoc/GET-notes-me.md) | ListMy | ListMy | JWT; Solo notas propias | Query cursor?, limit? (default 20, máximo 100) | 200 {notes:[],next_cursor:""} | 500 persistencia |
| GET | [/notes/:id](../endpointDoc/GET-notes-id.md) | Get | Get + ListAttachments | JWT; Autor, pública, link o miembro de grupo restricted | Path id UUID | 200 metadata + content? + attachments:[] | 404 inexistente/privada sin acceso/Drive ausente; 403 OAuth; 502 Drive; 500 DB |
| PATCH | [/notes/:id](../endpointDoc/PATCH-notes-id.md) | Patch | UpdateWithExpectedVersion | JWT; Solo autor | JSON title?, visibility?, content?, version?; X-Idempotency-Key opcional | 200 {id,title,visibility,version,updated_at} | 400 vacío/validación; 404 ajena/ausente; 409 versión/clave; 413 tamaño; 403 OAuth; 502 Drive; 500 DB |
| DELETE | [/notes/:id](../endpointDoc/DELETE-notes-id.md) | Delete | Delete | JWT; Solo autor | Path id UUID | 204 sin cuerpo | 404 ajena/ausente; 500 DB |
| POST | [/notes/:id/attachments](../endpointDoc/POST-notes-id-attachments.md) | UploadAttachment | AddAttachment / AddAttachmentExternal | JWT; Solo autor | Multipart file, is_inline?; o JSON external_file_id, file_name?, file_type?, file_size_bytes?, is_inline? | 201 {attachment_id,is_inline,file_url,external_file_id} | 400 vacío/MIME declarado distinto; 404 nota ajena/ausente; 413 tamaño; 415 tipo; 403 OAuth; 502 Drive; 500 DB |
| GET | [/notes/:id/attachments/:attachmentId/content](../endpointDoc/GET-notes-id-attachments-attachmentId-content.md) | AttachmentContent | GetAttachmentContent | JWT; Acceso de lectura a nota y pertenencia del adjunto a ella | Path id y attachmentId UUID | 200 bytes con Content-Type | 404 nota/adjunto ausente o sin acceso; 403 OAuth; 502 Drive; 500 DB |
| DELETE | [/notes/:id/attachments/:attachmentId](../endpointDoc/DELETE-notes-id-attachments-attachmentId.md) | DeleteAttachment | RemoveAttachment | JWT; Solo autor; adjunto pertenece a nota | Path id y attachmentId UUID | 204 sin cuerpo | 404 nota/adjunto/autor; 500 DB |
| POST | [/notes/upload](../endpointDoc/POST-notes-upload.md) | UploadFile | UploadToDrive | JWT; Usuario JWT, Drive propio | Multipart file (obligatorio) | 201 {external_file_id,file_url,file_name,file_type,file_size_bytes,is_inline:false} | 400 vacío/MIME; 413 tamaño; 415 tipo; 403 OAuth; 502 Drive |
| POST | [/notes/:id/save](../endpointDoc/POST-notes-id-save.md) | Save | Save | JWT; Lectura autorizada | Path id UUID; sin body | 201 sin cuerpo | 404 sin acceso/ausente; 409 already_saved; 500 DB |
| POST | [/notes/:id/copy](../endpointDoc/POST-notes-id-copy.md) | Copy | Copy | JWT; Lectura origen y Drive destino conectado | Path id UUID; X-Idempotency-Key opcional | 201 {note_id,forked_from_note_id} | 404 sin acceso/origen no disponible; 403 OAuth destino; 409 clave; 429 cuota; 502 Drive; 500 DB |
| POST | [/notes/:id/like](../endpointDoc/POST-notes-id-like.md) | Like | Like | JWT; Lectura autorizada | Path id UUID; sin body | 201 sin cuerpo | 404 sin acceso/ausente; 409 already_liked; 500 DB |
| DELETE | [/notes/:id/like](../endpointDoc/DELETE-notes-id-like.md) | Unlike | Unlike | JWT; JWT; solo elimina el like propio | Path id UUID; sin body | 204 sin cuerpo | 404 formato UUID inválido; 500 DB |
| POST | [/notes/:id/share](../endpointDoc/POST-notes-id-share.md) | Share | Share | JWT; Autor y miembro del grupo | JSON group_id UUID, access_mode link\|restricted | 201 {shared_note_id} | 400 validación; 403 autor/membresía; 404 nota; 413 JSON; 500 DB/directorio |
| DELETE | [/notes/shared/:sharedNoteId](../endpointDoc/DELETE-notes-shared-sharedNoteId.md) | Unshare | Unshare | JWT; Autor de nota o admin del grupo | Path sharedNoteId UUID | 204 sin cuerpo | 404 vínculo ausente; 403 permisos; 500 DB/Social |
| POST | [/notes/unshare-all](../endpointDoc/POST-notes-unshare-all.md) | UnshareAll | UnshareAll | JWT; Usuario objetivo o admin del grupo (JWT) | JSON user_id UUID, group_id UUID | 204 sin cuerpo | 400 JSON/UUID; 403 permisos; 413 JSON; 500 DB |
| GET | [/groups/:id/notes](../endpointDoc/GET-groups-id-notes.md) | ListGroupNotes | ListGroupNotes → ListGroupNotePairs | JWT; Miembro del grupo | Path id UUID; cursor?, limit? | 200 {notes:[],next_cursor:""} | 404 UUID inválido; 403 no miembro; 500 DB/Social |
| GET | [/notes/:id/access](../endpointDoc/GET-notes-id-access.md) | GetAccess | GetAccess | JWT; Autor, pública, link o miembro restricted | Path id UUID | 200 NoteAccessResponse | 404 inaccesible/ausente/sin archivo; 500 DB/Social |
| POST | [/notes/reconcile](../endpointDoc/POST-notes-reconcile.md) | Reconcile | ReconcileDriveDeletions | JWT; Solo recursos propios | Sin body | 200 {removed_notes,removed_attachments,pending,removed_note_ids} | 403 OAuth; 502 Drive; 500 DB |
| GET | [/health](../endpointDoc/GET-notes-health.md) | inline newHTTPRouter | Ninguno | Pública; Pública | Sin body | 200 {status:"UP",service:"notes-service"} | Sin errores de dominio |
| GET | [/health/drive](../endpointDoc/GET-notes-health-drive.md) | inline newHTTPRouter | Ninguno | Pública; Pública | Sin body | 200 {storage_mode} | Sin errores de dominio |

## Evidencia por endpoint

H: `back/notes/internal/handler/http/`, salvo router en `cmd/server/`.
U: `back/notes/internal/service/`, salvo indicación explícita.
I: `TestPostgresConsecutiveNoteEdits` (crear/leer/editar/listar) y
`TestIdempotencyClaimAfterExpiryPreservesResourceID` (claim, no endpoint HTTP).
E: `scripts/test-notes-e2e.py`, contra el Compose aislado: HTTP real, JWT emitido
por Auth real, repositorios PostgreSQL y llamadas gRPC reales; Drive mock.
La ejecución y resultados finales se registran en el informe interno de cierre (no versionado).

| Ruta | H | U | I | E |
| --- | --- | --- | --- | --- |
| `POST /notes` | TestHandlerCreateIdempotencyReplayAndConflict | TestCreateRetryAfterReplayWindowAdoptsExistingNote; TestCreateConcurrentRetriesSameKeyCreateOneNote | PG claim (repositorio) | E: escenario documentado en script |
| `GET /notes/me` | TestHandlerListMyPagination | TestListMyPagination | PG edits | E: escenario documentado en script |
| `GET /notes/:id` | TestHandlerGetWithDriveResilienceAlternative; TestHandlerPrivateAccessNotFound | TestGetWithContent; TestZeroKnowledgePrivateAndMissingIndistinguishable | PG edits | E: escenario documentado en script |
| `PATCH /notes/:id` | TestHandlerPatchVersionConflict; TestHandlerPatchIdempotencyReplay | TestPermissionConcurrentUpdatesSingleWinner; TestUpdateIdempotencyReplayAndErrorCleanup | PG edits | E: escenario documentado en script |
| `DELETE /notes/:id` | TestHandlerDeleteNoteAndAllFiles | TestDeleteNoteAllDriveFiles; TestDeleteCleanupToleratesMissingDriveFilesAndNetworkFailures | E usa PG; sin test I dedicado | E: escenario documentado en script |
| `POST /notes/:id/attachments` | TestHandlerAttachmentBoundary10MB; TestHandlerExternalAttachment; TestHandlerAttachmentRejectsMimeMismatch | TestAddAttachment | E usa PG; sin test I dedicado | E: escenario documentado en script |
| `GET /notes/:id/attachments/:attachmentId/content` | TestAttachmentContentAuthorizationAndMIME | TestAttachmentContentSharePolicy | E usa PG; sin test I dedicado | E: escenario documentado en script |
| `DELETE /notes/:id/attachments/:attachmentId` | TestHandlerAttachment | TestDeleteAttachmentDurableRecovery | E usa PG; sin test I dedicado | E: escenario documentado en script |
| `POST /notes/upload` | TestHandlerUploadDirectToDrive; TestHandlerUploadPdfThenLink | UploadToDrive ejercitado por handlers | E usa PG; sin test I dedicado | E: escenario documentado en script |
| `POST /notes/:id/save` | TestHandlerSaveConflict | TestSaveAndUnsave | E usa PG; sin test I dedicado | E: escenario documentado en script |
| `POST /notes/:id/copy` | TestHandlerCopyIdempotencyReplayAndConflict; TestHandlerCopyRateLimitReturns429 | TestIdempotencyCopyCrashRecoveryReusesPreassignedClone | E usa PG; sin test I dedicado | E: escenario documentado en script |
| `POST /notes/:id/like` | TestHandlerLikeAndCopy; TestConcurrentLikes | TestLikeReadAccess | E usa PG; sin test I dedicado | E: escenario documentado en script |
| `DELETE /notes/:id/like` | TestHandlerLikeAndCopy | TestLikeUnlikeOnlyDecrementsDeletedLike | E usa PG; sin test I dedicado | E: escenario documentado en script |
| `POST /notes/:id/share` | TestHandlerShareRestrictedGrantsDrive; TestHandlerShareAndGroupNotes | TestRestrictedSharePersistsEmailPermissionIDs; TestRestrictedUsesGoogleEmailsNotAppEmails | E usa PG; sin test I dedicado | E: escenario documentado en script |
| `DELETE /notes/shared/:sharedNoteId` | TestHandlerShareAndGroupNotes | TestUnsharePreservesOtherGroupPermissions; TestUnshareRevokesNominalPermissionAndClearsRow | E usa PG; sin test I dedicado | E: escenario documentado en script |
| `POST /notes/unshare-all` | TestUnshareAllIDORForbidden; TestUnshareAllSelfAllowedAndAdminAllowed | TestUnshareAllRevokesOnlyOwnerGroupAcrossPages; TestUnshareAllRevokesLeaverOnOtherAuthorsNote | E usa PG; sin test I dedicado | E: escenario documentado en script |
| `GET /groups/:id/notes` | TestHandlerShareAndGroupNotes; TestListGroupNotesOrdering | ListGroupNotePairs ejercitado por TestListGroupNotesPaginationAndOrdering (gRPC) | E usa PG; sin test I dedicado | E: escenario documentado en script |
| `GET /notes/:id/access` | TestAccessJSONContract; TestAccessJSONContractDriveConnectionRequired | GetAccess ejercitado por access_contract_test.go | E usa PG; sin test I dedicado | E: escenario documentado en script |
| `POST /notes/reconcile` | TestHandlerReconcileDriveDeletions | TestReconcileTimeoutNeverDeletes; TestReconcileRevokedOAuthNeverDeletes | E usa PG; sin test I dedicado | E: escenario documentado en script |
| `GET /health` | TestHTTPRouterContract | No aplica | E usa PG; sin test I dedicado | E: escenario documentado en script |
| `GET /health/drive` | TestHTTPRouterContract | No aplica | E usa PG; sin test I dedicado | E: escenario documentado en script |
