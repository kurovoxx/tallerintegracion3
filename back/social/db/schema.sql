CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE SCHEMA IF NOT EXISTS social;

CREATE TABLE IF NOT EXISTS social.groups (
  id uuid not null default gen_random_uuid (),
  name character varying(200) not null,
  description text null,
  owner_user_id uuid not null,
  notes_restricted_to_staff boolean not null default false,
  invite_token uuid not null default gen_random_uuid (),
  created_at timestamp with time zone not null default now(),
  constraint groups_pkey primary key (id),
  constraint groups_invite_token_key unique (invite_token)
);

CREATE TABLE IF NOT EXISTS social.todo_boards (
  id uuid not null default gen_random_uuid (),
  group_id uuid not null,
  name character varying(200) not null,
  created_at timestamp with time zone not null default now(),
  constraint todo_boards_pkey primary key (id),
  constraint todo_boards_group_id_fkey foreign KEY (group_id) references social.groups (id) on delete CASCADE
);

CREATE INDEX IF NOT EXISTS idx_todo_boards_group_id on social.todo_boards using btree (group_id);

CREATE TABLE IF NOT EXISTS social.todo_tasks (
  id uuid not null default gen_random_uuid (),
  board_id uuid not null,
  title character varying(300) not null,
  status character varying(20) not null default 'todo',
  assignee_user_id uuid null,
  due_date date null,
  created_at timestamp with time zone not null default now(),
  updated_at timestamp with time zone not null default now(),
  constraint todo_tasks_pkey primary key (id),
  constraint todo_tasks_board_id_fkey foreign KEY (board_id) references social.todo_boards (id) on delete CASCADE,
  constraint todo_tasks_status_check check (status in ('todo', 'in_progress', 'done'))
);

CREATE INDEX IF NOT EXISTS idx_todo_tasks_board_id on social.todo_tasks using btree (board_id);

CREATE TABLE IF NOT EXISTS social.sprint_sheets (
  id uuid not null default gen_random_uuid (),
  group_id uuid not null,
  name character varying(200) not null,
  period_start date null,
  period_end date null,
  created_at timestamp with time zone not null default now(),
  constraint sprint_sheets_pkey primary key (id),
  constraint sprint_sheets_group_id_fkey foreign KEY (group_id) references social.groups (id) on delete CASCADE
);

CREATE INDEX IF NOT EXISTS idx_sprint_sheets_group_id on social.sprint_sheets using btree (group_id);

CREATE TABLE IF NOT EXISTS social.sprint_sheet_tasks (
  id uuid not null default gen_random_uuid (),
  sheet_id uuid not null,
  assignee_user_id uuid not null,
  title character varying(300) not null,
  priority character varying(10) not null default 'media',
  status character varying(20) not null default 'sin_empezar',
  estimated_hours numeric(5, 2) not null default 0,
  created_at timestamp with time zone not null default now(),
  updated_at timestamp with time zone not null default now(),
  constraint sprint_sheet_tasks_pkey primary key (id),
  constraint sprint_sheet_tasks_sheet_id_fkey foreign KEY (sheet_id) references social.sprint_sheets (id) on delete CASCADE,
  constraint sprint_sheet_tasks_priority_check check (priority in ('alta', 'media', 'baja')),
  constraint sprint_sheet_tasks_status_check check (status in ('sin_empezar', 'en_proceso', 'listo'))
);

CREATE INDEX IF NOT EXISTS idx_sprint_sheet_tasks_sheet_id on social.sprint_sheet_tasks using btree (sheet_id);
CREATE INDEX IF NOT EXISTS idx_sprint_sheet_tasks_assignee on social.sprint_sheet_tasks using btree (assignee_user_id);

CREATE TABLE IF NOT EXISTS social.sprint_sheet_daily_hours (
  id uuid not null default gen_random_uuid (),
  task_id uuid not null,
  log_date date not null,
  hours numeric(4, 2) not null,
  constraint sprint_sheet_daily_hours_pkey primary key (id),
  constraint sprint_sheet_daily_hours_task_id_log_date_key unique (task_id, log_date),
  constraint sprint_sheet_daily_hours_task_id_fkey foreign KEY (task_id) references social.sprint_sheet_tasks (id) on delete CASCADE,
  constraint sprint_sheet_daily_hours_hours_check check (hours >= 0)
);

CREATE INDEX IF NOT EXISTS idx_sprint_sheet_daily_hours_task_id on social.sprint_sheet_daily_hours using btree (task_id);

CREATE TABLE IF NOT EXISTS social.meetings (
  id uuid not null default gen_random_uuid (),
  group_id uuid not null,
  title character varying(300) not null,
  description text null,
  scheduled_at timestamp with time zone not null,
  created_by_user_id uuid not null,
  notify_discord boolean not null default true,
  google_calendar_event_id character varying(255) null,
  created_at timestamp with time zone not null default now(),
  constraint meetings_pkey primary key (id),
  constraint meetings_group_id_fkey foreign KEY (group_id) references social.groups (id) on delete CASCADE
);

CREATE INDEX IF NOT EXISTS idx_meetings_group_id on social.meetings using btree (group_id);

CREATE TABLE IF NOT EXISTS social.meeting_notifications (
  id uuid not null default gen_random_uuid (),
  meeting_id uuid not null,
  user_id uuid not null,
  read_at timestamp with time zone null,
  created_at timestamp with time zone not null default now(),
  constraint meeting_notifications_pkey primary key (id),
  constraint meeting_notifications_meeting_id_fkey foreign KEY (meeting_id) references social.meetings (id) on delete CASCADE
);

CREATE INDEX IF NOT EXISTS idx_meeting_notifications_user_id on social.meeting_notifications using btree (user_id);
