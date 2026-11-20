# GET | PATCH /groups/{id}/permissions

Permisos "para todo el grupo": un admin habilita (switch) que **todos** los miembros puedan compartir notas, agendar reuniones o modificar el kanban/sprint. El permiso efectivo de un miembro es `permiso propio (PATCH /members/{user_id}/permissions) OR permiso del grupo`. Los admins pueden todo siempre; por defecto todo está apagado y los miembros nuevos heredan el estado del grupo.

| Permiso | Habilita |
|---|---|
| `can_share_notes` | compartir notas en el grupo (Notes lee `GET /internal/groups/{id}/members`, ya con el permiso del grupo aplicado) |
| `can_create_meetings` | `POST /groups/{id}/meetings` |
| `can_edit_kanban` | crear/editar/borrar tareas de la hoja de sprint |

- `GET`: cualquier miembro. `200 {can_share_notes, can_create_meetings, can_edit_kanban}`. `403` no miembro, `404` grupo inexistente.
- `PATCH`: solo admin. Body parcial (al menos un campo): `{ "can_share_notes"?: bool, "can_create_meetings"?: bool, "can_edit_kanban"?: bool }`. `200` con el estado resultante. `400` sin campos, `403` no admin.
- Front: switch `GroupPermissionSwitch` (solo visible para admins) en las pestañas Notas, Reuniones, Hoja Sprint y Kanban. Sprint y Kanban comparten `can_edit_kanban`; hoy el backend solo lo exige en la hoja de sprint (el kanban de tareas sigue abierto a todos los miembros).
- Migración: `back/social/db/migrations/007_group_member_defaults.sql` (columnas `members_can_*` en `social.groups`; hay que aplicarla a la BD desplegada).
