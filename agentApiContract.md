# Contrato de API — Sprint Notes + Social

Convenciones: todo endpoint requiere `Authorization: Bearer {access_token}` salvo que se
indique lo contrario. Errores de autenticación/autorización comunes a todos los endpoints
(401 sin token o inválido, 403 sin permiso) se omiten fila por fila y se asumen implícitos.

---

## 1. Identity — Auth core (Miguel)

| Método | Ruta | Body | Respuesta 2xx | Errores propios |
|---|---|---|---|---|
| POST | `/auth/register` | `{email, password}` | `201 {user_id}` | `409` email ya existe |
| POST | `/auth/login` | `{email, password}` | `200 {access_token, refresh_token, expires_in}` | `401` credenciales inválidas |

*Nota de alcance: las tareas de Academic (simulación de notas, notas requisito) que
figuraban antes en la lista de Miguel no se incluyen en este contrato — Academic no forma
parte de este sprint. Si siguen vigentes para otro momento, avisa y se documentan aparte.*

## 2. Identity — Sesión, perfil y Drive (Agustín)

| Método | Ruta | Body | Respuesta 2xx | Errores propios |
|---|---|---|---|---|
| POST | `/auth/refresh` | `{refresh_token}` | `200 {access_token, expires_in}` | `401` revocado/expirado/inexistente |
| POST | `/auth/logout` | `{refresh_token}` | `204` | — (idempotente si ya estaba revocado) |
| GET | `/profile/me` | — | `200 {display_name, photo_url, phone, institution, description, visibility}` | — |
| PATCH | `/profile/me` | campos parciales del perfil | `200` perfil actualizado | `400` `visibility` inválida |
| POST | `/auth/google-drive/connect` | `{oauth_code}` (código de redirect de Google) | `200 {connected: true}` | `400` código inválido/expirado |
| POST | `/auth/google-calendar/connect` *(añadido para completar el contrato)* | `{oauth_code}` | `200 {connected: true}` | `400` código inválido/expirado |
| GET | `/notes/{id}/access` | — | `200 {access_mode, can_read: bool, drive_url}` | `403` sin permiso; `404` nota o vínculo inexistente |

**Notas de implementación no expuestas como endpoint** (lógica interna, no HTTP público):
refresco de `access_token` de Drive cuando `expires_at` vence; marcar `revoked_at` al
detectar un 401/403 de la API de Google; al fallar la lectura de un archivo por autor
inexistente, eliminar la nota y sus `shared_notes`.

## 3. Notes — Núcleo de apuntes (Héctor)

| Método | Ruta | Body | Respuesta 2xx | Errores propios |
|---|---|---|---|---|
| POST | `/notes` | `{title, subject_id?, visibility}` | `201 {note_id}` — crea metadata y dispara creación async del archivo en Drive | `400` visibilidad inválida |
| PATCH | `/notes/{id}` | `{title?, visibility?}` (el contenido se edita directo en Drive, no vía este endpoint) | `200` | `403` no eres el autor |
| DELETE | `/notes/{id}` | — | `204` — borra metadata y el archivo en Drive | `403` no eres el autor |
| GET | `/notes/{id}` | — | `200 {title, content (descargado de Drive), likes_count, ...}` | `404`/`410` no disponible (403/404 de Drive) |
| GET | `/notes/me` | query: `cursor`, `limit` | `200 {notes: [...], next_cursor}` | — |
| POST | `/notes/{id}/attachments` | `{file}` (multipart) o `{external_file_id}` si ya se subió directo a Drive desde el cliente | `201 {attachment_id, is_inline}` | `413` archivo muy grande |
| DELETE | `/notes/{id}/attachments/{attachmentId}` | — | `204` | `403` no eres el autor |
| POST | `/notes/{id}/save` | — | `201` | `409` ya guardado |
| POST | `/notes/{id}/copy` | — | `201 {note_id}` (nuevo, `forked_from_note_id` apuntando al original) | `404` original no disponible |
| POST | `/notes/{id}/like` | — | `201` | `409` ya likeado |
| DELETE | `/notes/{id}/like` | — | `204` | — |
| POST | `/notes/{id}/share` | `{group_id, access_mode}` | `201` — resuelve `is_admin_note` vía gRPC a Social | `403` no eres miembro del grupo |
| DELETE | `/notes/shared/{sharedNoteId}` | — | `204` | `403` no eres autor ni Staff del grupo |
| POST | `/notes/unshare-all` *(interno, llamado por Social)* | `{user_id, group_id}` | `204` | — |

## 4. Social — Núcleo de grupos (Benjamín)

| Método | Ruta | Body | Respuesta 2xx | Errores propios |
|---|---|---|---|---|
| POST | `/groups` | `{name, description?}` | `201 {group_id}` — creador queda como `admin`; dispara creación de canal Stream | — |
| GET | `/groups/{id}` | — | `200` datos del grupo | `404` |
| GET | `/groups/me` | — | `200 [{group_id, name, role}]` | — |
| POST | `/groups/{id}/join` | `{invite_token}` | `201` — crea membresía como `member` | `403` baneado; `404` token inválido |
| POST | `/groups/{id}/invite/regenerate` | — | `200 {new_invite_token}` | `403` no eres admin |
| POST | `/groups/{id}/members/{userId}/kick` | — | `204` | `403` no eres admin; `400` no puedes expulsar a otro admin |
| POST | `/groups/{id}/members/{userId}/ban` | — | `204` — igual que kick + fila en `banned_users` | `403`/`400` igual que kick |
| PATCH | `/groups/{id}/members/{userId}/role` | `{role}` | `200` | `403` no eres admin |
| POST | `/groups/{id}/transfer-admin` | `{new_admin_user_id}` | `200` | `403` no eres admin |
| GET | `/groups/{id}/members` | — | `200 [{user_id, role, joined_at}]` | — |
| POST | `/groups/{id}/leave` | `{cleanup_shared_notes: bool}` | `204` — si `cleanup_shared_notes`, llama a `/notes/unshare-all` | `400` eres el único admin, transfiere primero |
| GET | `/groups/{id}/notes` | query: `cursor`, `limit` | `200 {notes: [...], next_cursor}` — orden: admin primero, luego likes, seguidores, fecha | `403` no eres miembro |

## 5. Social — Colaboración e integraciones externas (Martín)

| Método | Ruta | Body | Respuesta 2xx | Errores propios |
|---|---|---|---|---|
| POST | `/groups/{id}/todo` | `{title, assignee_user_id?, due_date?}` | `201 {task_id}` | `400` asignado no es miembro |
| GET | `/groups/{id}/todo` | — | `200 [...]` | — |
| PATCH | `/groups/{id}/todo/{taskId}` | campos parciales | `200` | — |
| DELETE | `/groups/{id}/todo/{taskId}` | — | `204` | — |
| POST | `/groups/{id}/sprint-sheet` | `{title, assignee_user_id, priority, estimated_hours}` | `201 {task_id}` | `400` asignado no es miembro |
| GET | `/groups/{id}/sprint-sheet` | — | `200 [...]` | — |
| PATCH | `/groups/{id}/sprint-sheet/{taskId}` | campos parciales | `200` | — |
| DELETE | `/groups/{id}/sprint-sheet/{taskId}` | — | `204` | — |
| POST | `/groups/{id}/sprint-sheet/{taskId}/hours` | `{log_date, hours}` | `201` | `409` ya hay horas registradas ese día (usar PATCH) |
| POST | `/groups/{id}/meetings` | `{title, description?, scheduled_at}` | `201 {meeting_id}` — creación local síncrona; dispara Calendar/Discord/Stream en background | — |
| PUT | `/groups/{id}/discord-config` | `{server_name, invite_url, webhook_url?}` | `200` | `403` no eres admin |
| GET | `/groups/{id}/stream-token` *(añadido para completar el contrato)* | — | `200 {token, channel_id}` — token firmado con la API secret de Stream para que el cliente se conecte directo | `403` no eres miembro |

**Notas de implementación no expuestas como endpoint**: creación del canal de Stream
(disparada internamente al crear el grupo, no por el cliente); sincronización con Google
Calendar y notificación a Discord/Stream al agendar una reunión (asíncronas, best-effort —
un fallo ahí no revierte la creación de la reunión, ya persistida).

---

## Aclaraciones pendientes de tu parte

1. ¿El endpoint de conectar Google Calendar reutiliza el mismo flujo de consentimiento que
   Drive (pidiendo ambos scopes de una sola vez) o son dos conexiones completamente
   independientes como quedaron documentadas arriba?
2. El token de Stream: ¿lo emite el mismo servicio que gestiona los grupos (Martín), o
   debería vivir en Identity junto al resto de la emisión de tokens? Lo dejé en Social por
   cercanía con el resto de la integración, pero es una decisión de ustedes.