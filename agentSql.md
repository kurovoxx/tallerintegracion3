-- =====================================================
-- SETUP INICIAL
-- =====================================================
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Si vas a recrear todo desde cero, descomenta esta línea primero:
-- DROP SCHEMA IF EXISTS identity, academic, notes, social, ai CASCADE;

-- =====================================================
-- SCHEMAS (uno por microservicio)
-- =====================================================
CREATE SCHEMA IF NOT EXISTS identity;
CREATE SCHEMA IF NOT EXISTS academic;
CREATE SCHEMA IF NOT EXISTS notes;
CREATE SCHEMA IF NOT EXISTS social;
CREATE SCHEMA IF NOT EXISTS ai;

-- =====================================================
-- IDENTITY SERVICE
-- =====================================================

CREATE TABLE identity.users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email varchar(255) NOT NULL UNIQUE,
    password_hash varchar(255) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE identity.refresh_tokens (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    token_hash varchar(255) NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_refresh_tokens_user_id ON identity.refresh_tokens(user_id);

CREATE TABLE identity.profiles (
    user_id uuid PRIMARY KEY REFERENCES identity.users(id) ON DELETE CASCADE,
    display_name varchar(100) NOT NULL,
    photo_url varchar(500),
    phone varchar(30),
    institution varchar(200),
    description text,
    visibility varchar(20) NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'private')),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Conexiones OAuth a servicios externos (Google Calendar, Google Drive) por usuario
CREATE TABLE identity.oauth_connections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    provider varchar(30) NOT NULL CHECK (provider IN ('google_calendar', 'google_drive')),
    access_token text NOT NULL,
    refresh_token text,
    expires_at timestamptz,
    external_account_email varchar(255),
    revoked_at timestamptz, -- se marca al detectar un 401/403 de la API del proveedor
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, provider)
);

-- =====================================================
-- ACADEMIC SERVICE
-- Nota: user_id es referencia LÓGICA a identity.users.id (sin FK física)
-- =====================================================

CREATE TABLE academic.curricula (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL,
    name varchar(200) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_curricula_user_id ON academic.curricula(user_id);

CREATE TABLE academic.subjects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    curriculum_id uuid NOT NULL REFERENCES academic.curricula(id) ON DELETE CASCADE,
    name varchar(200) NOT NULL,
    semester int,
    credits int,
    total_classes int,
    min_attendance_pct decimal(5,2),
    average_decimal_places smallint NOT NULL DEFAULT 2 CHECK (average_decimal_places BETWEEN 0 AND 2),
    status varchar(20) NOT NULL DEFAULT 'in_progress'
        CHECK (status IN ('in_progress', 'approved', 'failed', 'pending'))
);

CREATE INDEX idx_subjects_curriculum_id ON academic.subjects(curriculum_id);

CREATE TABLE academic.schedules (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subject_id uuid NOT NULL REFERENCES academic.subjects(id) ON DELETE CASCADE,
    day_of_week int NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
    start_time time NOT NULL,
    end_time time NOT NULL,
    CHECK (end_time > start_time)
);

CREATE INDEX idx_schedules_subject_id ON academic.schedules(subject_id);

-- Una "grade" sin score todavía = evaluación programada a futuro.
-- parent_grade_id: subnotas (una evaluación compuesta por sub-partes cuyo
-- peso se reparte entre ellas, no contra el total de la materia).
CREATE TABLE academic.grades (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subject_id uuid NOT NULL REFERENCES academic.subjects(id) ON DELETE CASCADE,
    parent_grade_id uuid REFERENCES academic.grades(id) ON DELETE CASCADE,
    is_final_exam boolean NOT NULL DEFAULT false,
    status varchar(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('graded', 'pending', 'excused', 'missing')),
    name varchar(200) NOT NULL,
    -- Escala interna 10-70 (equivale a 1.0-7.0 con un decimal, sin usar floats)
    score smallint CHECK (score BETWEEN 10 AND 70),
    weight decimal(4,3) NOT NULL,
    scheduled_date date,
    topics text,
    notify_discord boolean NOT NULL DEFAULT false,
    google_calendar_event_id varchar(255),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_grades_subject_id ON academic.grades(subject_id);
CREATE INDEX idx_grades_parent_id ON academic.grades(parent_grade_id);

-- Notas requisito a nivel de la materia del alumno (self-service, sin profesor).
CREATE TABLE academic.grade_requirements (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subject_id uuid NOT NULL REFERENCES academic.subjects(id) ON DELETE CASCADE,
    source_grade_id uuid REFERENCES academic.grades(id) ON DELETE CASCADE,
    checks_final_average boolean NOT NULL DEFAULT false,
    comparator varchar(2) NOT NULL CHECK (comparator IN ('>', '>=', '<', '<=', '=')),
    threshold_value decimal(5,2) NOT NULL,
    action varchar(20) NOT NULL CHECK (action IN ('force_exam', 'force_fail', 'force_pass')),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (checks_final_average = true OR source_grade_id IS NOT NULL)
);

CREATE INDEX idx_grade_requirements_subject_id ON academic.grade_requirements(subject_id);

CREATE TABLE academic.absences (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subject_id uuid NOT NULL REFERENCES academic.subjects(id) ON DELETE CASCADE,
    date date NOT NULL,
    justified boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_absences_subject_id ON academic.absences(subject_id);

-- =====================================================
-- NOTES SERVICE
-- =====================================================

-- El contenido del apunte vive como archivo Markdown en el Drive del autor,
-- no en Postgres. external_file_id es el ID de ese archivo en Drive; puede
-- ser nulo brevemente entre la creación local (offline-first) y el momento
-- en que la sincronización a Drive se completa.
CREATE TABLE notes.notes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL,
    subject_id uuid,
    title varchar(300) NOT NULL,
    external_file_id varchar(255),
    visibility varchar(20) NOT NULL DEFAULT 'private' CHECK (visibility IN ('public', 'private')),
    likes_count int NOT NULL DEFAULT 0,
    forked_from_note_id uuid REFERENCES notes.notes(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_notes_user_id ON notes.notes(user_id);
CREATE INDEX idx_notes_subject_id ON notes.notes(subject_id);
CREATE INDEX idx_notes_visibility ON notes.notes(visibility);

-- Archivos adjuntos (imágenes, PDF, etc), también alojados en el Drive del
-- autor. is_inline distingue si ya está referenciado dentro del Markdown del
-- apunte (imagen embebida) o si es un adjunto suelto listado aparte.
CREATE TABLE notes.note_attachments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    note_id uuid NOT NULL REFERENCES notes.notes(id) ON DELETE CASCADE,
    external_file_id varchar(255) NOT NULL,
    file_url varchar(500) NOT NULL,
    file_type varchar(50) NOT NULL, -- 'image/png', 'application/pdf', etc.
    file_name varchar(300),
    file_size_bytes int,
    is_inline boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_note_attachments_note_id ON notes.note_attachments(note_id);

-- Referencia (no copia): "guardar" un apunte de otro usuario para verlo después.
-- Si el original se borra, la fila desaparece sola (CASCADE) sin lógica especial.
CREATE TABLE notes.saved_notes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL, -- ref lógica a identity.users.id (quien guarda)
    note_id uuid NOT NULL REFERENCES notes.notes(id) ON DELETE CASCADE,
    saved_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, note_id)
);

CREATE INDEX idx_saved_notes_user_id ON notes.saved_notes(user_id);

-- Un like por usuario por apunte; likes_count en notes.notes se actualiza
-- en la misma transacción que el insert/delete aquí
CREATE TABLE notes.note_likes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    note_id uuid NOT NULL REFERENCES notes.notes(id) ON DELETE CASCADE,
    user_id uuid NOT NULL, -- ref lógica a identity.users.id
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (note_id, user_id)
);

CREATE INDEX idx_note_likes_note_id ON notes.note_likes(note_id);

-- is_admin_note: true si quien comparte es admin del grupo al momento de
-- compartir (Staff = admin, ya que el rol teacher se eliminó por completo).
-- access_mode: 'link' = cualquiera con el enlace puede ver el archivo en
-- Drive; 'restricted' = se otorga permiso nominal a cada miembro del grupo
-- por su email (más costoso de mantener, pero acota el acceso al grupo).
-- author_followers_snapshot: conteo de seguidores del autor al momento de
-- compartir, usado solo como criterio de desempate de prestigio.
CREATE TABLE notes.shared_notes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    note_id uuid NOT NULL REFERENCES notes.notes(id) ON DELETE CASCADE,
    group_id uuid NOT NULL,
    is_admin_note boolean NOT NULL DEFAULT false,
    access_mode varchar(20) NOT NULL DEFAULT 'link' CHECK (access_mode IN ('link', 'restricted')),
    author_followers_snapshot int NOT NULL DEFAULT 0,
    shared_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_shared_notes_group_id ON notes.shared_notes(group_id);

-- Cubre el filtro de mayor impacto (group_id + admin primero). El resto del
-- orden (likes, seguidores, fecha) se resuelve con un sort en memoria tras el
-- JOIN — no se puede cubrir todo con un solo índice porque likes_count vive
-- en notes.notes, no en esta tabla. A esta escala de datos, es instantáneo.
CREATE INDEX idx_shared_notes_group_priority
    ON notes.shared_notes (group_id, is_admin_note DESC);

-- =====================================================
-- SOCIAL / GROUPS SERVICE
-- =====================================================

-- Sin campo "type": ya no existe distinción study/teacher_class desde que se
-- eliminó por completo el rol administrativo de profesor. Todo grupo es un
-- espacio de colaboración entre pares, gestionado por su(s) admin.
CREATE TABLE social.groups (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name varchar(200) NOT NULL,
    description text,
    owner_user_id uuid NOT NULL,
    notes_restricted_to_staff boolean NOT NULL DEFAULT false,
    invite_token uuid NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE social.group_memberships (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid NOT NULL REFERENCES social.groups(id) ON DELETE CASCADE,
    user_id uuid NOT NULL,
    role varchar(20) NOT NULL DEFAULT 'member' CHECK (role IN ('admin', 'member')),
    joined_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (group_id, user_id)
);

CREATE INDEX idx_group_memberships_user_id ON social.group_memberships(user_id);

-- Soporta la query de sucesión automática de admin al borrar una cuenta
-- (filtra por group_id + role, ordena por joined_at)
CREATE INDEX idx_group_memberships_succession ON social.group_memberships(group_id, role, joined_at);

-- Bloquea reingreso aunque el usuario tenga un invite_token/link válido
CREATE TABLE social.banned_users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid NOT NULL REFERENCES social.groups(id) ON DELETE CASCADE,
    user_id uuid NOT NULL, -- ref lógica a identity.users.id
    banned_by_user_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (group_id, user_id)
);

CREATE TABLE social.follows (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    follower_user_id uuid NOT NULL,
    followee_user_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (follower_user_id, followee_user_id),
    CHECK (follower_user_id <> followee_user_id)
);

CREATE INDEX idx_follows_followee_id ON social.follows(followee_user_id);

-- Contador desnormalizado, mantenido en la misma transacción que insert/delete de follows
CREATE TABLE social.follow_counts (
    user_id uuid PRIMARY KEY, -- ref lógica a identity.users.id
    followers_count int NOT NULL DEFAULT 0,
    following_count int NOT NULL DEFAULT 0
);

-- Config de Discord por grupo: sirve tanto de "chat" (invite_url) como
-- para notificaciones de reuniones/evaluaciones (webhook_url).
-- category_id / channel_id quedan nulos mientras se use el modelo MVP
-- (servidor creado manualmente por el usuario); se usarían si más adelante
-- se automatiza con un bot administrando categorías dentro de un servidor
-- compartido (ver conversación: límite de la API de Discord para crear
-- servidores nuevos por bot).
CREATE TABLE social.discord_integrations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid NOT NULL REFERENCES social.groups(id) ON DELETE CASCADE,
    server_name varchar(200),
    invite_url varchar(500),
    webhook_url varchar(500),
    category_id varchar(50),
    channel_id varchar(50),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (group_id)
);

-- Canal de chat en Stream (getstream.io) por grupo, creado automáticamente
-- al crear el grupo. El backend emite tokens de conexión por usuario al
-- vuelo con su API secret — no se guardan credenciales de Stream por usuario,
-- solo el identificador del canal.
CREATE TABLE social.stream_channels (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid NOT NULL REFERENCES social.groups(id) ON DELETE CASCADE,
    channel_id varchar(255) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (group_id)
);

CREATE TABLE social.meetings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid NOT NULL REFERENCES social.groups(id) ON DELETE CASCADE,
    title varchar(300) NOT NULL,
    description text,
    scheduled_at timestamptz NOT NULL,
    created_by_user_id uuid NOT NULL,
    notify_discord boolean NOT NULL DEFAULT true,
    google_calendar_event_id varchar(255),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_meetings_group_id ON social.meetings(group_id);

-- To-do list simple (Kanban: Pendiente / En Progreso / Completada)
CREATE TABLE social.todo_boards (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid NOT NULL REFERENCES social.groups(id) ON DELETE CASCADE,
    name varchar(200) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_todo_boards_group_id ON social.todo_boards(group_id);

CREATE TABLE social.todo_tasks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    board_id uuid NOT NULL REFERENCES social.todo_boards(id) ON DELETE CASCADE,
    title varchar(300) NOT NULL,
    status varchar(20) NOT NULL DEFAULT 'todo' CHECK (status IN ('todo', 'in_progress', 'done')),
    assignee_user_id uuid,
    due_date date,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_todo_tasks_board_id ON social.todo_tasks(board_id);

-- Hoja de sprint elaborada: prioridad, estado, horas estimadas vs. usadas por día
CREATE TABLE social.sprint_sheets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid NOT NULL REFERENCES social.groups(id) ON DELETE CASCADE,
    name varchar(200) NOT NULL,
    period_start date,
    period_end date,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_sprint_sheets_group_id ON social.sprint_sheets(group_id);

CREATE TABLE social.sprint_sheet_tasks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    sheet_id uuid NOT NULL REFERENCES social.sprint_sheets(id) ON DELETE CASCADE,
    assignee_user_id uuid NOT NULL,
    title varchar(300) NOT NULL,
    priority varchar(10) NOT NULL DEFAULT 'media' CHECK (priority IN ('alta', 'media', 'baja')),
    status varchar(20) NOT NULL DEFAULT 'sin_empezar'
        CHECK (status IN ('sin_empezar', 'en_proceso', 'listo')),
    estimated_hours decimal(5,2) NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_sprint_sheet_tasks_sheet_id ON social.sprint_sheet_tasks(sheet_id);
CREATE INDEX idx_sprint_sheet_tasks_assignee ON social.sprint_sheet_tasks(assignee_user_id);

-- Horas registradas por tarea en una fecha específica del sprint
-- (horas usadas = SUM(hours); horas restantes = estimated_hours - SUM(hours))
CREATE TABLE social.sprint_sheet_daily_hours (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id uuid NOT NULL REFERENCES social.sprint_sheet_tasks(id) ON DELETE CASCADE,
    log_date date NOT NULL,
    hours decimal(4,2) NOT NULL CHECK (hours >= 0),
    UNIQUE (task_id, log_date)
);

CREATE INDEX idx_sprint_sheet_daily_hours_task_id ON social.sprint_sheet_daily_hours(task_id);

-- Notificación in-app de una reunión, dirigida a cada miembro del grupo
-- (excepto quien la creó). Reemplaza el antiguo schema "messaging" genérico:
-- al ser el único caso de uso de notificación in-app del MVP, no se justifica
-- un schema/servicio aparte para una sola tabla con un solo evento disparador.
CREATE TABLE social.meeting_notifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    meeting_id uuid NOT NULL REFERENCES social.meetings(id) ON DELETE CASCADE,
    user_id uuid NOT NULL, -- ref lógica a identity.users.id (destinatario)
    read_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_meeting_notifications_user_id ON social.meeting_notifications(user_id);

-- =====================================================
-- AI ASSISTANT SERVICE
-- =====================================================

CREATE TABLE ai.conversations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_conversations_user_id ON ai.conversations(user_id);

CREATE TABLE ai.messages (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id uuid NOT NULL REFERENCES ai.conversations(id) ON DELETE CASCADE,
    role varchar(20) NOT NULL CHECK (role IN ('user', 'assistant')),
    content text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_messages_conversation_id ON ai.messages(conversation_id);

-- =====================================================
-- GRANTS PARA LA DATA API (schemas personalizados)
-- =====================================================

GRANT USAGE ON SCHEMA identity, academic, notes, social, ai
    TO service_role, authenticated, anon;

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA identity TO service_role;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA academic TO service_role;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA notes TO service_role;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA social TO service_role;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA ai TO service_role;

ALTER DEFAULT PRIVILEGES IN SCHEMA identity, academic, notes, social, ai
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO service_role;