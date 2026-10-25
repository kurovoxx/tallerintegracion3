# POST /notes

Microservicio `notes`; ruta del router de producción.

## Req

- JWT HS256 en `Authorization: Bearer <access_token>`; identidad extraída del token.
- Permiso: Autor = JWT.
- Entrada: JSON title, visibility, subject_id?, content?; X-Idempotency-Key opcional.

```json
{"title":"Álgebra","visibility":"private","content":"# Matrices\n"}
```

## Res

- 201 {note_id}.
- 400 validación; 403 OAuth; 409 clave/payload; 413 tamaño; 502 Drive.
- `401` por token ausente/inválido/expirado, antes del handler. Errores con forma `{"error":{"code":"not_found","message":"Nota no encontrada"}}`; ver [catálogo común](../documentation/docHector.md#autenticación-y-errores).

Ejemplo de respuesta (IDs y fechas ilustrativos, campos del contrato real):

```json
{"note_id":"11111111-1111-4111-8111-111111111111"}
```

## Flujo

`Create` → `Create`. El body completo tiene límite de 1 MiB. El título efectivo admite 1–255 caracteres tras recortar (el binding HTTP admite hasta 300, el servicio valida 255). subject_id solo se valida como UUID. El Markdown se sanitiza. Se guarda metadata pending_drive, se crea/adopta el archivo y se marca synced; el request espera este flujo. La misma clave y payload reutilizan el resource_id. Rechazos definitivos de Drive limpian la fila; fallos ambiguos conservan el intento recuperable.

Fuentes: `back/notes/cmd/server/main.go`, `internal/handler/http/` y `internal/service/` dentro de `back/notes`.

## Tests

- Handler/router: `TestHandlerCreateIdempotencyReplayAndConflict`.
- Dominio: `TestCreateRetryAfterReplayWindowAdoptsExistingNote; TestCreateConcurrentRetriesSameKeyCreateOneNote`.
- Alcance de integración y E2E: [matriz de cobertura](../documentation/notes_api_matrix.md).
