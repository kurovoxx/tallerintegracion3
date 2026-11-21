-- Relación de "seguir" entre usuarios de identity.
-- follower_id sigue a following_id. Un usuario no puede seguirse a sí mismo.
-- Se usa para saber a quién sigue cada usuario (timeline, notificaciones, etc.).
CREATE TABLE IF NOT EXISTS identity.user_follows (
    follower_id uuid not null,
    following_id uuid not null,
    created_at timestamp with time zone not null default now(),
    constraint user_follows_pkey primary key (follower_id, following_id),
    constraint user_follows_follower_fkey foreign key (follower_id) references identity.users (id) on delete CASCADE,
    constraint user_follows_following_fkey foreign key (following_id) references identity.users (id) on delete CASCADE,
    constraint user_follows_no_self_check check (follower_id <> following_id)
);

CREATE INDEX IF NOT EXISTS idx_user_follows_following ON identity.user_follows USING btree (following_id);
CREATE INDEX IF NOT EXISTS idx_user_follows_follower ON identity.user_follows USING btree (follower_id);
