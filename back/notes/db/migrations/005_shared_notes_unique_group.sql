-- Un apunte solo puede compartirse una vez en cada grupo.
--
-- POST /notes/{id}/share sobre un (note_id, group_id) ya vinculado responde
-- 409 already_shared (también con otro access_mode: para cambiarlo se retira
-- el share y se vuelve a compartir). El servicio lo comprueba bajo el lock por
-- nota y la BD lo garantiza con este UNIQUE.
--
-- Idempotente. Mantener alineado con docker/postgres/init.sql.
BEGIN;

-- 1. Instalaciones previas pueden tener duplicados: se conserva el vínculo
--    más antiguo de cada (note_id, group_id) y las notas afectadas quedan
--    'pending' para que el reconciliador vuelva a converger el ACL de Drive.
UPDATE notes.shared_notes
SET permission_sync_status = 'pending'
WHERE note_id IN (
    SELECT note_id FROM notes.shared_notes
    GROUP BY note_id, group_id HAVING count(*) > 1
);

DELETE FROM notes.shared_notes s
USING (
    SELECT id,
           row_number() OVER (PARTITION BY note_id, group_id ORDER BY shared_at, id) AS rn
    FROM notes.shared_notes
) ranked
WHERE s.id = ranked.id AND ranked.rn > 1;

-- 2. Restricción única.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'notes.shared_notes'::regclass
          AND conname = 'shared_notes_note_id_group_id_key'
    ) THEN
        ALTER TABLE notes.shared_notes
            ADD CONSTRAINT shared_notes_note_id_group_id_key UNIQUE (note_id, group_id);
    END IF;
END $$;

COMMIT;
