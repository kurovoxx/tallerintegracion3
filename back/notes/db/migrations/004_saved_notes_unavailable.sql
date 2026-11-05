-- Guardados que sobreviven al borrado del apunte original ("no disponible").
--
-- Antes: notes.saved_notes.note_id tenía FK ON DELETE CASCADE, de modo que si
-- el autor borraba su apunte, la referencia desaparecía en silencio de los
-- guardados de los demás usuarios. Ahora:
--   1. saved_notes.note_id pasa a ser una referencia lógica (sin FK): la fila
--      sobrevive y el listado la muestra con is_available = false.
--   2. notes.note_tombstones registra (vía trigger AFTER DELETE) la metadata
--      mínima de cada apunte borrado, para poder responder 410 "apunte no
--      disponible (eliminado por el autor)" en vez de un 404 genérico o un 500.
--
-- Idempotente: se puede re-ejecutar sobre una base ya migrada.
-- Mantener alineado con la sección NOTES SERVICE de docker/postgres/init.sql.
BEGIN;

CREATE TABLE IF NOT EXISTS notes.note_tombstones (
    note_id uuid PRIMARY KEY,
    owner_user_id uuid NOT NULL,
    title varchar(300) NOT NULL,
    subject_id uuid,
    visibility varchar(20) NOT NULL,
    deleted_at timestamptz NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION notes.record_note_tombstone() RETURNS trigger AS $$
BEGIN
    INSERT INTO notes.note_tombstones (note_id, owner_user_id, title, subject_id, visibility)
    VALUES (OLD.id, OLD.user_id, OLD.title, OLD.subject_id, OLD.visibility)
    ON CONFLICT (note_id) DO UPDATE SET deleted_at = now();
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_notes_record_tombstone ON notes.notes;
CREATE TRIGGER trg_notes_record_tombstone
    AFTER DELETE ON notes.notes
    FOR EACH ROW EXECUTE FUNCTION notes.record_note_tombstone();

-- Quitar cualquier FK de saved_notes hacia notes.notes (el nombre por defecto
-- es saved_notes_note_id_fkey, pero se busca por catálogo por si difiere).
DO $$
DECLARE
    fk record;
BEGIN
    FOR fk IN
        SELECT conname FROM pg_constraint
        WHERE conrelid = 'notes.saved_notes'::regclass
          AND confrelid = 'notes.notes'::regclass
          AND contype = 'f'
    LOOP
        EXECUTE format('ALTER TABLE notes.saved_notes DROP CONSTRAINT %I', fk.conname);
    END LOOP;
END $$;

-- saves_count por apunte (COUNT por note_id).
CREATE INDEX IF NOT EXISTS idx_saved_notes_note_id ON notes.saved_notes(note_id);

COMMIT;
