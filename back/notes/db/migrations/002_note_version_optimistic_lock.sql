-- Apply to an existing database initialized with docker/postgres/init.sql.
-- Keep this migration aligned with the NOTES SERVICE section of that file.
BEGIN;

-- version es el contador de versionado optimista de notes.notes: cada UPDATE de
-- metadata (title/visibility) exige la versión leída por el cliente y la
-- incrementa en la misma sentencia, de modo que dos writers concurrentes no
-- puedan pisarse silenciosamente (el que llega tarde recibe conflict/409).
ALTER TABLE notes.notes
    ADD COLUMN IF NOT EXISTS version bigint NOT NULL DEFAULT 1;

COMMIT;
