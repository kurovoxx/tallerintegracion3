# Contrato de API — Sprint Notes + Social

Convenciones: todo endpoint requiere `Authorization: Bearer {access_token}` salvo que se
indique lo contrario. Errores de autenticación/autorización comunes a todos los endpoints
(401 sin token o inválido, 403 sin permiso) se omiten fila por fila y se asumen implícitos.

## Convenciones de respuesta (oficial — sin excepciones)

- **Éxito**: el recurso se devuelve directo en el body, sin envoltorio.
  Ej: `{"access_token": "...", "expires_in": 900}`, nunca `{"data": {...}}`.
- **Listas acotadas** (no necesitan escalar: miembros de un grupo, tareas de un tablero):
  `{"items": [...]}`, sin `next_cursor`.
- **Listas con paginación por cursor** (diseñadas para escalar: apuntes propios, apuntes
  de un grupo): `{"items": [...], "next_cursor": "..."}`.
  La clave es siempre `items` en ambos casos, sin importar qué se esté listando.
- **Error**: siempre `{"error": {"code": "...", "message": "..."}}`.
  Nunca `{"message": ..., "data": ...}` ni variantes.
- **Nombres de campo**: coinciden exactamente con el nombre de columna en la base de datos
  — **`assignee_user_id`, nunca `assigned_to`**, en tablas, en bodies de request, y en
  cualquier ejemplo de payload (incluida la sección 6). Esta regla no tiene excepciones.

---

## 1. Identity — Auth core (Miguel)

| Método | Ruta | Body | Respuesta 2xx | Errores propios |
|---|---|---|---|---|
| POST | `/auth/register` | `{email, password}` | `201 {user_id}` | `409` email ya existe |
| POST | `/auth/login` | `{email, password}` | `200 {access_token, refresh_token, expires_in}` | `401` credenciales inválidas |

*Nota de alcance: las tareas de Academic (simulación de notas, notas requisito) no se
incluyen en este contrato — Academic no forma parte de este sprint.*

## 2. Identity — Sesión, perfil y Drive (Agustín)

| Método | Ruta | Body | Respuesta 2xx | Errores propios |
|---|---|---|---|---|
| POST | `/auth/refresh` | `{refresh_token}` | `200 {access_token, expires_in}` | `401` revocado/expirado/inexistente |
| POST | `/auth/logout` | `{refresh_token}` | `204` | — (idempotente si ya estaba revocado) |
| GET | `/profile/me` | — | `200 {display_name, photo_url, phone, institution, description, visibility}` | — |
| PATCH | `/profile/me` | campos parciales del perfil | `200` perfil actualizado | `400` `visibility` inválida |
| POST | `/auth/google-drive/connect` | `{oauth_code}` | `200 {connected: true}` | `400` código inválido/expirado |
| POST | `/auth/google-calendar/connect` | `{oauth_code}` | `200 {connected: true}` | `400` código inválido/expirado |
| GET | `/notes/{id}/access` | — | `200 {access_mode, can_read: bool, drive_url}` | `403` sin permiso; `404` nota o vínculo inexistente |

**Lógica interna, no expuesta como endpoint**: refresco de `access_token` de Drive al
vencer; marcar `revoked_at` al detectar 401/403 de Google; eliminar nota + `shared_notes`
si su autor ya no existe en el sistema.

## 3. Notes — Núcleo de apuntes (Héctor)

| Método | Ruta | Body | Respuesta 2xx | Errores propios |
|---|---|---|---|---|
| POST | `/notes` | `{title, subject_id?, visibility}` | `201 {note_id}` — crea metadata y dispara creación async del archivo en Drive | `400` visibilidad inválida |
| PATCH | `/notes/{id}` | `{title?, visibility?}` (el contenido se edita directo en Drive) | `200` | `403` no eres el autor |
| DELETE | `/notes/{id}` | — | `204` — borra metadata y archivo en Drive | `403` no eres el autor |
| GET | `/notes/{id}` | — | `200 {title, content, likes_count, ...}` | `404`/`410` no disponible |
| GET | `/notes/me` | query: `cursor`, `limit` | `200 {items: [...], next_cursor}` | — |
| POST | `/notes/{id}/attachments` | `{file}` o `{external_file_id}` | `201 {attachment_id, is_inline}` | `413` archivo muy grande |
| DELETE | `/notes/{id}/attachments/{attachmentId}` | — | `204` | `403` no eres el autor |
| POST | `/notes/{id}/save` | — | `201` | `409` ya guardado |
| POST | `/notes/{id}/copy` | — | `201 {note_id}` (`forked_from_note_id` al original) | `404` original no disponible |
| POST | `/notes/{id}/like` | — | `201` | `409` ya likeado |
| DELETE | `/notes/{id}/like` | — | `204` | — |
| POST | `/notes/{id}/share` | `{group_id, access_mode}` | `201` — resuelve `is_admin_note` vía gRPC a Social | `403` no eres miembro del grupo |
| DELETE | `/notes/shared/{sharedNoteId}` | — | `204` | `403` no eres autor ni admin del grupo |
| POST | `/notes/unshare-all` *(interno, gRPC desde Social)* | `{user_id, group_id}` | `204` | — |
| GET | `/groups/{id}/notes` *(implementado por Notes service, ver nota de propiedad)* | query: `cursor`, `limit` | `200 {items: [...], next_cursor}` — orden: admin primero, luego likes, seguidores, fecha | `403` no eres miembro |

**Nota de propiedad de servicio**: aunque la ruta queda anidada bajo `/groups/{id}/...`,
las tablas consultadas (`shared_notes`, `notes`) pertenecen al schema `notes` — la lógica
vive en el código de **Notes service**, no en Social. "Quién lo codea este sprint" y "a qué
servicio pertenece" son ejes independientes.

## 4. Social — Núcleo de grupos (Benjamín)

| Método | Ruta | Body | Respuesta 2xx | Errores propios |
|---|---|---|---|---|
| POST | `/groups` | `{name, description?}` | `201 {group_id}` — creador queda `admin`; dispara creación de canal Stream | — |
| GET | `/groups/{id}` | — | `200` datos del grupo + `role` del solicitante; `invite_token` solo si eres admin | `403` no eres miembro; `404` |
| GET | `/groups/me` | — | `200 {items: [{group_id, name, role}]}` | — |
| POST | `/groups/{id}/join` | `{invite_token}` | `201` — crea membresía `member` | `403` baneado (con cualquier token); `404` token inválido |
| POST | `/groups/{id}/invite/regenerate` | — | `200 {new_invite_token}` | `403` no eres admin |
| POST | `/groups/{id}/members/{userId}/kick` | — | `204` | `403` no eres admin; `400` objetivo es otro admin o eres tú; `404` no es miembro |
| POST | `/groups/{id}/members/{userId}/ban` | — | `204` — igual que kick + fila en `banned_users` | igual que kick |
| PATCH | `/groups/{id}/members/{userId}/role` | `{role}` | `200` | `403` no eres admin |
| POST | `/groups/{id}/transfer-admin` | `{new_admin_user_id}` | `200` | `403` no eres admin |
| GET | `/groups/{id}/members` | — | `200 {items: [{user_id, role, joined_at}]}` | — |
| POST | `/groups/{id}/leave` | `{cleanup_shared_notes: bool}` | `204` — si `cleanup_shared_notes`, llama a `/notes/unshare-all`; si eras el último miembro, el grupo se elimina | `400` eres el único admin y quedan otros miembros, transfiere primero |
| gRPC | `Social.AccountDeletionCleanup` *(interno, invocado por Identity — no expuesto por HTTP)* | `{user_id}` | — | sucesión automática de admin (promueve al miembro más antiguo por grupo donde eras único admin); elimina grupos sin más miembros; limpia tus baneos. Idempotente |

**Nota**: se convirtió de endpoint HTTP a llamada gRPC interna para ser consistente con
`POST /notes/unshare-all` — ningún cliente debería poder alcanzar esta operación
directamente, así que no debe vivir en la superficie pública de la API.

**Aclaración de diseño pendiente**: `account-deletion` está documentado como invocado
"con el JWT del propio usuario" desde el flujo de borrado de cuenta. Como es un paso
interno orquestado por Identity, probablemente convenga que sea una llamada **gRPC
servicio-a-servicio** (Identity → Social) en vez de reenviar el JWT del usuario — evita
depender de que el token siga siendo válido en ese instante. Confirmar antes de implementar.

## 5. Social — Colaboración e integraciones externas (Martín)

| Método | Ruta | Body | Respuesta 2xx | Errores propios |
|---|---|---|---|---|
| POST | `/groups/{id}/todo` | `{title, assignee_user_id?, due_date?}` | `201 {task_id}` | `400` asignado no es miembro |
| GET | `/groups/{id}/todo` | — | `200 {items: [...]}` | — |
| PATCH | `/groups/{id}/todo/{taskId}` | campos parciales, incluye `assignee_user_id` si se reasigna | `200` | `400` nuevo asignado no es miembro |
| DELETE | `/groups/{id}/todo/{taskId}` | — | `204` | — |
| POST | `/groups/{id}/sprint-sheet` | `{title, assignee_user_id, priority, estimated_hours}` | `201 {task_id}` | `400` asignado no es miembro |
| GET | `/groups/{id}/sprint-sheet` | — | `200 {items: [...]}` | — |
| PATCH | `/groups/{id}/sprint-sheet/{taskId}` | campos parciales, incluye `assignee_user_id` si se reasigna | `200` | `400` nuevo asignado no es miembro |
| DELETE | `/groups/{id}/sprint-sheet/{taskId}` | — | `204` | — |
| POST | `/groups/{id}/sprint-sheet/{taskId}/hours` | `{log_date, hours}` | `201` | `409` ya hay horas ese día (usar PATCH) |
| POST | `/groups/{id}/meetings` | `{title, description?, scheduled_at}` | `201 {meeting_id}` — síncrono; dispara Calendar/Discord/Stream en background | — |
| PUT | `/groups/{id}/discord-config` | `{server_name, invite_url, webhook_url?}` | `200` | `403` no eres admin |
| GET | `/groups/{id}/stream-token` | — | `200 {token, channel_id}` | `403` no eres miembro |

**Lógica interna, no expuesta como endpoint**: creación del canal de Stream al crear el
grupo; sincronización con Calendar y notificación a Discord/Stream al agendar reunión
(asíncronas, best-effort — un fallo ahí no revierte la reunión ya creada).

## 6. Social — Payloads de vistas (Benjamín)

Endpoints de solo lectura que agregan datos para que cada pantalla de Flutter necesite una
sola llamada. Social solo agrega datos de grupos: el perfil vive en Identity
(`GET /profile/me`) y los apuntes en Notes (`GET /notes/me`, `GET /groups/{id}/notes`), que
el cliente sigue consultando directo. Las listas vacías se serializan como `[]`, nunca
`null`.

| Método | Ruta | Respuesta 2xx | Errores propios |
|---|---|---|---|
| GET | `/me/overview` | `200` vista principal: grupos del usuario y barra lateral | — |
| GET | `/groups/{id}/workspace` | `200` sprint sheet, Kanban, próximas reuniones y datos de chat del grupo | `403` no eres miembro; `404` grupo inexistente |

### `GET /me/overview`

```json
{
  "user_id": "aaaaaaaa-0000-4000-8000-000000000001",
  "sidebar": { "groups": [ { "group_id": "…", "name": "Taller de Integración III", "role": "admin" } ] },
  "groups": [
    {
      "group_id": "…", "name": "Taller de Integración III", "description": "…",
      "role": "admin", "member_count": 5, "joined_at": "2026-08-11T12:00:00Z"
    }
  ],
  "stats": { "groups_count": 1, "admin_groups_count": 1 }
}
```

Orden de `groups`/`sidebar.groups`: grupo más reciente primero. `description` se omite si
es nula.

### `GET /groups/{id}/workspace`

```json
{
  "group": { "id": "…", "name": "…", "description": "…", "role": "member" },
  "kanban": {
    "todo": [ { "id": "…", "group_id": "…", "board_id": "…", "board_name": "General", "title": "…", "status": "todo", "assignee_user_id": null, "due_date": null, "created_at": "…", "updated_at": "…" } ],
    "in_progress": [],
    "done": []
  },
  "sprint_sheet": {
    "sheets": [ { "id": "…", "name": "Sprint 1", "period_start": null, "period_end": null } ],
    "tasks": [ { "id": "…", "group_id": "…", "sheet_id": "…", "sheet_name": "Sprint 1", "title": "…", "assignee_user_id": "…", "priority": "alta", "status": "en_proceso", "estimated_hours": 2.5, "created_at": "…", "updated_at": "…" } ]
  },
  "meetings": { "upcoming": [ { "id": "…", "title": "Daily", "description": "…", "scheduled_at": "2026-09-30T15:00:00Z" } ] },
  "chat": { "provider": "stream", "token_endpoint": "/groups/{id}/stream-token" }
}
```

- `kanban.*` y `sprint_sheet.tasks` son los mismos objetos que devuelven
  `GET /groups/{id}/todo` y `GET /groups/{id}/sprint-sheet` — **con `assignee_user_id`**,
  no `assigned_to`.
- Horas diarias no incluidas aquí (serían N consultas): se piden por tarea con
  `GET /groups/{id}/sprint-sheet/{taskId}/hours`.
- `meetings.upcoming` solo reuniones con `scheduled_at` futuro, orden cronológico.
- `chat.token_endpoint` apunta al endpoint de §5 que emite el token — el payload nunca
  embebe el token de Stream directamente.

---

## Aclaraciones pendientes de tu parte

1. **Resuelto**: `google-drive/connect` y `google-calendar/connect` son flujos de OAuth
   completamente independientes, cada uno con su propio consentimiento.
2. **Resuelto**: el token de Stream se emite desde Social (no Identity) — Social ya
   conoce el `channel_id` del grupo localmente; moverlo a Identity añadiría una llamada
   gRPC extra a Social en cada conexión al chat sin ningún beneficio a cambio.
3. **Resuelto**: la limpieza de sucesión de admin al borrar cuenta pasa a ser una llamada
   gRPC interna (`Social.AccountDeletionCleanup`), no un endpoint HTTP — por consistencia
   con cómo ya tratamos `POST /notes/unshare-all`.

Sin aclaraciones pendientes por ahora.