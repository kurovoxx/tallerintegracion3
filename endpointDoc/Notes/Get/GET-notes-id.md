# GET /notes/{id}

Devuelve la metadata de Postgres y el contenido Markdown descargado del Drive del autor, más la lista de adjuntos. Ruta interna del backend Notes (`:8082`). Desde el navegador: `GET https://ti3-brojas.dev.censei.cl/api-notes/notes/<id>` ([prefijos](../COMUN-notes.md#1-rutas-internas-y-prefijos-externos)). Equivalente gRPC: `NotesService/GetNote` (sin adjuntos, ver [doc](../Grpc/GRPC-notes-NotesService.md)).

## Req

- **Registro:** `back/notes/cmd/server/main.go:210` → `protected.GET("/notes/:id", h.Get)`. Handler `back/notes/internal/handler/http/note_handler.go:199-251` (`Get`).
- **Autenticación:** `RequireAuth` (`main.go:192`). Ver [COMUN §2](../COMUN-notes.md#2-autenticación-http).
- **Permiso de lectura** (`hasReadAccess`, `back/notes/internal/service/note_service.go:617-642`): autor, o nota `public`, o nota `private` con algún share `link`, o con un share `restricted` en un grupo del que el solicitante es miembro (Social `IsMember`).
- **Path:** `id` UUID obligatorio. No UUID → 404 `not_found` (`validateNoteID`, `note_handler.go:144-150`).
- **Query:** ninguna. **Body:** no tiene.

## Res

**200 OK** (`note_handler.go:216-250`)

```json
{
  "id": "11111111-1111-4111-8111-111111111111",
  "user_id": "22222222-2222-4222-8222-222222222222",
  "title": "Álgebra",
  "visibility": "private",
  "likes_count": 0,
  "version": 1,
  "created_at": "2026-10-22T12:00:00Z",
  "updated_at": "2026-10-22T12:00:00Z",
  "subject_id": "6b0c1f0e-2f4a-4c1e-9d7a-3a1b2c3d4e5f",
  "external_file_id": "1AbCdriveFileId",
  "drive_url": "https://drive.google.com/file/d/1AbCdriveFileId/view",
  "content": "# Matrices\n",
  "saves_count": 2,
  "is_liked": false,
  "is_saved": true,
  "is_own": false,
  "attachments": []
}
```

| Campo | Significado |
| --- | --- |
| `id`, `user_id` | UUID de la nota y de su autor |
| `title`, `visibility` | Metadata (`public` \| `private`) |
| `likes_count` | Contador mantenido por like/unlike |
| `version` | Versión para el bloqueo optimista de PATCH |
| `saves_count`, `is_liked`, `is_saved` | Contadores exactos y estado del solicitante (`NoteService.Interactions`); se omiten solo si su lectura falla |
| `is_own` | `true` si el solicitante es el autor |
| `created_at`, `updated_at` | RFC3339 en UTC |
| `subject_id` | Solo si existe (`L226-228`) |
| `external_file_id`, `drive_url` | ID del `.md` en Drive y enlace web construido a partir de él; solo si existe (`L229-233`). En la práctica siempre está en un 200, porque sin archivo el servicio responde 404 |
| `forked_from_note_id` | Solo si la nota es una copia (`L234-236`) |
| `content` | Markdown leído de Drive con el token del autor (`L237-239`) |
| `attachments` | Siempre lista; `[]` si no hay (`L240-249`). Objetos `model.Attachment` (`back/notes/internal/model/note.go`) |

No incluye `sync_status` (sí lo incluye el RPC).

**Errores**

| HTTP | `error.code` | Condición |
| --- | --- | --- |
| 404 | `not_found` "nota no encontrada" | `id` no UUID, la nota no existe en Postgres, o es privada y el solicitante no tiene acceso. Ambos casos son indistinguibles a propósito (`note_service.go:957-976`) |
| 410 | `note_deleted` "Apunte no disponible (eliminado por el autor)" | La fila no existe pero hay lápida en `notes.note_tombstones` y el apunte era público o el solicitante lo tenía guardado. Para el propio autor o un privado ajeno se mantiene el 404 |
| 404 | `note_unavailable` "Nota no disponible en almacenamiento remoto" | La nota existe y es legible, pero no tiene `external_file_id`, o Drive responde 404/403, o el **autor** perdió su conexión OAuth (`L979-1005`) |
| 502 | `internal_error` "Servicio de almacenamiento no disponible" | Drive caído o timeout (`L1006`) |
| 500 | `internal_error` "Error interno" | Fallo de Postgres, de Social al comprobar membresía (`L632-635`), o al listar adjuntos (`note_handler.go:240-244`) |
| 401 / 423 / 503 | ver [COMUN §2](../COMUN-notes.md#2-autenticación-http) | Middleware |

No existe un 403 en esta ruta: un OAuth revocado del autor se convierte en `note_unavailable` (404) dentro del servicio antes de llegar al handler (`note_service.go:992-994`). (El documento anterior indicaba "403 OAuth"; no coincide con el código.)

**Efectos:** solo lectura. No escribe en Postgres ni en Drive. Nada queda en segundo plano.

## Invocación (Front / Otros Servicios)

1. **Detalle de nota propia** — `AllNotesScreen`.
   - Acción: tocar una tarjeta abre `_openNoteViewer` (`front/lib/features/notes/all_notes_screen.dart:2370`), que muestra `_NoteDetailSheet`; su `initState` llama `_loadRemote` (`L3886`, `L3956-3962`) → `widget.onFetchRemote` → `_fetchHybridContent` (`L2433`, `L1625-1675`). También se llama antes de un PATCH sin versión conocida (`L1684-1689`).
   - Petición: `GET $notesBaseUrl/notes/<id>` con `_authHeaders()` (Bearer `SessionManager.token`, `L1540-1550`), timeout 8 s, vía `AuthedHttp.run`. Solo si el id es UUID remoto (`L1626-1628`).
   - Respuesta: con 200 guarda `version` en `_versions` y actualiza la fila en Drift con `title`/`content` (`_applyLocalUpsert`, `L1652-1659`), salvo que la nota local tenga cambios pendientes (`needsSync`), en cuyo caso devuelve el borrador local (`L1643-1650`). Los errores se muestran con textos como "No tienes acceso a esta nota." para 403 (`L3950-3952`).
2. **Abrir nota compartida en un grupo** — `GroupNotesTab`.
   - Acción: botón **Abrir** de una nota compartida → `_openNote` (`front/lib/features/groups/group_notes_tab.dart:100-104`).
   - Cliente: `NotesSharingService.getNoteContent` (`front/lib/core/services/notes_sharing_service.dart:283-308`): `GET {notesApiBaseUrl}/notes/<id>` con `Authorization: Bearer` (`L113-126`), timeout 15 s. Usa `title`, `content` y `attachments`; no escribe en Drift.
- **Otros microservicios:** No se encontró invocación en el código revisado.
- **Tests/scripts:** `scripts/test-notes-e2e.py:104-105,149`.

## Flujo

1. `main.go:210` → `RequireAuth` (`auth_middleware.go:91`).
2. `NoteHandler.Get` (`note_handler.go:199`): `GetUserID`, `validateNoteID`.
3. `NoteService.Get` (`note_service.go:956`): `GetByID` (`service/store_pg.go:25` → `repository/note_repository.go:72`).
4. `hasReadAccess` (`L617`): `SharedStore.ListByNote` (`store_pg.go:124` → `repository/shared_repository.go:107`) y, para shares `restricted`, `SocialResolver.IsMember` → gRPC `social.v1.SocialService/IsMember` (`service/social_grpc_adapter.go:57-68`, síncrono, timeout 5 s).
5. Drive (síncrono): `GetFileContent` con el token del autor (`note_service.go:982-989`; `drive/real_client.go:192`).
6. Handler: `ListAttachments` (`note_service.go:1546` → `repository/attachment_repository.go:57`), arma el JSON y responde 200.

Finalmente devuelve `200` con metadata, `content` y `attachments`, y la pantalla de detalle (o la pestaña de notas del grupo) utiliza ese resultado para mostrar el Markdown del servidor y, en el caso de notas propias, refrescar la copia local en Drift y la versión usada por el siguiente PATCH.

## Tests

| Archivo:línea | Prueba | Qué comprueba | Tipo |
| --- | --- | --- | --- |
| `back/notes/internal/handler/http/note_handler_test.go:144` | `TestHandlerGetNotFound404` | Nota inexistente → 404 | Unitaria |
| `note_handler_test.go:158` | `TestHandlerGetWithDriveResilienceAlternative` | Respuesta ante fallos de Drive | Unitaria (Drive mock) |
| `note_handler_test.go:215` | `TestHandlerPrivateAccessNotFound` | Privada ajena → 404 | Unitaria |
| `audit_fixes_test.go:144` | `TestHandlerInvalidUUID404` | `id` no UUID → 404 | Unitaria |
| `audit_fixes_test.go:411` | `TestHandlerZeroKnowledgePrivateNotFound` | Privada ajena indistinguible de inexistente | Unitaria |
| `access_contract_test.go:57,234` | `TestNoteListsEmptyArrays`, `TestNoteListsUTCRFC3339` | `attachments: []` y fechas UTC | Unitaria |
| `back/notes/internal/service/note_service_test.go:163,177,201` | `TestGetWithContent`, `TestGetDriveNotFound`, `TestGetDriveForbidden` | Contenido y mapeo de errores de Drive | Unitaria |
| `note_service_test.go:1060` | `TestDriveGetMaps403And404ToNoteUnavailable` | 403/404 de Drive → `note_unavailable` | Unitaria |
| `service/like_read_contract_test.go:138` | `TestGetMissingRemoteFile` | Sin archivo remoto | Unitaria |
| `service/remediation_v3_test.go:192` | `TestZeroKnowledgePrivateAndMissingIndistinguishable` | Mismo error para privada y ausente | Unitaria |
| `note_service_test.go:645` | `TestPublicNoteAccessibleWithoutShare` | Pública legible sin share | Unitaria |
| `back/notes/internal/handler/grpc/server_test.go:254` | `TestGetNoteForeignAndMissingAreNotFound` | Equivalente gRPC (bufconn) | Unitaria gRPC |
| `front/test/notes_edit_lifecycle_test.dart:35` | `ediciones consecutivas, cache, JWT y adjuntos local=$local` | Lectura híbrida y JWT en el detalle (HTTP falso) | Widget |
| `front/test/group_notes_tab_test.dart:108` | `notas compartidas ${size.width} escala $scale: estados y diálogo` | Pulsa ABRIR en una nota compartida; el `MockClient` responde `GET /notes/<id>` (`L72`) | Widget (HTTP falso) |
