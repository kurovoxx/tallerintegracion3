# GET /health

Microservicio `notes`; ruta del router de producción.

## Req

- Sin autenticación.
- Permiso: Pública.
- Entrada: Sin body.

## Res

- 200 {status:"UP",service:"notes-service"}.
- Sin errores de dominio.

Ejemplo de respuesta (IDs y fechas ilustrativos, campos del contrato real):

```json
{"status":"UP","service":"notes-service"}
```

## Flujo

`inline newHTTPRouter` → `Ninguno`. Liveness HTTP. No verifica PostgreSQL, Identity, Social ni disponibilidad real de Drive. Comparte path con otros servicios; seleccionar la base de Notes.

Fuentes: `back/notes/cmd/server/main.go`, `internal/handler/http/` y `internal/service/` dentro de `back/notes`.

## Tests

- Handler/router: `TestHTTPRouterContract`.
- Dominio: `No aplica`.
- Alcance de integración y E2E: [matriz de cobertura](../documentation/notes_api_matrix.md).
