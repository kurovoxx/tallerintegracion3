-- Permisos "para todo el grupo": el admin puede habilitar de una vez que todos
-- los miembros compartan apuntes, agenden reuniones o editen el kanban/sprint.
-- El permiso efectivo de un miembro es (permiso propio OR permiso del grupo).
-- Por defecto quedan en false: solo los admins (y los miembros delegados) pueden.
ALTER TABLE social.groups
  ADD COLUMN IF NOT EXISTS members_can_share_notes boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS members_can_create_meetings boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS members_can_edit_kanban boolean NOT NULL DEFAULT false;
