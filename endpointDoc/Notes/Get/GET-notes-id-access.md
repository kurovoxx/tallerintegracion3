# GET /notes/:id/access

Microservicio `notes`; ruta del router de producción.

## Req

- JWT HS256 en `Authorization: Bearer <access_token>`; identidad extraída del token.
- Permiso: Autor, pública, link o miembro restricted.
- Entrada: Path id UUID.

## Res

- 200 NoteAccessResponse.
- 404 inaccesible/ausente/sin archivo; 500 DB/Social.
- `401` por token ausente/inválido/expirado, antes del handler. Errores con forma `{"error":{"code":"not_found","message":"Nota no encontrada"}}`; ver [catálogo común](../documentation/docHector.md#autenticación-y-errores).

Ejemplo de respuesta (IDs y fechas ilustrativos, campos del contrato real):

```json
{"can_read":true,"can_write":true,"visibility":"private","access_mode":"owner","drive_sync_status":"synced","drive_access_verified":true,"drive_connection_required":false,"drive_url":"https://drive.google.com/file/d/drive-file-id/view"}
```

## Flujo

`GetAccess` → `GetAccess`. can_write solo para autor. drive_sync_status agrega ACL administrado: failed > pending > synced. Solo owner de nota no pública verifica OAuth/acceso remoto; OAuth faltante se comunica con drive_connection_required, no 403. public/link/restricted no verifican OAuth del solicitante. No descarga contenido.

Fuentes: `back/notes/cmd/server/main.go`, `internal/handler/http/` y `internal/service/` dentro de `back/notes`.

## Tests

- Handler/router: `TestAccessJSONContract; TestAccessJSONContractDriveConnectionRequired`.
- Dominio: `GetAccess ejercitado por access_contract_test.go`.
- Alcance de integración y E2E: [matriz de cobertura](../documentation/notes_api_matrix.md).
