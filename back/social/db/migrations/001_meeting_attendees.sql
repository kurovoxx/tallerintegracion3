-- Invitados por email de una reunión (selección del creador en la agenda).
-- Van al evento de Google Calendar como attendees. Las notificaciones in-app
-- siguen la regla 4.8 (todos los miembros excepto el creador).
CREATE TABLE IF NOT EXISTS social.meeting_attendees (
  id uuid not null default gen_random_uuid (),
  meeting_id uuid not null,
  email character varying(255) not null,
  created_at timestamp with time zone not null default now(),
  constraint meeting_attendees_pkey primary key (id),
  constraint meeting_attendees_meeting_id_fkey foreign KEY (meeting_id) references social.meetings (id) on delete CASCADE,
  constraint meeting_attendees_meeting_email_key unique (meeting_id, email)
);
