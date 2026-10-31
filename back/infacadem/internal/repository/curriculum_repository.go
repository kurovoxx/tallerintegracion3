package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
)

// CurriculumRepository es la única capa que toca SQL de academic.curricula.
// Toda lectura/escritura filtra por user_id: un recurso ajeno es invisible
// (el service lo mapea a 404, nunca 403).
type CurriculumRepository struct {
	pool *pgxpool.Pool
}

func NewCurriculumRepository(pool *pgxpool.Pool) *CurriculumRepository {
	return &CurriculumRepository{pool: pool}
}

// Create inserta una malla propia y la devuelve.
func (r *CurriculumRepository) Create(ctx context.Context, userID, name string) (*model.Curriculum, error) {
	var c model.Curriculum
	err := r.pool.QueryRow(ctx, `
		INSERT INTO academic.curricula (user_id, name)
		VALUES ($1, $2)
		RETURNING id, user_id, name, created_at
	`, userID, name).Scan(&c.ID, &c.UserID, &c.Name, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create curriculum: %w", err)
	}
	return &c, nil
}

// GetByID retorna la malla solo si es propia; nil si no existe o es ajena.
func (r *CurriculumRepository) GetByID(ctx context.Context, id, userID string) (*model.Curriculum, error) {
	var c model.Curriculum
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, name, created_at
		FROM academic.curricula
		WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&c.ID, &c.UserID, &c.Name, &c.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get curriculum: %w", err)
	}
	return &c, nil
}

// ListByUser lista solo las mallas propias (nunca null).
func (r *CurriculumRepository) ListByUser(ctx context.Context, userID string) ([]model.Curriculum, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, name, created_at
		FROM academic.curricula
		WHERE user_id = $1
		ORDER BY created_at DESC, id DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list curricula: %w", err)
	}
	defer rows.Close()
	out := make([]model.Curriculum, 0)
	for rows.Next() {
		var c model.Curriculum
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("list curricula: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list curricula: %w", err)
	}
	return out, nil
}

// UpdateName renombra una malla propia; nil si no existe o es ajena.
func (r *CurriculumRepository) UpdateName(ctx context.Context, id, userID, name string) (*model.Curriculum, error) {
	var c model.Curriculum
	err := r.pool.QueryRow(ctx, `
		UPDATE academic.curricula
		SET name = $3
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, name, created_at
	`, id, userID, name).Scan(&c.ID, &c.UserID, &c.Name, &c.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("update curriculum: %w", err)
	}
	return &c, nil
}

// Delete borra una malla propia; subjects y schedules caen por
// ON DELETE CASCADE. Retorna false si no existe o es ajena.
func (r *CurriculumRepository) Delete(ctx context.Context, id, userID string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM academic.curricula
		WHERE id = $1 AND user_id = $2
	`, id, userID)
	if err != nil {
		return false, fmt.Errorf("delete curriculum: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
