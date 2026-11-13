-- Enlace o sala de la reunión (Meet, Zoom, sala física, etc.).
-- Se incluye en el correo de invitación a los asistentes.
ALTER TABLE social.meetings
    ADD COLUMN IF NOT EXISTS link varchar(500);
