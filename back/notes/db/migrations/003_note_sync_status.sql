-- Apply to an existing database initialized with docker/postgres/init.sql.
-- Keep this migration aligned with the NOTES SERVICE section of that file.
BEGIN;

-- sync_status es el estado de sincronización de la nota con Drive
-- ('synced' | 'pending_drive' | 'failed_sync'); lo lee la reconciliación
-- periódica de notes (GetPendingSyncNotes). Las notas existentes ya están en Drive.
ALTER TABLE notes.notes
    ADD COLUMN IF NOT EXISTS sync_status varchar(30) NOT NULL DEFAULT 'synced';

COMMIT;
