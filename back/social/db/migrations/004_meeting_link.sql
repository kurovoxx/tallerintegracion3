-- Enlace/sala de la reunión (Meet, Zoom, etc.). Se muestra en la notificación
-- in-app y se sincroniza como descripción del evento de Google Calendar.
ALTER TABLE social.meetings
  ADD COLUMN IF NOT EXISTS link character varying(1000);
