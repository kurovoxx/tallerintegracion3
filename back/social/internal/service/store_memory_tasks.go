package service

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
)

// Fakes en memoria para TodoRepo, SprintRepo y HoursRepo.
// Separados en dos structs porque Todo y Sprint usan los mismos nombres de
// método (GetTaskByID, CreateTask, ...) con tipos distintos.
// Ambos embeben *MemoryMeetingStore para grupos, miembros y utilidades.

func newUUID() pgtype.UUID {
	var u pgtype.UUID
	_ = u.Scan(uuid.NewString())
	return u
}

func nowTSTZ() pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
}

func uuidStr(u pgtype.UUID) string {
	id, _ := uuid.FromBytes(u.Bytes[:])
	return id.String()
}

// uniqueViolation simula el error 23505 de Postgres (único violado).
func uniqueViolation() error {
	return &pgconn.PgError{Code: "23505", Message: "duplicate key value"}
}

// ---------- tableros y tareas todo ----------

type MemoryTodoStore struct {
	*MemoryMeetingStore
	mu     sync.Mutex
	boards map[string]sqlc.SocialTodoBoard
	tasks  map[string]sqlc.SocialTodoTask
}

func NewMemoryTodoStore() *MemoryTodoStore {
	return &MemoryTodoStore{
		MemoryMeetingStore: NewMemoryMeetingStore(),
		boards:             map[string]sqlc.SocialTodoBoard{},
		tasks:              map[string]sqlc.SocialTodoTask{},
	}
}

func (m *MemoryTodoStore) GetBoardByID(_ context.Context, id pgtype.UUID) (sqlc.SocialTodoBoard, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if b, ok := m.boards[uuidStr(id)]; ok {
		return b, nil
	}
	return sqlc.SocialTodoBoard{}, pgx.ErrNoRows
}

func (m *MemoryTodoStore) ListBoardsByGroup(_ context.Context, groupID pgtype.UUID) ([]sqlc.SocialTodoBoard, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []sqlc.SocialTodoBoard
	for _, b := range m.boards {
		if uuidStr(b.GroupID) == uuidStr(groupID) {
			out = append(out, b)
		}
	}
	return out, nil
}

func (m *MemoryTodoStore) CreateBoard(_ context.Context, arg sqlc.CreateBoardParams) (sqlc.SocialTodoBoard, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := sqlc.SocialTodoBoard{ID: newUUID(), GroupID: arg.GroupID, Name: arg.Name, CreatedAt: nowTSTZ()}
	m.boards[uuidStr(b.ID)] = b
	return b, nil
}

func (m *MemoryTodoStore) GetTaskByID(_ context.Context, id pgtype.UUID) (sqlc.SocialTodoTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.tasks[uuidStr(id)]; ok {
		return t, nil
	}
	return sqlc.SocialTodoTask{}, pgx.ErrNoRows
}

func (m *MemoryTodoStore) CreateTask(_ context.Context, arg sqlc.CreateTodoTaskParams) (sqlc.SocialTodoTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := sqlc.SocialTodoTask{
		ID: newUUID(), BoardID: arg.BoardID, Title: arg.Title, Status: arg.Status,
		AssigneeUserID: arg.AssigneeUserID, DueDate: arg.DueDate,
		CreatedAt: nowTSTZ(), UpdatedAt: nowTSTZ(),
	}
	m.tasks[uuidStr(t.ID)] = t
	return t, nil
}

func (m *MemoryTodoStore) ListTasksByGroup(_ context.Context, arg sqlc.ListTodoTasksByGroupParams) ([]sqlc.ListTodoTasksByGroupRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []sqlc.ListTodoTasksByGroupRow
	for _, t := range m.tasks {
		b, ok := m.boards[uuidStr(t.BoardID)]
		if !ok {
			continue
		}
		if uuidStr(b.GroupID) != uuidStr(arg.GroupID) {
			continue
		}
		if arg.Status != "" && t.Status != arg.Status {
			continue
		}
		if arg.BoardID.Valid && uuidStr(t.BoardID) != uuidStr(arg.BoardID) {
			continue
		}
		out = append(out, sqlc.ListTodoTasksByGroupRow{
			ID: t.ID, BoardID: t.BoardID, Title: t.Title, Status: t.Status,
			AssigneeUserID: t.AssigneeUserID, DueDate: t.DueDate,
			CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, BoardName: b.Name,
		})
	}
	return out, nil
}

func (m *MemoryTodoStore) UpdateTask(_ context.Context, arg sqlc.UpdateTodoTaskParams) (sqlc.SocialTodoTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[uuidStr(arg.ID)]
	if !ok {
		return sqlc.SocialTodoTask{}, pgx.ErrNoRows
	}
	t.Title = arg.Title
	t.Status = arg.Status
	t.AssigneeUserID = arg.AssigneeUserID
	t.DueDate = arg.DueDate
	t.UpdatedAt = nowTSTZ()
	m.tasks[uuidStr(arg.ID)] = t
	return t, nil
}

func (m *MemoryTodoStore) DeleteTask(_ context.Context, id pgtype.UUID) (sqlc.SocialTodoTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[uuidStr(id)]
	if !ok {
		return sqlc.SocialTodoTask{}, pgx.ErrNoRows
	}
	delete(m.tasks, uuidStr(id))
	return t, nil
}

// ---------- hojas, tareas sprint y horas ----------

type MemorySprintStore struct {
	*MemoryMeetingStore
	mu      sync.Mutex
	sheets  map[string]sqlc.SocialSprintSheet
	tasks   map[string]sqlc.SocialSprintSheetTask
	hours   map[string]sqlc.SocialSprintSheetDailyHour
	hourKey map[string]string // "taskID|logDate" -> hourID
}

func NewMemorySprintStore() *MemorySprintStore {
	return &MemorySprintStore{
		MemoryMeetingStore: NewMemoryMeetingStore(),
		sheets:             map[string]sqlc.SocialSprintSheet{},
		tasks:              map[string]sqlc.SocialSprintSheetTask{},
		hours:              map[string]sqlc.SocialSprintSheetDailyHour{},
		hourKey:            map[string]string{},
	}
}

func (m *MemorySprintStore) GetSheetByID(_ context.Context, id pgtype.UUID) (sqlc.SocialSprintSheet, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sheets[uuidStr(id)]; ok {
		return s, nil
	}
	return sqlc.SocialSprintSheet{}, pgx.ErrNoRows
}

func (m *MemorySprintStore) ListSheetsByGroup(_ context.Context, groupID pgtype.UUID) ([]sqlc.SocialSprintSheet, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []sqlc.SocialSprintSheet
	for _, s := range m.sheets {
		if uuidStr(s.GroupID) == uuidStr(groupID) {
			out = append(out, s)
		}
	}
	return out, nil
}

func (m *MemorySprintStore) CreateSheet(_ context.Context, arg sqlc.CreateSheetParams) (sqlc.SocialSprintSheet, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := sqlc.SocialSprintSheet{ID: newUUID(), GroupID: arg.GroupID, Name: arg.Name, CreatedAt: nowTSTZ()}
	m.sheets[uuidStr(s.ID)] = s
	return s, nil
}

func (m *MemorySprintStore) GetTaskByID(_ context.Context, id pgtype.UUID) (sqlc.SocialSprintSheetTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.tasks[uuidStr(id)]; ok {
		return t, nil
	}
	return sqlc.SocialSprintSheetTask{}, pgx.ErrNoRows
}

func (m *MemorySprintStore) CreateTask(_ context.Context, arg sqlc.CreateSprintSheetTaskParams) (sqlc.SocialSprintSheetTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := sqlc.SocialSprintSheetTask{
		ID: newUUID(), SheetID: arg.SheetID, AssigneeUserID: arg.AssigneeUserID,
		Title: arg.Title, Priority: arg.Priority, Status: arg.Status,
		EstimatedHours: arg.EstimatedHours, CreatedAt: nowTSTZ(), UpdatedAt: nowTSTZ(),
	}
	m.tasks[uuidStr(t.ID)] = t
	return t, nil
}

func (m *MemorySprintStore) ListTasksByGroup(_ context.Context, arg sqlc.ListSprintSheetTasksByGroupParams) ([]sqlc.ListSprintSheetTasksByGroupRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []sqlc.ListSprintSheetTasksByGroupRow
	for _, t := range m.tasks {
		s, ok := m.sheets[uuidStr(t.SheetID)]
		if !ok {
			continue
		}
		if uuidStr(s.GroupID) != uuidStr(arg.GroupID) {
			continue
		}
		if arg.Status != "" && t.Status != arg.Status {
			continue
		}
		if arg.Priority != "" && t.Priority != arg.Priority {
			continue
		}
		if arg.SheetID.Valid && uuidStr(t.SheetID) != uuidStr(arg.SheetID) {
			continue
		}
		out = append(out, sqlc.ListSprintSheetTasksByGroupRow{
			ID: t.ID, SheetID: t.SheetID, AssigneeUserID: t.AssigneeUserID, Title: t.Title,
			Priority: t.Priority, Status: t.Status, EstimatedHours: t.EstimatedHours,
			CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, SheetName: s.Name,
		})
	}
	return out, nil
}

func (m *MemorySprintStore) UpdateTask(_ context.Context, arg sqlc.UpdateSprintSheetTaskParams) (sqlc.SocialSprintSheetTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[uuidStr(arg.ID)]
	if !ok {
		return sqlc.SocialSprintSheetTask{}, pgx.ErrNoRows
	}
	t.Title = arg.Title
	t.AssigneeUserID = arg.AssigneeUserID
	t.Priority = arg.Priority
	t.Status = arg.Status
	t.EstimatedHours = arg.EstimatedHours
	t.UpdatedAt = nowTSTZ()
	m.tasks[uuidStr(arg.ID)] = t
	return t, nil
}

func (m *MemorySprintStore) DeleteTask(_ context.Context, id pgtype.UUID) (sqlc.SocialSprintSheetTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[uuidStr(id)]
	if !ok {
		return sqlc.SocialSprintSheetTask{}, pgx.ErrNoRows
	}
	delete(m.tasks, uuidStr(id))
	return t, nil
}

func logDateStr(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("2006-01-02")
}

func (m *MemorySprintStore) GetByTaskAndDate(_ context.Context, arg sqlc.GetDailyHoursByTaskAndDateParams) (sqlc.SocialSprintSheetDailyHour, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.hourKey[uuidStr(arg.TaskID)+"|"+logDateStr(arg.LogDate)]; ok {
		return m.hours[id], nil
	}
	return sqlc.SocialSprintSheetDailyHour{}, pgx.ErrNoRows
}

func (m *MemorySprintStore) ListByTask(_ context.Context, arg sqlc.ListDailyHoursByTaskParams) ([]sqlc.SocialSprintSheetDailyHour, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []sqlc.SocialSprintSheetDailyHour
	for _, h := range m.hours {
		if uuidStr(h.TaskID) == uuidStr(arg.TaskID) {
			out = append(out, h)
		}
	}
	return out, nil
}

func (m *MemorySprintStore) Create(_ context.Context, arg sqlc.CreateDailyHoursParams) (sqlc.SocialSprintSheetDailyHour, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := uuidStr(arg.TaskID) + "|" + logDateStr(arg.LogDate)
	if _, exists := m.hourKey[k]; exists {
		return sqlc.SocialSprintSheetDailyHour{}, uniqueViolation()
	}
	h := sqlc.SocialSprintSheetDailyHour{ID: newUUID(), TaskID: arg.TaskID, LogDate: arg.LogDate, Hours: arg.Hours}
	m.hours[uuidStr(h.ID)] = h
	m.hourKey[k] = uuidStr(h.ID)
	return h, nil
}

func (m *MemorySprintStore) Update(_ context.Context, arg sqlc.UpdateDailyHoursParams) (sqlc.SocialSprintSheetDailyHour, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.hours[uuidStr(arg.ID)]
	if !ok {
		return sqlc.SocialSprintSheetDailyHour{}, pgx.ErrNoRows
	}
	h.Hours = arg.Hours
	m.hours[uuidStr(arg.ID)] = h
	return h, nil
}

var (
	_ TodoRepo   = (*MemoryTodoStore)(nil)
	_ SprintRepo = (*MemorySprintStore)(nil)
	_ HoursRepo  = (*MemorySprintStore)(nil)
)
