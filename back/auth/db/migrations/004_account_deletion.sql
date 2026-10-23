-- Durable account-deletion state is deliberately not FK-linked to users:
-- completed tombstones keep already issued JWTs invalid after local deletion.
CREATE TABLE IF NOT EXISTS identity.account_deletions (
    user_id uuid PRIMARY KEY,
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'complete')),
    last_error text,
    started_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_account_deletions_pending
    ON identity.account_deletions (updated_at)
    WHERE status = 'pending';
