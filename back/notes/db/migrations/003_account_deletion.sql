-- Notes tombstones block writes using already-issued JWTs. They are retained
-- after cleanup because account-owned note content is intentionally preserved.
CREATE TABLE IF NOT EXISTS notes.account_deletion_users (
    user_id uuid PRIMARY KEY,
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'complete')),
    started_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);

-- Preserve the note IDs that still need Drive ACL confirmation after the
-- corresponding shared_notes rows have been removed. A pending Drive failure
-- can then be retried after a process restart, including for deleted groups.
CREATE TABLE IF NOT EXISTS notes.account_deletion_group_notes (
    user_id uuid NOT NULL,
    group_id uuid NOT NULL,
    note_id uuid NOT NULL,
    group_deleted boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, group_id, note_id)
);

CREATE INDEX IF NOT EXISTS idx_account_deletion_group_notes_group
    ON notes.account_deletion_group_notes (group_id, user_id);

CREATE INDEX IF NOT EXISTS idx_shared_notes_note_id
    ON notes.shared_notes (note_id);
