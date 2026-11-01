# gRPC notes.v1.NotesService

Puerto interno 9082. Fuente: `back/proto/notes/v1/notes.proto`;
implementación `back/notes/internal/handler/grpc/server.go`.

## Req

9 RPC usan metadata `authorization: Bearer <JWT>` HS256 (issuer apuntes-auth,
audience apuntes-client). UnshareAll usa x-internal-key y rechaza JWT como
sustituto. Create/Update reciben idempotency_key en mensaje, no header HTTP.

| RPC | Request | Respuesta | Servicio / permiso |
| --- | --- | --- | --- |
| CreateNote | title, subject_id?, visibility, content?, idempotency_key | note_id | Create; autor JWT |
| GetNote | id | note | Get; lectura autorizada |
| ListMyNotes | page {page_size,page_token} | notes[], page {next_page_token} | ListMy; propias |
| UpdateNote | id, title?, visibility?, content?, expected_version, idempotency_key | note | UpdateWithExpectedVersion; autor |
| DeleteNote | id | vacío | Delete; autor |
| ShareNote | note_id, group_id, access_mode | shared_note_id | Share; autor y miembro |
| UnshareNote | shared_note_id | vacío | Unshare; autor o admin |
| ListGroupNotes | group_id, page | notes[] NoteWithShare, page | ListGroupNotePairs; miembro |
| GetNoteAccess | id | access | GetAccess; lectura autorizada |
| UnshareAll | user_id, group_id | vacío | UnshareAll; servicio interno |

Opcionales preservan presencia proto3. expected_version > 0 exige versión
vigente; 0 usa la versión bajo lock. page_size fuera de 1–100 usa default 20 en
PG; page_token es opaco. El límite JSON HTTP no limita el envelope gRPC.

## Res

Note: id, user_id, subject_id?, title, external_file_id?, visibility, likes_count,
forked_from_note_id?, sync_status, version, created_at, updated_at, content?,
drive_url?. Timestamps RFC3339 UTC. GetNote trae contenido, **no attachments**;
para adjuntos se usa HTTP. Int64 sigue serialización protobuf.

NoteWithShare contiene note y share. SharedNote: id, note_id, group_id,
is_admin_note, access_mode, author_followers_snapshot, permission_sync_status,
shared_at. HTTP ListGroupNotes solo devuelve metadata de nota.
NoteAccess: can_read, can_write, visibility, access_mode, drive_sync_status,
drive_access_verified, drive_connection_required, drive_url?.

| Código | Causa |
| --- | --- |
| UNAUTHENTICATED | JWT/clave ausente, inválida o expirada |
| NOT_FOUND | Ausente/inaccesible/origen ilegible |
| PERMISSION_DENIED | Permiso u OAuth |
| INVALID_ARGUMENT | Validación/tamaño/MIME |
| ABORTED | conflict: versión o idempotencia |
| ALREADY_EXISTS | already_saved/already_liked en mapeo común |
| RESOURCE_EXHAUSTED | rate_limited en mapeo común |
| UNAVAILABLE | Drive no disponible |
| INTERNAL | Persistencia/Social/error no tipado |

Status transmite mensaje humano sanitizado; no hay details estructurados ni
code snake_case garantizado en el mensaje. La autoridad es errors.go, aunque
comentarios históricos del proto sugirieran lo contrario. Save/Like/Copy no son
RPC declarados; los códigos anteriores incluyen el catálogo compartido.

## Flujo

Logging → auth por método → NotesServer → NoteService → stores/Drive.
Mismos locks, idempotencia, ACL deseado y outbox que HTTP. Social consume
UnshareAll al salir/expulsar; la clave interna no reemplaza JWT en otros RPC.

Ejemplo grpcurl local (proporcionar proto: servidor sin reflexión):

```sh
grpcurl -plaintext -import-path back/proto/notes -proto v1/notes.proto \
  -H "authorization: Bearer $TOKEN" \
  -d '{"title":"Álgebra","visibility":"private","content":"# Matrices","idempotencyKey":"create-demo-1"}' \
  localhost:9082 notes.v1.NotesService/CreateNote
# {"noteId":"<uuid>"}
grpcurl -plaintext -import-path back/proto/notes -proto v1/notes.proto \
  -H "x-internal-key: $INTERNAL_TEST_KEY" \
  -d '{"userId":"<uuid>","groupId":"<uuid>"}' \
  localhost:9082 notes.v1.NotesService/UnshareAll
# {}
```

9082 host corresponde a Compose normal; E2E conserva gRPC en red interna.

## Tests

- internal/handler/grpc/server_test.go: bufconn, JWT/clave, CRUD, compartir,
  permisos, versión, aislamiento y paginación.
- internal/interceptor/*test.go: credenciales y enrutamiento.
- test/e2e_social_notes_test.go: 6 escenarios Notes + Social fake gRPC y Drive
  memoria. No es una prueba contra Google ni PG real.
- back/social/test/cross_service_e2e_test.go (-tags=crossservice): procesos
  Auth/Notes/Social y PostgreSQL real aislado; proveedores externos mock.
- [Matriz](../documentation/notes_api_matrix.md) e informe de cierre: ejecución
  exacta y alcance, incluidos tests añadidos en esta revisión.
