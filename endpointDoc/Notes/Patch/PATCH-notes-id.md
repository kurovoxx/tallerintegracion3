# PATCH /notes/:id

Microservicio `notes`; ruta del router de producción.

## Req

- JWT HS256 en `Authorization: Bearer <access_token>`; identidad extraída del token.
- Permiso: Solo autor.
- Entrada: JSON title?, visibility?, content?, version?; X-Idempotency-Key opcional.

```json
{"title":"Matrices revisadas","content":"# Versión corregida\n","version":1}
```

## Res

- 200 {id,title,visibility,version,updated_at}.
- 400 vacío/validación; 404 ajena/ausente; 409 versión/clave; 413 tamaño; 403 OAuth; 502 Drive; 500 DB.
- `401` por token ausente/inválido/expirado, antes del handler. Errores con forma `{"error":{"code":"not_found","message":"Nota no encontrada"}}`; ver [catálogo común](../documentation/docHector.md#autenticación-y-errores).

Ejemplo de respuesta (IDs y fechas ilustrativos, campos del contrato real):

```json
{"id":"11111111-1111-4111-8111-111111111111","title":"Matrices revisadas","visibility":"private","version":2,"updated_at":"2026-10-22T12:01:00Z"}
```

## Flujo

`Patch` → `UpdateWithExpectedVersion`. Debe incluir al menos title, visibility o content; version por sí sola no actualiza. version >= 1 es una precondición optimista. Body máximo 1 MiB. Lock por nota, snapshot remoto y compensación/outbox coordinan Drive con PostgreSQL. El replay PATCH usa caché de proceso de 10 minutos; no ofrece la durabilidad entre reinicios de Create/Copy.

Fuentes: `back/notes/cmd/server/main.go`, `internal/handler/http/` y `internal/service/` dentro de `back/notes`.

## Tests

- Handler/router: `TestHandlerPatchVersionConflict; TestHandlerPatchIdempotencyReplay`.
- Dominio: `TestPermissionConcurrentUpdatesSingleWinner; TestUpdateIdempotencyReplayAndErrorCleanup`.
- Alcance de integración y E2E: [matriz de cobertura](../documentation/notes_api_matrix.md).
