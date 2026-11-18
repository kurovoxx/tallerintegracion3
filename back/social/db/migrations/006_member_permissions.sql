-- Permisos delegables por miembro: el admin puede permitir a miembros comunes
-- compartir apuntes, agendar reuniones y editar el kanban del sprint.
-- Los admins tienen todos los permisos siempre; por defecto un miembro no tiene ninguno.
ALTER TABLE social.group_memberships
  ADD COLUMN IF NOT EXISTS can_share_notes boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS can_create_meetings boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS can_edit_kanban boolean NOT NULL DEFAULT false;
