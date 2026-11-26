-- Exclusión individual de permisos "para todo el grupo": un admin puede quitar
-- a un miembro concreto un permiso que el switch del grupo habilitó para todos.
-- El permiso efectivo pasa a ser (propio OR grupo) AND NOT denegado.
ALTER TABLE social.group_memberships
  ADD COLUMN IF NOT EXISTS deny_share_notes boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS deny_create_meetings boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS deny_edit_kanban boolean NOT NULL DEFAULT false;
