-- Invitaciones a grupos: un admin de un grupo puede invitar a un usuario
-- seguido. El invitado recibe una notificación in-app con botones Aceptar /
-- Rechazar. Al aceptar, se une al grupo como member regular.
CREATE TABLE IF NOT EXISTS social.group_invitations (
    id uuid not null default gen_random_uuid (),
    group_id uuid not null,
    invited_user_id uuid not null,
    invited_by_user_id uuid not null,
    status character varying(20) not null default 'pending',
    created_at timestamp with time zone not null default now(),
    responded_at timestamp with time zone null,
    constraint group_invitations_pkey primary key (id),
    constraint group_invitations_group_id_fkey foreign key (group_id) references social.groups (id) on delete CASCADE,
    constraint group_invitations_invited_user_id_fkey foreign key (invited_user_id) references identity.users (id) on delete CASCADE,
    constraint group_invitations_invited_by_user_id_fkey foreign key (invited_by_user_id) references identity.users (id) on delete CASCADE,
    constraint group_invitations_status_check check (status in ('pending', 'accepted', 'rejected'))
);

CREATE UNIQUE INDEX IF NOT EXISTS group_invitations_unique_pending
    ON social.group_invitations (group_id, invited_user_id)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_group_invitations_invited_user_id ON social.group_invitations USING btree (invited_user_id, status);
CREATE INDEX IF NOT EXISTS idx_group_invitations_group_id ON social.group_invitations USING btree (group_id);
CREATE INDEX IF NOT EXISTS idx_group_invitations_invited_by_user_id ON social.group_invitations USING btree (invited_by_user_id);
