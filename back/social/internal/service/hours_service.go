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
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
)

var (
	ErrInvalidLogDate = errors.New("invalid log_date: must be YYYY-MM-DD")
	ErrInvalidHours   = errors.New("invalid hours: must be a number >= 0")
)

type HoursService struct {
	repo *repository.HoursRepository
}

func NewHoursService(repo *repository.HoursRepository) *HoursService {
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

func (s *HoursService) requireTask(ctx context.Context, tid pgtype.UUID) error {
	if _, err := s.repo.GetTaskByID(ctx, tid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSprintTaskNotFound
		}
		return err
	}
	return nil
}

func (s *HoursService) LogHours(ctx context.Context, taskID, logDate string, hours float64) (sqlc.SocialSprintSheetDailyHour, bool, error) {
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
	if err := s.requireTask(ctx, tid); err != nil {
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

func (s *HoursService) ListHours(ctx context.Context, taskID, from, to string) ([]sqlc.SocialSprintSheetDailyHour, float64, error) {
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
	if err := s.requireTask(ctx, tid); err != nil {
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
