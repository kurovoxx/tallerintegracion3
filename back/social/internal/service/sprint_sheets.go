package service

import (
	"strconv"
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
	"strings"
	"time"
)

var ErrInvalidSheet = errors.New("nombre o fechas de sprint inválidos")

type SheetPatch struct {
	Name        *string `json:"name"`
	PeriodStart *string `json:"period_start"`
	PeriodEnd   *string `json:"period_end"`
}

func (s *SprintService) sheetAccess(ctx context.Context, groupID, userID string) (pgtype.UUID, error) {
	return s.sheetAccessAs(ctx, groupID, userID, false)
}

// sheetAccessAs valida grupo y membresía; con adminOnly exige además rol admin.
func (s *SprintService) sheetAccessAs(ctx context.Context, groupID, userID string, adminOnly bool) (pgtype.UUID, error) {
	gid, err := parseGroupUUID(groupID)
	if err != nil {
		return gid, err
	}
	uid, err := parseMeetingUserUUID(userID)
	if err != nil {
		return gid, err
	}
	if err := s.requireGroup(ctx, gid); err != nil {
		return gid, err
	}
	if adminOnly {
		return gid, s.requireAdmin(ctx, gid, uid)
	}
	return gid, s.requireMembership(ctx, gid, uid)
}

func sheetDate(raw string) (pgtype.Date, error) {
	d, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return pgtype.Date{}, ErrInvalidSheet
	}
	return pgtype.Date{Time: d, Valid: true}, nil
}

func (s *SprintService) ListSprintSheets(ctx context.Context, groupID, userID string) ([]sqlc.SocialSprintSheet, error) {
	gid, err := s.sheetAccess(ctx, groupID, userID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListSheetsByGroup(ctx, gid)
}

func (s *SprintService) CreateSprintSheet(ctx context.Context, groupID, userID string, patch SheetPatch) (sqlc.SocialSprintSheet, error) {
	gid, err := s.sheetAccessAs(ctx, groupID, userID, true)
	if err != nil {
		return sqlc.SocialSprintSheet{}, err
	}
	if patch.Name == nil || patch.PeriodStart == nil || patch.PeriodEnd == nil {
		return sqlc.SocialSprintSheet{}, ErrInvalidSheet
	}
	value := sqlc.SocialSprintSheet{GroupID: gid}
	if err := applySheetPatch(&value, patch); err != nil {
		return value, err
	}
	return s.repo.CreateSheet(ctx, sqlc.CreateSheetParams{GroupID: gid, Name: value.Name, PeriodStart: value.PeriodStart, PeriodEnd: value.PeriodEnd})
}

func (s *SprintService) UpdateSprintSheet(ctx context.Context, groupID, userID, sheetID string, patch SheetPatch) (sqlc.SocialSprintSheet, error) {
	gid, err := s.sheetAccessAs(ctx, groupID, userID, true)
	if err != nil {
		return sqlc.SocialSprintSheet{}, err
	}
	if strings.TrimSpace(sheetID) == "" {
		return sqlc.SocialSprintSheet{}, ErrInvalidSheetID
	}
	value, err := s.resolveSheet(ctx, gid, sheetID)
	if err != nil {
		return value, err
	}
	if patch.Name == nil && patch.PeriodStart == nil && patch.PeriodEnd == nil {
		return value, ErrInvalidSheet
	}
	if err := applySheetPatch(&value, patch); err != nil {
		return value, err
	}
	value, err = s.repo.UpdateSheet(ctx, sqlc.UpdateSheetParams{ID: value.ID, GroupID: gid, Name: value.Name, PeriodStart: value.PeriodStart, PeriodEnd: value.PeriodEnd})
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrSheetNotFound
	}
	return value, err
}

// DeleteSprintSheet borra la hoja con la MISMA autorización que crear y
// editar (solo admins, vía sheetAccessAs). PostgreSQL resuelve la cascada
// (tareas y daily hours) por ON DELETE CASCADE; no hay deletes manuales.
// Nunca deja al grupo sin hojas: con una sola responde ErrLastSheet.
func (s *SprintService) DeleteSprintSheet(ctx context.Context, groupID, userID, sheetID string) (sqlc.SocialSprintSheet, error) {
	gid, err := s.sheetAccessAs(ctx, groupID, userID, true)
	if err != nil {
		return sqlc.SocialSprintSheet{}, err
	}
	if strings.TrimSpace(sheetID) == "" {
		return sqlc.SocialSprintSheet{}, ErrInvalidSheetID
	}
	value, err := s.resolveSheet(ctx, gid, sheetID)
	if err != nil {
		return value, err
	}
	sheets, err := s.repo.ListSheetsByGroup(ctx, gid)
	if err != nil {
		return sqlc.SocialSprintSheet{}, err
	}
	if len(sheets) <= 1 {
		return sqlc.SocialSprintSheet{}, ErrLastSheet
	}
	deleted, err := s.repo.DeleteSheet(ctx, sqlc.DeleteSheetParams{ID: value.ID, GroupID: gid})
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrSheetNotFound
	}
	return deleted, err
}

func applySheetPatch(value *sqlc.SocialSprintSheet, patch SheetPatch) error {
	if patch.Name != nil {
		value.Name = strings.TrimSpace(*patch.Name)
	}
	if value.Name == "" || len([]rune(value.Name)) > 200 {
		return ErrInvalidSheet
	}
	var err error
	if patch.PeriodStart != nil {
		value.PeriodStart, err = sheetDate(*patch.PeriodStart)
		if err != nil {
			return err
		}
	}
	if patch.PeriodEnd != nil {
		value.PeriodEnd, err = sheetDate(*patch.PeriodEnd)
		if err != nil {
			return err
		}
	}
	// Las hojas históricas sin rango siguen siendo editables por nombre.
	if value.PeriodStart.Valid != value.PeriodEnd.Valid {
		return ErrInvalidSheet
	}
	if value.PeriodStart.Valid && value.PeriodEnd.Time.Before(value.PeriodStart.Time) {
		return ErrInvalidSheet
	}
	return nil
}

// SprintNoticeThresholdDays: desde cuántos días restantes se avisa al grupo.
const SprintNoticeThresholdDays = 3

// SprintRemaining describe el tiempo que le queda a un sprint.
type SprintRemaining struct {
	Sheet         sqlc.SocialSprintSheet
	State         string // not_started | active | ended
	DaysRemaining int    // días hasta period_end (0 = termina hoy); negativo si terminó
	Notify        bool   // true si está activo y quedan <= SprintNoticeThresholdDays
	Message       string
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// SprintTimeRemaining calcula el tiempo restante de cada sprint con rango de
// fechas del grupo, respecto de now (inyectado para poder testear). Cualquier
// miembro puede consultarlo. Hojas históricas sin rango se omiten.
func (s *SprintService) SprintTimeRemaining(ctx context.Context, groupID, userID string, now time.Time) ([]SprintRemaining, error) {
	gid, err := s.sheetAccess(ctx, groupID, userID)
	if err != nil {
		return nil, err
	}
	sheets, err := s.repo.ListSheetsByGroup(ctx, gid)
	if err != nil {
		return nil, err
	}
	n := now.UTC()
	today := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
	out := make([]SprintRemaining, 0, len(sheets))
	for _, sh := range sheets {
		if !sh.PeriodStart.Valid || !sh.PeriodEnd.Valid {
			continue
		}
		start := sh.PeriodStart.Time.UTC()
		end := sh.PeriodEnd.Time.UTC()
		end = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
		start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
		days := int(end.Sub(today).Hours() / 24)
		r := SprintRemaining{Sheet: sh, DaysRemaining: days}
		switch {
		case today.Before(start):
			r.State = "not_started"
			r.Message = "El sprint \"" + sh.Name + "\" aún no comienza."
		case days < 0:
			r.State = "ended"
			r.Message = "El sprint \"" + sh.Name + "\" ya terminó."
		case days == 0:
			r.State = "active"
			r.Notify = true
			r.Message = "El sprint \"" + sh.Name + "\" termina hoy."
		default:
			r.State = "active"
			r.Notify = days <= SprintNoticeThresholdDays
			r.Message = "Al sprint \"" + sh.Name + "\" le " + plural(days, "queda ", "quedan ") + strconv.Itoa(days) + plural(days, " día.", " días.")
		}
		out = append(out, r)
	}
	return out, nil
}
