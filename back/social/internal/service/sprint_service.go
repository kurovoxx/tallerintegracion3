package service

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
)

var (
	ErrInvalidPriority       = errors.New("invalid priority: must be alta, media or baja")
	ErrInvalidSprintStatus   = errors.New("invalid status: must be sin_empezar, en_proceso or listo")
	ErrSprintTaskNotFound    = errors.New("sprint task not found")
	ErrSheetNotFound         = errors.New("sheet not found in group")
	ErrInvalidSprintTaskID   = errors.New("invalid sprint task ID")
	ErrInvalidSheetID        = errors.New("invalid sheet ID")
	ErrAssigneeRequired      = errors.New("assigned_to is required")
	ErrInvalidEstimatedHours = errors.New("invalid estimated_hours: must be a number >= 0")
)

var validPriorities = map[string]struct{}{
	"alta":  {},
	"media": {},
	"baja":  {},
}

var validSprintStatuses = map[string]struct{}{
	"sin_empezar": {},
	"en_proceso":  {},
	"listo":       {},
}

const defaultSheetName = "Sprint 1"

type SprintService struct {
	repo SprintRepo
}

// SprintRepo es la porción de persistencia que usa SprintService.
// *repository.SprintRepository es la implementación real (Postgres);
// MemoryTasksStore, la de tests.
type SprintRepo interface {
	UpdateSheet(ctx context.Context, arg sqlc.UpdateSheetParams) (sqlc.SocialSprintSheet, error)
	GetGroupByID(ctx context.Context, id pgtype.UUID) (sqlc.SocialGroup, error)
	IsMember(ctx context.Context, groupID, userID pgtype.UUID) (bool, error)
	GetSheetByID(ctx context.Context, id pgtype.UUID) (sqlc.SocialSprintSheet, error)
	ListSheetsByGroup(ctx context.Context, groupID pgtype.UUID) ([]sqlc.SocialSprintSheet, error)
	CreateSheet(ctx context.Context, arg sqlc.CreateSheetParams) (sqlc.SocialSprintSheet, error)
	GetTaskByID(ctx context.Context, id pgtype.UUID) (sqlc.SocialSprintSheetTask, error)
	CreateTask(ctx context.Context, arg sqlc.CreateSprintSheetTaskParams) (sqlc.SocialSprintSheetTask, error)
	ListTasksByGroup(ctx context.Context, arg sqlc.ListSprintSheetTasksByGroupParams) ([]sqlc.ListSprintSheetTasksByGroupRow, error)
	UpdateTask(ctx context.Context, arg sqlc.UpdateSprintSheetTaskParams) (sqlc.SocialSprintSheetTask, error)
	DeleteTask(ctx context.Context, id pgtype.UUID) (sqlc.SocialSprintSheetTask, error)
}

func NewSprintService(repo SprintRepo) *SprintService {
	return &SprintService{
		repo: repo,
	}
}

type SprintTaskView struct {
	Task  sqlc.SocialSprintSheetTask
	Sheet sqlc.SocialSprintSheet
}

func (s *SprintService) requireGroup(ctx context.Context, gid pgtype.UUID) error {
	if _, err := s.repo.GetGroupByID(ctx, gid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrGroupNotFound
		}
		return err
	}
	return nil
}

// requireMembership exige que el usuario pertenezca al grupo (403 si no).
func (s *SprintService) requireMembership(ctx context.Context, gid, uid pgtype.UUID) error {
	isMember, err := s.repo.IsMember(ctx, gid, uid)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrForbidden
	}
	return nil
}

// requireAssigneeMember exige regla 4.4: el asignado debe ser miembro del grupo.
func (s *SprintService) requireAssigneeMember(ctx context.Context, gid, auid pgtype.UUID) error {
	if !auid.Valid {
		return ErrAssigneeRequired
	}
	isMember, err := s.repo.IsMember(ctx, gid, auid)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrAssigneeNotMember
	}
	return nil
}

// requireTaskInGroup verifica que la hoja de la tarea pertenece al grupo.
// Tarea de otro grupo (u hoja inexistente) => ErrSprintTaskNotFound (404, sin filtrar).
func (s *SprintService) requireTaskInGroup(ctx context.Context, gid, sheetID pgtype.UUID) error {
	sheet, err := s.repo.GetSheetByID(ctx, sheetID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSprintTaskNotFound
		}
		return err
	}
	if sheet.GroupID != gid {
		return ErrSprintTaskNotFound
	}
	return nil
}

func (s *SprintService) resolveSheet(ctx context.Context, gid pgtype.UUID, sheetID string) (sqlc.SocialSprintSheet, error) {
	sheetID = strings.TrimSpace(sheetID)
	if sheetID != "" {
		var sid pgtype.UUID
		if _, err := uuid.Parse(sheetID); err != nil {
			return sqlc.SocialSprintSheet{}, ErrInvalidSheetID
		}
		if err := sid.Scan(sheetID); err != nil {
			return sqlc.SocialSprintSheet{}, ErrInvalidSheetID
		}
		sheet, err := s.repo.GetSheetByID(ctx, sid)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return sqlc.SocialSprintSheet{}, ErrSheetNotFound
			}
			return sqlc.SocialSprintSheet{}, err
		}
		if sheet.GroupID != gid {
			return sqlc.SocialSprintSheet{}, ErrSheetNotFound
		}
		return sheet, nil
	}

	sheets, err := s.repo.ListSheetsByGroup(ctx, gid)
	if err != nil {
		return sqlc.SocialSprintSheet{}, err
	}
	if len(sheets) > 0 {
		return sheets[0], nil
	}
	return s.repo.CreateSheet(ctx, sqlc.CreateSheetParams{GroupID: gid, Name: defaultSheetName})
}

func normalizePriority(priority string) (string, error) {
	priority = strings.TrimSpace(priority)
	if priority == "" {
		priority = "media"
	}
	if _, ok := validPriorities[priority]; !ok {
		return "", ErrInvalidPriority
	}
	return priority, nil
}

func normalizeSprintStatus(status string) (string, error) {
	status = strings.TrimSpace(status)
	if status == "" {
		status = "sin_empezar"
	}
	if _, ok := validSprintStatuses[status]; !ok {
		return "", ErrInvalidSprintStatus
	}
	return status, nil
}

func parseRequiredAssignee(assignedTo string) (pgtype.UUID, error) {
	var auid pgtype.UUID
	assignedTo = strings.TrimSpace(assignedTo)
	if assignedTo == "" {
		return auid, ErrAssigneeRequired
	}
	if _, err := uuid.Parse(assignedTo); err != nil {
		return auid, ErrInvalidUserID
	}
	if err := auid.Scan(assignedTo); err != nil {
		return auid, ErrInvalidUserID
	}
	return auid, nil
}

func parseEstimatedHours(hours *float64) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if hours == nil {
		if err := n.Scan("0"); err != nil {
			return n, err
		}
		return n, nil
	}
	if *hours < 0 {
		return n, ErrInvalidEstimatedHours
	}
	if err := n.Scan(strconv.FormatFloat(*hours, 'f', 2, 64)); err != nil {
		return n, ErrInvalidEstimatedHours
	}
	return n, nil
}

func (s *SprintService) CreateSprintTask(ctx context.Context, groupID, userID, sheetID, title, assignedTo, priority, status string, estimatedHours *float64) (SprintTaskView, error) {
	title, err := validateTitle(title)
	if err != nil {
		return SprintTaskView{}, err
	}
	priority, err = normalizePriority(priority)
	if err != nil {
		return SprintTaskView{}, err
	}
	status, err = normalizeSprintStatus(status)
	if err != nil {
		return SprintTaskView{}, err
	}
	assignee, err := parseRequiredAssignee(assignedTo)
	if err != nil {
		return SprintTaskView{}, err
	}
	hours, err := parseEstimatedHours(estimatedHours)
	if err != nil {
		return SprintTaskView{}, err
	}

	gid, err := parseGroupUUID(groupID)
	if err != nil {
		return SprintTaskView{}, err
	}
	uid, err := parseMeetingUserUUID(userID)
	if err != nil {
		return SprintTaskView{}, err
	}
	if err := s.requireGroup(ctx, gid); err != nil {
		return SprintTaskView{}, err
	}
	if err := s.requireMembership(ctx, gid, uid); err != nil {
		return SprintTaskView{}, err
	}
	if err := s.requireAssigneeMember(ctx, gid, assignee); err != nil {
		return SprintTaskView{}, err
	}
	sheet, err := s.resolveSheet(ctx, gid, sheetID)
	if err != nil {
		return SprintTaskView{}, err
	}

	task, err := s.repo.CreateTask(ctx, sqlc.CreateSprintSheetTaskParams{
		SheetID:        sheet.ID,
		AssigneeUserID: assignee,
		Title:          title,
		Priority:       priority,
		Status:         status,
		EstimatedHours: hours,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return SprintTaskView{}, ErrSheetNotFound
		}
		return SprintTaskView{}, err
	}
	return SprintTaskView{Task: task, Sheet: sheet}, nil
}

// ListSheets devuelve las hojas de sprint del grupo (la más antigua primero).
func (s *SprintService) ListSheets(ctx context.Context, groupID string) ([]sqlc.SocialSprintSheet, error) {
	gid, err := parseGroupUUID(groupID)
	if err != nil {
		return nil, err
	}
	if err := s.requireGroup(ctx, gid); err != nil {
		return nil, err
	}
	return s.repo.ListSheetsByGroup(ctx, gid)
}

func (s *SprintService) ListSprintTasks(ctx context.Context, groupID, userID, status, priority, sheetID string) ([]SprintTaskView, error) {
	gid, err := parseGroupUUID(groupID)
	if err != nil {
		return nil, err
	}
	uid, err := parseMeetingUserUUID(userID)
	if err != nil {
		return nil, err
	}

	status = strings.TrimSpace(status)
	if status != "" {
		if _, ok := validSprintStatuses[status]; !ok {
			return nil, ErrInvalidSprintStatus
		}
	}
	priority = strings.TrimSpace(priority)
	if priority != "" {
		if _, ok := validPriorities[priority]; !ok {
			return nil, ErrInvalidPriority
		}
	}

	if err := s.requireGroup(ctx, gid); err != nil {
		return nil, err
	}
	if err := s.requireMembership(ctx, gid, uid); err != nil {
		return nil, err
	}

	var sid pgtype.UUID
	if strings.TrimSpace(sheetID) != "" {
		sheet, err := s.resolveSheet(ctx, gid, sheetID)
		if err != nil {
			return nil, err
		}
		sid = sheet.ID
	}

	rows, err := s.repo.ListTasksByGroup(ctx, sqlc.ListSprintSheetTasksByGroupParams{
		GroupID:  gid,
		Status:   status,
		Priority: priority,
		SheetID:  sid,
	})
	if err != nil {
		return nil, err
	}

	views := make([]SprintTaskView, 0, len(rows))
	for _, r := range rows {
		views = append(views, SprintTaskView{
			Task: sqlc.SocialSprintSheetTask{
				ID:             r.ID,
				SheetID:        r.SheetID,
				AssigneeUserID: r.AssigneeUserID,
				Title:          r.Title,
				Priority:       r.Priority,
				Status:         r.Status,
				EstimatedHours: r.EstimatedHours,
				CreatedAt:      r.CreatedAt,
				UpdatedAt:      r.UpdatedAt,
			},
			Sheet: sqlc.SocialSprintSheet{
				ID:      r.SheetID,
				GroupID: gid,
				Name:    r.SheetName,
			},
		})
	}
	return views, nil
}

func (s *SprintService) UpdateSprintTask(ctx context.Context, groupID, userID, taskID string, title, assignedTo, priority, status *string, estimatedHours *float64) (SprintTaskView, error) {
	if title == nil && assignedTo == nil && priority == nil && status == nil && estimatedHours == nil {
		return SprintTaskView{}, ErrNothingToPatch
	}

	gid, err := parseGroupUUID(groupID)
	if err != nil {
		return SprintTaskView{}, err
	}
	uid, err := parseMeetingUserUUID(userID)
	if err != nil {
		return SprintTaskView{}, err
	}
	if err := s.requireGroup(ctx, gid); err != nil {
		return SprintTaskView{}, err
	}
	if err := s.requireMembership(ctx, gid, uid); err != nil {
		return SprintTaskView{}, err
	}

	taskID = strings.TrimSpace(taskID)
	if _, err := uuid.Parse(taskID); err != nil {
		return SprintTaskView{}, ErrInvalidSprintTaskID
	}
	var tid pgtype.UUID
	if err := tid.Scan(taskID); err != nil {
		return SprintTaskView{}, ErrInvalidSprintTaskID
	}

	current, err := s.repo.GetTaskByID(ctx, tid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SprintTaskView{}, ErrSprintTaskNotFound
		}
		return SprintTaskView{}, err
	}
	if err := s.requireTaskInGroup(ctx, gid, current.SheetID); err != nil {
		return SprintTaskView{}, err
	}

	arg := sqlc.UpdateSprintSheetTaskParams{
		ID:             current.ID,
		Title:          current.Title,
		AssigneeUserID: current.AssigneeUserID,
		Priority:       current.Priority,
		Status:         current.Status,
		EstimatedHours: current.EstimatedHours,
	}

	if title != nil {
		t, err := validateTitle(*title)
		if err != nil {
			return SprintTaskView{}, err
		}
		arg.Title = t
	}
	if assignedTo != nil {
		a, err := parseRequiredAssignee(*assignedTo)
		if err != nil {
			return SprintTaskView{}, err
		}
		if err := s.requireAssigneeMember(ctx, gid, a); err != nil {
			return SprintTaskView{}, err
		}
		arg.AssigneeUserID = a
	}
	if priority != nil {
		if strings.TrimSpace(*priority) == "" {
			return SprintTaskView{}, ErrInvalidPriority
		}
		p, err := normalizePriority(*priority)
		if err != nil {
			return SprintTaskView{}, err
		}
		arg.Priority = p
	}
	if status != nil {
		if strings.TrimSpace(*status) == "" {
			return SprintTaskView{}, ErrInvalidSprintStatus
		}
		st, err := normalizeSprintStatus(*status)
		if err != nil {
			return SprintTaskView{}, err
		}
		arg.Status = st
	}
	if estimatedHours != nil {
		h, err := parseEstimatedHours(estimatedHours)
		if err != nil {
			return SprintTaskView{}, err
		}
		arg.EstimatedHours = h
	}

	updated, err := s.repo.UpdateTask(ctx, arg)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SprintTaskView{}, ErrSprintTaskNotFound
		}
		return SprintTaskView{}, err
	}

	sheet, err := s.repo.GetSheetByID(ctx, updated.SheetID)
	if err != nil {
		return SprintTaskView{}, err
	}
	return SprintTaskView{Task: updated, Sheet: sheet}, nil
}

func (s *SprintService) DeleteSprintTask(ctx context.Context, groupID, userID, taskID string) (SprintTaskView, error) {
	gid, err := parseGroupUUID(groupID)
	if err != nil {
		return SprintTaskView{}, err
	}
	uid, err := parseMeetingUserUUID(userID)
	if err != nil {
		return SprintTaskView{}, err
	}
	if err := s.requireGroup(ctx, gid); err != nil {
		return SprintTaskView{}, err
	}
	if err := s.requireMembership(ctx, gid, uid); err != nil {
		return SprintTaskView{}, err
	}

	taskID = strings.TrimSpace(taskID)
	if _, err := uuid.Parse(taskID); err != nil {
		return SprintTaskView{}, ErrInvalidSprintTaskID
	}
	var tid pgtype.UUID
	if err := tid.Scan(taskID); err != nil {
		return SprintTaskView{}, ErrInvalidSprintTaskID
	}

	current, err := s.repo.GetTaskByID(ctx, tid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SprintTaskView{}, ErrSprintTaskNotFound
		}
		return SprintTaskView{}, err
	}
	if err := s.requireTaskInGroup(ctx, gid, current.SheetID); err != nil {
		return SprintTaskView{}, err
	}

	deleted, err := s.repo.DeleteTask(ctx, tid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SprintTaskView{}, ErrSprintTaskNotFound
		}
		return SprintTaskView{}, err
	}

	sheet, err := s.repo.GetSheetByID(ctx, deleted.SheetID)
	if err != nil {
		return SprintTaskView{Task: deleted}, err
	}
	return SprintTaskView{Task: deleted, Sheet: sheet}, nil
}
