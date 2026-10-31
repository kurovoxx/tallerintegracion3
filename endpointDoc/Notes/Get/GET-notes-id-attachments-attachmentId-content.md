# `GET /notes/:id/attachments/:attachmentId/content`


## Req

- `GET /notes/:id/attachments/:attachmentId/content`, protegido por JWT: `back/notes/cmd/server/main.go:201`.
- id y attachmentId identifican nota y adjunto. Sin body ni query. La identidad del solicitante procede del JWT.
- Base de servicio Notes; el prefijo del gateway `/api-notes` se reescribe y no pertenece al path Gin.

## Res

- `200 OK`: devuelve bytes con `Content-Type` del adjunto. `Content-Disposition` es `inline` para tipos permitidos y `attachment` para otros; también incluye `Cache-Control: private, no-store` y `X-Content-Type-Options: nosniff`.
- `401 Unauthorized`: JWT ausente/inválido/expirado; error `unauthorized` con `No autorizado: token ausente, inválido o expirado`.
- `404 Not Found`: ID malformado, nota sin acceso, adjunto ausente o archivo Drive inaccesible. El fallo remoto devuelve `note_unavailable` con `Nota no disponible en almacenamiento remoto`, sin revelar contenido de terceros.
- `413 Content Too Large`: adjunto mayor al límite; `file_too_large` con `Archivo muy grande`.
- `502 Bad Gateway`: Drive no disponible; `internal_error` con `Servicio de almacenamiento no disponible`.
- `500 Internal Server Error`: fallo PostgreSQL u otro error no tipado; `internal_error` con `Error interno`. `handleServiceError` define los casos (`back/notes/internal/handler/http/note_handler.go:890-941`). Los errores usan `{"error":{"code":"...","message":"..."}}` (`back/notes/internal/handler/http/telemetry.go:56-61`).
- El servicio comprueba permisos Notes y descarga con la conexión Drive del dueño mediante Identity gRPC; no redirige a un enlace público.

## Invocación (Front / Otros Servicios)

- `AllNotesScreen._buildResourceImage` construye el URI para imágenes inline (`front/lib/features/notes/all_notes_screen.dart:6425-6459`); `_AuthenticatedAttachmentImageState._load` solicita los bytes con el JWT (`front/lib/features/notes/authenticated_attachment_image.dart:67-100`).
- `GroupNotesTab` también crea `AuthenticatedAttachmentImage` para un adjunto compartido (`front/lib/features/groups/group_notes_tab.dart:507-515`).
- No se encontró consumidor backend directo en producción.

## Flujo

1. `main()` registra la ruta protegida (`back/notes/cmd/server/main.go:201`).
2. `NoteHandler.AttachmentContent` obtiene actor e IDs y prepara headers y bytes de respuesta (`back/notes/internal/handler/http/attachment_content.go:11-35`).
3. `NoteService.GetAttachmentContent` aplica autorización y verifica que el adjunto pertenezca a la nota (`back/notes/internal/service/attachment_content.go:12-36`); `PGNoteStore.GetByID`, `PGAttachmentStore.GetByID` y `AttachmentRepository.GetByID` leen la metadata (`back/notes/internal/service/store_pg.go:25-27,61-63`; `back/notes/internal/repository/attachment_repository.go:40-55`).
4. `GRPCOAuthTokenProvider.GetValidAccessToken` obtiene el token OAuth del dueño mediante Identity (`back/notes/internal/drive/oauth_grpc.go:62-89`; `back/auth/internal/handler/grpc/identity_server.go:118-151`).
5. `RealDriveClient.DownloadAttachment` descarga bytes desde Drive (`back/notes/internal/drive/attachment_content.go:12-36`). El handler devuelve `200` con `Content-Disposition`, `Cache-Control: private, no-store` y el tipo MIME (`back/notes/internal/handler/http/attachment_content.go:23-35`). Flutter presenta los bytes en `_AuthenticatedAttachmentImageState._load` (`front/lib/features/notes/authenticated_attachment_image.dart:67-100`).

## Tests

- `back/notes/internal/handler/http/attachment_content_test.go:13`, `TestAttachmentContentAuthorizationAndMIME`.
- `back/notes/internal/service/attachment_content_test.go:9`, `TestAttachmentContentSharePolicy`.
- `back/notes/internal/drive/attachment_content_test.go:12`, `l`ímite y contenido con Drive falso.
- Estas pruebas no contactan Drive real.
