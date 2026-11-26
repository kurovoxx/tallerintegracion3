-- Esquema inicial del servicio académico (infacadem).
-- Todas las tablas viven en el schema `academic`. user_id es referencia
-- lógica a identity.users.id (sin FK física, como el resto de servicios).
-- Idempotente: CREATE TABLE IF NOT EXISTS / CREATE INDEX IF NOT EXISTS.

CREATE SCHEMA IF NOT EXISTS academic;

CREATE TABLE IF NOT EXISTS academic.curricula (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL,
    name varchar(200) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_curricula_user_id
    ON academic.curricula(user_id);

CREATE TABLE IF NOT EXISTS academic.subjects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    curriculum_id uuid NOT NULL REFERENCES academic.curricula(id) ON DELETE CASCADE,
    name varchar(200) NOT NULL,
    code varchar(50),
    professor varchar(200),
    room varchar(100),
    semester int,
    credits int,
    total_classes int,
    min_attendance_pct decimal(5,2),
    average_decimal_places smallint NOT NULL DEFAULT 2
        CHECK (average_decimal_places BETWEEN 0 AND 2),
    status varchar(20) NOT NULL DEFAULT 'in_progress'
        CHECK (status IN ('in_progress', 'approved', 'failed', 'pending'))
);

CREATE INDEX IF NOT EXISTS idx_subjects_curriculum_id
    ON academic.subjects(curriculum_id);

CREATE TABLE IF NOT EXISTS academic.schedules (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subject_id uuid NOT NULL REFERENCES academic.subjects(id) ON DELETE CASCADE,
    day_of_week int NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
    start_time time NOT NULL,
    end_time time NOT NULL,
    modality varchar(50),
    CHECK (end_time > start_time)
);

CREATE INDEX IF NOT EXISTS idx_schedules_subject_id
    ON academic.schedules(subject_id);

-- Contenidos (unidades) de una materia para el front.
CREATE TABLE IF NOT EXISTS academic.contents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subject_id uuid NOT NULL REFERENCES academic.subjects(id) ON DELETE CASCADE,
    title varchar(200) NOT NULL,
    body text,
    position int NOT NULL DEFAULT 0,
    done boolean NOT NULL DEFAULT false
);

CREATE INDEX IF NOT EXISTS idx_contents_subject_id
    ON academic.contents(subject_id);

-- Una "grade" sin score todavía = evaluación programada a futuro.
-- parent_grade_id: subnotas (evaluación compuesta).
CREATE TABLE IF NOT EXISTS academic.grades (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subject_id uuid NOT NULL REFERENCES academic.subjects(id) ON DELETE CASCADE,
    parent_grade_id uuid REFERENCES academic.grades(id) ON DELETE CASCADE,
    is_final_exam boolean NOT NULL DEFAULT false,
    status varchar(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('graded', 'pending', 'excused', 'missing')),
    name varchar(200) NOT NULL,
    -- Escala interna 10-70 (equivale a 1.0-7.0 con un decimal, sin floats)
    score smallint CHECK (score BETWEEN 10 AND 70),
    weight decimal(4,3) NOT NULL,
    scheduled_date date,
    topics text,
    notify_discord boolean NOT NULL DEFAULT false,
    google_calendar_event_id varchar(255),
    pinned boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_grades_subject_id
    ON academic.grades(subject_id);
CREATE INDEX IF NOT EXISTS idx_grades_parent_id
    ON academic.grades(parent_grade_id);

-- Notas requisito a nivel de la materia (self-service, sin profesor).
CREATE TABLE IF NOT EXISTS academic.grade_requirements (
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

CREATE INDEX IF NOT EXISTS idx_grade_requirements_subject_id
    ON academic.grade_requirements(subject_id);

-- Inasistencias por materia.
CREATE TABLE IF NOT EXISTS academic.absences (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subject_id uuid NOT NULL REFERENCES academic.subjects(id) ON DELETE CASCADE,
    date date NOT NULL,
    justified boolean NOT NULL DEFAULT false,
    source varchar(20),
    type varchar(50),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_absences_subject_id
    ON academic.absences(subject_id);
