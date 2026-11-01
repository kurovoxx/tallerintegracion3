-- Apply to an existing database initialized with docker/postgres/init.sql.
-- Keep this migration aligned with the NOTES SERVICE section of that file.
BEGIN;

-- Logical references deliberately survive note/attachment deletion so that
-- pending Drive cleanup can still be reconciled after local rows are removed.
CREATE TABLE IF NOT EXISTS notes.drive_reconciliation_queue (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    operation text NOT NULL,
    note_id uuid,
    attachment_id uuid,
    external_file_id text,
    owner_user_id uuid NOT NULL,
    payload jsonb NOT NULL DEFAULT '{}',
    status text NOT NULL DEFAULT 'pending',
    attempts integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_error text,
    created_at timestamptz DEFAULT now(),
    updated_at timestamptz DEFAULT now(),
    completed_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_drive_reconciliation_queue_pending
    ON notes.drive_reconciliation_queue (status, next_attempt_at)
    WHERE status IN ('pending', 'failed');

CREATE TABLE IF NOT EXISTS notes.idempotency_keys (
    user_id uuid NOT NULL,
    operation text NOT NULL,
    idempotency_key text NOT NULL,
    resource_id uuid,
    request_hash text NOT NULL,
    status text NOT NULL,
    created_at timestamptz DEFAULT now(),
    expires_at timestamptz NOT NULL,
    UNIQUE (user_id, operation, idempotency_key)
);

CREATE TABLE IF NOT EXISTS notes.drive_managed_permissions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    note_id uuid NOT NULL,
    external_file_id text NOT NULL,
    principal_type text NOT NULL,
    principal_key text NOT NULL,
    drive_permission_id text,
    role text NOT NULL,
    sync_status text NOT NULL,
    created_at timestamptz DEFAULT now(),
    updated_at timestamptz DEFAULT now(),
    UNIQUE (note_id, principal_type, principal_key)
);

-- These constraints are already present in the current initialization schema.
-- Add them for older installations without creating duplicate indexes on reruns.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'notes.note_likes'::regclass
          AND conname = 'note_likes_note_id_user_id_key'
          AND contype = 'u'
    ) THEN
        ALTER TABLE notes.note_likes
            ADD CONSTRAINT note_likes_note_id_user_id_key UNIQUE (note_id, user_id);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'notes.saved_notes'::regclass
          AND conname = 'saved_notes_user_id_note_id_key'
          AND contype = 'u'
    ) THEN
        ALTER TABLE notes.saved_notes
            ADD CONSTRAINT saved_notes_user_id_note_id_key UNIQUE (user_id, note_id);
    END IF;
END $$;

-- permission_sync_status es el estado de convergencia del ACL de Drive por
-- share: 'pending' (intención persistida) | 'in_sync' | 'failed'. Se usa junto
-- con notes.drive_managed_permissions para reconciliación de convergencia.
ALTER TABLE notes.shared_notes
    ADD COLUMN IF NOT EXISTS permission_sync_status text NOT NULL DEFAULT 'pending';

-- Instalaciones previas al desired-state usaban 'synced': normalizar.
UPDATE notes.shared_notes SET permission_sync_status = 'in_sync' WHERE permission_sync_status = 'synced';

ALTER TABLE notes.shared_notes
    ALTER COLUMN permission_sync_status SET DEFAULT 'pending';

COMMIT;
