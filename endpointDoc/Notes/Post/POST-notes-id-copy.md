# POST /notes/:id/copy

Microservicio `notes`; ruta del router de producción.

## Req

- JWT HS256 en `Authorization: Bearer <access_token>`; identidad extraída del token.
- Permiso: Lectura origen y Drive destino conectado.
- Entrada: Path id UUID; X-Idempotency-Key opcional.

## Res

- 201 {note_id,forked_from_note_id}.
- 404 sin acceso/origen no disponible; 403 OAuth destino; 409 clave; 429 cuota; 502 Drive; 500 DB.
- `401` por token ausente/inválido/expirado, antes del handler. Errores con forma `{"error":{"code":"not_found","message":"Nota no encontrada"}}`; ver [catálogo común](../documentation/docHector.md#autenticación-y-errores).

Ejemplo de respuesta (IDs y fechas ilustrativos, campos del contrato real):

```json
{"note_id":"44444444-4444-4444-8444-444444444444","forked_from_note_id":"11111111-1111-4111-8111-111111111111"}
```

## Flujo

`Copy` → `Copy`. Clona contenido a Drive del solicitante y registra forked_from_note_id. La autorización origen no depende de que el lector tenga OAuth del autor. Idempotencia durable por usuario/operación/clave y hash del origen. Límite por proceso: 10 copias/minuto/usuario y 100/minuto global; aplica también a replays que lleguen al handler.

Fuentes: `back/notes/cmd/server/main.go`, `internal/handler/http/` y `internal/service/` dentro de `back/notes`.

## Tests

- Handler/router: `TestHandlerCopyIdempotencyReplayAndConflict; TestHandlerCopyRateLimitReturns429`.
- Dominio: `TestIdempotencyCopyCrashRecoveryReusesPreassignedClone`.
- Alcance de integración y E2E: [matriz de cobertura](../documentation/notes_api_matrix.md).
