package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
)

var (
	ErrInvalidLogDate = errors.New("invalid log_date: must be YYYY-MM-DD")
	ErrInvalidHours   = errors.New("invalid hours: must be a number >= 0")
)

type HoursService struct {
	repo HoursRepo
}

// HoursRepo es la porción de persistencia que usa HoursService.
// *repository.HoursRepository es la implementación real (Postgres);
// MemoryTasksStore, la de tests.
type HoursRepo interface {
	GetTaskByID(ctx context.Context, id pgtype.UUID) (sqlc.SocialSprintSheetTask, error)
	GetSheetByID(ctx context.Context, id pgtype.UUID) (sqlc.SocialSprintSheet, error)
	IsMember(ctx context.Context, groupID, userID pgtype.UUID) (bool, error)
	GetByTaskAndDate(ctx context.Context, arg sqlc.GetDailyHoursByTaskAndDateParams) (sqlc.SocialSprintSheetDailyHour, error)
	ListByTask(ctx context.Context, arg sqlc.ListDailyHoursByTaskParams) ([]sqlc.SocialSprintSheetDailyHour, error)
	Create(ctx context.Context, arg sqlc.CreateDailyHoursParams) (sqlc.SocialSprintSheetDailyHour, error)
	Update(ctx context.Context, arg sqlc.UpdateDailyHoursParams) (sqlc.SocialSprintSheetDailyHour, error)
}

func NewHoursService(repo HoursRepo) *HoursService {
	return &HoursService{
		repo: repo,
	}
}

func parseTaskUUID(taskID string) (pgtype.UUID, error) {
	var tid pgtype.UUID
	if _, err := uuid.Parse(strings.TrimSpace(taskID)); err != nil {
		return tid, ErrInvalidSprintTaskID
	}
	if err := tid.Scan(strings.TrimSpace(taskID)); err != nil {
		return tid, ErrInvalidSprintTaskID
	}
	return tid, nil
}

func parseLogDate(logDate string) (pgtype.Date, error) {
	var d pgtype.Date
	logDate = strings.TrimSpace(logDate)
	if logDate == "" {
		return d, ErrInvalidLogDate
	}
	if _, err := time.Parse("2006-01-02", logDate); err != nil {
		return d, ErrInvalidLogDate
	}
	if err := d.Scan(logDate); err != nil {
		return d, ErrInvalidLogDate
	}
	return d, nil
}

func parseOptionalDate(value string) (pgtype.Date, error) {
	var d pgtype.Date
	value = strings.TrimSpace(value)
	if value == "" {
		return d, nil
	}
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return d, ErrInvalidLogDate
	}
	if err := d.Scan(value); err != nil {
		return d, ErrInvalidLogDate
	}
	return d, nil
}

func parseHours(hours float64) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if hours < 0 {
		return n, ErrInvalidHours
	}
	if err := n.Scan(strconv.FormatFloat(hours, 'f', 2, 64)); err != nil {
		return n, ErrInvalidHours
	}
	return n, nil
}

// requireTaskGroup resuelve el grupo de la tarea (tarea → hoja → grupo).
func (s *HoursService) requireTaskGroup(ctx context.Context, tid pgtype.UUID) (pgtype.UUID, error) {
	var gid pgtype.UUID
	task, err := s.repo.GetTaskByID(ctx, tid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gid, ErrSprintTaskNotFound
		}
		return gid, err
	}
	sheet, err := s.repo.GetSheetByID(ctx, task.SheetID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gid, ErrSprintTaskNotFound
		}
		return gid, err
	}
	return sheet.GroupID, nil
}

// requireMembership exige que el usuario pertenezca al grupo (403 si no).
func (s *HoursService) requireMembership(ctx context.Context, gid, uid pgtype.UUID) error {
	isMember, err := s.repo.IsMember(ctx, gid, uid)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrForbidden
	}
	return nil
}

func (s *HoursService) LogHours(ctx context.Context, userID, taskID, logDate string, hours float64) (sqlc.SocialSprintSheetDailyHour, bool, error) {
	uid, err := parseMeetingUserUUID(userID)
	if err != nil {
		return sqlc.SocialSprintSheetDailyHour{}, false, err
	}
	tid, err := parseTaskUUID(taskID)
	if err != nil {
		return sqlc.SocialSprintSheetDailyHour{}, false, err
	}
	date, err := parseLogDate(logDate)
	if err != nil {
		return sqlc.SocialSprintSheetDailyHour{}, false, err
	}
	amount, err := parseHours(hours)
	if err != nil {
		return sqlc.SocialSprintSheetDailyHour{}, false, err
	}
	gid, err := s.requireTaskGroup(ctx, tid)
	if err != nil {
		return sqlc.SocialSprintSheetDailyHour{}, false, err
	}
	if err := s.requireMembership(ctx, gid, uid); err != nil {
		return sqlc.SocialSprintSheetDailyHour{}, false, err
	}

	existing, err := s.repo.GetByTaskAndDate(ctx, sqlc.GetDailyHoursByTaskAndDateParams{
		TaskID:  tid,
		LogDate: date,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return sqlc.SocialSprintSheetDailyHour{}, false, err
	}
	if err == nil {
		updated, err := s.repo.Update(ctx, sqlc.UpdateDailyHoursParams{
			ID:    existing.ID,
			Hours: amount,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return sqlc.SocialSprintSheetDailyHour{}, false, ErrSprintTaskNotFound
			}
			return sqlc.SocialSprintSheetDailyHour{}, false, err
		}
		return updated, false, nil
	}

	created, err := s.repo.Create(ctx, sqlc.CreateDailyHoursParams{
		TaskID:  tid,
		LogDate: date,
		Hours:   amount,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return sqlc.SocialSprintSheetDailyHour{}, false, ErrSprintTaskNotFound
		}
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			existing, gerr := s.repo.GetByTaskAndDate(ctx, sqlc.GetDailyHoursByTaskAndDateParams{
				TaskID:  tid,
				LogDate: date,
			})
			if gerr != nil {
				return sqlc.SocialSprintSheetDailyHour{}, false, gerr
			}
			updated, uerr := s.repo.Update(ctx, sqlc.UpdateDailyHoursParams{
				ID:    existing.ID,
				Hours: amount,
			})
			if uerr != nil {
				return sqlc.SocialSprintSheetDailyHour{}, false, uerr
			}
			return updated, false, nil
		}
		return sqlc.SocialSprintSheetDailyHour{}, false, err
	}
	return created, true, nil
}

func (s *HoursService) ListHours(ctx context.Context, userID, taskID, from, to string) ([]sqlc.SocialSprintSheetDailyHour, float64, error) {
	uid, err := parseMeetingUserUUID(userID)
	if err != nil {
		return nil, 0, err
	}
	tid, err := parseTaskUUID(taskID)
	if err != nil {
		return nil, 0, err
	}
	fromDate, err := parseOptionalDate(from)
	if err != nil {
		return nil, 0, err
	}
	toDate, err := parseOptionalDate(to)
	if err != nil {
		return nil, 0, err
	}
	gid, err := s.requireTaskGroup(ctx, tid)
	if err != nil {
		return nil, 0, err
	}
	if err := s.requireMembership(ctx, gid, uid); err != nil {
		return nil, 0, err
	}

	entries, err := s.repo.ListByTask(ctx, sqlc.ListDailyHoursByTaskParams{
		TaskID: tid,
		From:   fromDate,
		To:     toDate,
	})
	if err != nil {
		return nil, 0, err
	}
	if entries == nil {
		entries = []sqlc.SocialSprintSheetDailyHour{}
	}

	var total float64
	for _, e := range entries {
		if f, ferr := e.Hours.Float64Value(); ferr == nil && f.Valid {
			total += f.Float64
		}
	}
	return entries, total, nil
}
