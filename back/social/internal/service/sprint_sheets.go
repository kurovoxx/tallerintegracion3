package service

import (
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
	gid, err := s.sheetAccess(ctx, groupID, userID)
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
	gid, err := s.sheetAccess(ctx, groupID, userID)
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
// editar (solo membresía, vía sheetAccess). PostgreSQL resuelve la cascada
// (tareas y daily hours) por ON DELETE CASCADE; no hay deletes manuales.
// Nunca deja al grupo sin hojas: con una sola responde ErrLastSheet.
func (s *SprintService) DeleteSprintSheet(ctx context.Context, groupID, userID, sheetID string) (sqlc.SocialSprintSheet, error) {
	gid, err := s.sheetAccess(ctx, groupID, userID)
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
