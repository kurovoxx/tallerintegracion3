# PATCH /groups/{id}/members/{user_id}/permissions

Un admin delega permisos a un miembro común. Los admins pueden todo siempre; por defecto un miembro no tiene ninguno.

| Permiso | Habilita |
|---|---|
| `can_share_notes` | compartir apuntes en el grupo (Notes consulta `GET /internal/groups/{id}/members`, campo `can_share_notes`) |
| `can_create_meetings` | `POST /groups/{id}/meetings` |
| `can_edit_kanban` | crear/editar/borrar tareas del sprint (kanban) |

- Auth: JWT. Solo admin del grupo.
- Body (parcial, al menos un campo): `{ "can_share_notes"?: bool, "can_create_meetings"?: bool, "can_edit_kanban"?: bool }`.
- `200`: permisos resultantes `{can_share_notes, can_create_meetings, can_edit_kanban}`.
- `400`: body sin permisos, objetivo admin (`target_is_admin`) o uno mismo (`cannot_modify_self`).
- `403`: quien llama no es admin. `404`: grupo o miembro inexistente.
- `GET /groups/{id}/members` incluye los tres booleanos por miembro.
- Migración: `back/social/db/migrations/006_member_permissions.sql` (hay que aplicarla a la BD desplegada).
