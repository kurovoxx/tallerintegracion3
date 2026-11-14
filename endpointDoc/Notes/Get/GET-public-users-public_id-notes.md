# GET /public-users/{public_id}/notes

Lista los apuntes públicos del perfil identificado por `public_id`. Ruta
interna del backend Notes (`:8082`); desde el navegador se antepone el prefijo
externo ([prefijos](../COMUN-notes.md#1-rutas-internas-y-prefijos-externos)).
No tiene equivalente gRPC.

## Req

- **Registro:** `back/notes/cmd/server/main.go` → grupo propio con
  `middleware.PrivateNoStore()` **antes** de `authMw.RequireAuth()` y
  `GET("/public-users/:public_id/notes", h.ListPublicByPublicID)`.
- **Autenticación:** JWT obligatorio. Ver [COMUN §2](../COMUN-notes.md#2-autenticación-http).
- **Path:** `public_id`, 12 caracteres Crockford Base32 en minúsculas.
- **Query:** `cursor` (UUID del último apunte recibido, opcional) y `limit`
  (20 por defecto, 100 máximo; valores fuera de rango se normalizan).

## Res

**200 OK**

```json
{
  "notes": [
    {
      "id": "11111111-1111-4111-8111-111111111111",
      "title": "Cálculo I",
      "subject_id": null,
      "visibility": "public",
      "likes_count": 3,
      "saves_count": 1,
      "is_liked": false,
      "is_saved": true,
      "created_at": "2026-11-01T10:00:00Z",
      "updated_at": "2026-11-02T10:00:00Z"
    }
  ],
  "next_cursor": ""
}
```

- No incluye el UUID del autor, `external_file_id`, `sync_status`,
  `forked_from_note_id` ni el contenido Markdown (se lee con
  `GET /notes/{id}`).
- Solo filas con `visibility='public'` **y** `external_file_id` no nulo, en
  orden `created_at DESC, id DESC`. Los apuntes privados nunca aparecen.
- La visibilidad del **perfil** no filtra este listado: un perfil `private`
  oculta sus datos en Auth, pero sus apuntes públicos siguen visibles para
  cualquier usuario autenticado.
- `is_liked` / `is_saved` son del solicitante; los contadores se resuelven en
  lote (3 consultas por página).

**Errores**

| HTTP | `error.code` | Condición |
| --- | --- | --- |
| 404 | `not_found` | `public_id` inválido o inexistente (indistinguibles) |
| 400 | `bad_request` | `cursor` no es UUID |
| 503 | `service_unavailable` | Identity caído, sin `INTERNAL_API_KEY`, sin `IDENTITY_GRPC_ADDR` o respuesta inválida |
| 500 | `internal_error` | Fallo de PostgreSQL |
| 401 / 423 / 503 | ver COMUN §2 | Middleware |

Un cursor inexistente, privado, sin archivo o de otro autor devuelve lista
vacía (nunca reinicia la paginación ni salta los filtros). Todas las
respuestas llevan `Cache-Control: private, no-store`.

## Invocación (Front)

- `NotesApi.listPublicByPublicId` (`front/lib/features/notes/notes_api.dart`):
  valida el ID localmente y traduce 404 → `profile_not_found` y 502/503/504 →
  `profile_service_unavailable`.
- `PublicProfileScreen` la llama también cuando Auth responde 404 (perfil
  privado): muestra «Perfil privado» y la lista de apuntes públicos, y solo
  muestra «Perfil no disponible» si Notes también responde 404. «Ver apunte» abre
  `PublicNotePreviewScreen`, que lee el apunte con `GET /notes/{id}`.

## Flujo

1. `NoteHandler.ListPublicByPublicID` (`internal/handler/http/note_handler.go`).
2. `NoteService.ListPublicByPublicID` (`internal/service/note_service.go`):
   valida `public_id` y `cursor`, resuelve el `user_id` con
   `ResolvePublicProfile` (ver
   [GRPC ResolvePublicProfile](../../Auth/Grpc/GRPC-auth-IdentityService-ResolvePublicProfile.md)),
   y lista con
   `NoteStore.ListPublicByUser` (`repository/note_repository.go`).
3. `interactionsForTargets` (`service/saved_interactions.go`) arma los
   contadores de la página.

## Tests

- `back/notes/internal/handler/http/public_profile_notes_handler_test.go`
  (401, DTO sin UUID, 404 inexistente, 503).
- `back/notes/internal/service/public_profile_notes_test.go` y
  `public_notes_test.go` (filtrado, paginación, cursores, límites, sin N+1).
  La visibilidad del perfil no llega a Notes; el caso «perfil privado con
  apuntes visibles» se prueba en `front/test/public_profile_screen_test.dart`.
- `back/notes/internal/repository/public_notes_repository_test.go` y
  `public_notes_pg_integration_test.go` (consulta SQL; la integración requiere
  `NOTES_TEST_DATABASE_URL`).
- `front/test/notes_public_saved_test.dart`, `front/test/public_profile_screen_test.dart`.
