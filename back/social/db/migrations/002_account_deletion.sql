-- Tombstones outlive membership and group rows, so deleted users cannot use
-- an old JWT or rejoin. Jobs have no FK to groups because a last-member
-- cleanup deletes that group before Notes' Drive permissions necessarily sync.
CREATE TABLE IF NOT EXISTS social.account_deletion_users (
    user_id uuid PRIMARY KEY,
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'complete')),
    started_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);

CREATE TABLE IF NOT EXISTS social.account_deletion_groups (
    user_id uuid NOT NULL,
    group_id uuid NOT NULL,
    membership_removed boolean NOT NULL DEFAULT false,
    group_deleted boolean NOT NULL DEFAULT false,
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'complete')),
    updated_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    PRIMARY KEY (user_id, group_id)
);

CREATE INDEX IF NOT EXISTS idx_account_deletion_groups_pending
    ON social.account_deletion_groups (user_id, group_id)
    WHERE status = 'pending';
