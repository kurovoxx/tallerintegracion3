-- 006: integridad de la copia de apuntes y procedencia de adjuntos copiados.
--
-- 1) Un usuario solo puede tener un clon vivo por apunte origen. El índice es
--    parcial porque forked_from_note_id queda NULL al borrar el original
--    (ON DELETE SET NULL) y las notas creadas desde cero no participan.
-- 2) Cada adjunto clonado recuerda de qué adjunto del original proviene. La
--    unicidad (note_id, copied_from_attachment_id) hace idempotente el copiado
--    de adjuntos: un reintento tras un crash reutiliza la fila ya creada en
--    lugar de duplicar archivos en Drive.
--
-- No se eliminan apuntes automáticamente. Si la base ya contiene duplicados
-- históricos, el preflight falla con un mensaje accionable para resolverlos
-- manualmente antes de aplicar la restricción.

BEGIN;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM notes.notes
        WHERE forked_from_note_id IS NOT NULL
        GROUP BY user_id, forked_from_note_id
        HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION
            'notes 006: existen copias duplicadas (user_id, forked_from_note_id); resuélvelas manualmente antes de aplicar la migración';
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_notes_user_forked_from
    ON notes.notes (user_id, forked_from_note_id)
    WHERE forked_from_note_id IS NOT NULL;

ALTER TABLE notes.note_attachments
    ADD COLUMN IF NOT EXISTS copied_from_attachment_id uuid;

COMMENT ON COLUMN notes.note_attachments.copied_from_attachment_id IS
    'Adjunto del apunte original del que se copió este archivo (solo copias)';

CREATE UNIQUE INDEX IF NOT EXISTS uq_note_attachments_copy_source
    ON notes.note_attachments (note_id, copied_from_attachment_id)
    WHERE copied_from_attachment_id IS NOT NULL;

COMMIT;
