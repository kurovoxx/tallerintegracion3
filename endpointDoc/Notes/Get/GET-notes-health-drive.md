# GET /health/drive

Microservicio `notes`; ruta del router de producción.

## Req

- Sin autenticación.
- Permiso: Pública.
- Entrada: Sin body.

## Res

- 200 {storage_mode}.
- Sin errores de dominio.

Ejemplo de respuesta (IDs y fechas ilustrativos, campos del contrato real):

```json
{"storage_mode":"mock"}
```

## Flujo

`inline newHTTPRouter` → `Ninguno`. Expone el modo configurado, no realiza un probe OAuth/Drive ni certifica conectividad. Con DATABASE_URL vacío el servidor fuerza el cliente mock incluso si STORAGE_MODE declara drive; interpretar este campo como configuración, no prueba de almacenamiento real.

Fuentes: `back/notes/cmd/server/main.go`, `internal/handler/http/` y `internal/service/` dentro de `back/notes`.

## Tests

- Handler/router: `TestHTTPRouterContract`.
- Dominio: `No aplica`.
- Alcance de integración y E2E: [matriz de cobertura](../documentation/notes_api_matrix.md).
