package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
)

// ContentRepository es la única capa que toca SQL de academic.contents.
// La propiedad se resuelve vía subjects → curricula (user_id):
// recurso ajeno = invisible (nil), nunca error distinguible.
type ContentRepository struct {
	pool *pgxpool.Pool
}

func NewContentRepository(pool *pgxpool.Pool) *ContentRepository {
	return &ContentRepository{pool: pool}
}

const contentColumns = `id, subject_id, title, body, position`

func scanContent(row pgx.Row) (*model.Content, error) {
	var c model.Content
	if err := row.Scan(&c.ID, &c.SubjectID, &c.Title, &c.Body, &c.Position); err != nil {
		return nil, err
	}
	return &c, nil
}

// Create inserta un contenido solo si la materia es propia (atómico);
// nil si la materia no existe o es ajena.
func (r *ContentRepository) Create(ctx context.Context, userID, subjectID, title string, body *string, position int) (*model.Content, error) {
	var b any
	if body != nil {
		b = *body
	}
	c, err := scanContent(r.pool.QueryRow(ctx, `
		INSERT INTO academic.contents (subject_id, title, body, position)
		SELECT $1::uuid, $3, $4, $5
		WHERE EXISTS (
			SELECT 1 FROM academic.subjects s
			JOIN academic.curricula c ON c.id = s.curriculum_id
			WHERE s.id = $1::uuid AND c.user_id = $2
		)
		RETURNING `+contentColumns+`
	`, subjectID, userID, title, b, position))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("create content: %w", err)
	}
	return c, nil
}

// GetByID retorna el contenido solo si su materia es propia.
func (r *ContentRepository) GetByID(ctx context.Context, id, userID string) (*model.Content, error) {
	c, err := scanContent(r.pool.QueryRow(ctx, `
		SELECT `+qualify(contentColumns, "t")+`
		FROM academic.contents t
		JOIN academic.subjects s ON s.id = t.subject_id
		JOIN academic.curricula c ON c.id = s.curriculum_id
		WHERE t.id = $1 AND c.user_id = $2
	`, id, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get content: %w", err)
	}
	return c, nil
}

// ListBySubject lista los contenidos ordenados por posición si la materia
// es propia. owned=false si no existe o es ajena (service → 404).
func (r *ContentRepository) ListBySubject(ctx context.Context, subjectID, userID string) (list []model.Content, owned bool, err error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM academic.subjects s
			JOIN academic.curricula c ON c.id = s.curriculum_id
			WHERE s.id = $1 AND c.user_id = $2
		)
	`, subjectID, userID).Scan(&exists); err != nil {
		return nil, false, fmt.Errorf("check subject ownership: %w", err)
	}
	if !exists {
		return nil, false, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+contentColumns+`
		FROM academic.contents
		WHERE subject_id = $1
		ORDER BY position ASC, id ASC
	`, subjectID)
	if err != nil {
		return nil, true, fmt.Errorf("list contents: %w", err)
	}
	defer rows.Close()
	out := make([]model.Content, 0)
	for rows.Next() {
		var c model.Content
		if err := rows.Scan(&c.ID, &c.SubjectID, &c.Title, &c.Body, &c.Position); err != nil {
			return nil, true, fmt.Errorf("list contents: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, true, fmt.Errorf("list contents: %w", err)
	}
	return out, true, nil
}

// ContentPatch agrupa edición parcial (nil = no tocar, ClearBody = NULL).
type ContentPatch struct {
	Title     *string
	Body      *string
	Position  *int
	ClearBody bool
}

// Update aplica PATCH parcial solo si la materia es propia;
// nil si no existe o es ajena.
func (r *ContentRepository) Update(ctx context.Context, id, userID string, p ContentPatch) (*model.Content, error) {
	set := make([]string, 0, 3)
	args := []any{id, userID}
	add := func(expr string, v any) {
		args = append(args, v)
		set = append(set, fmt.Sprintf(expr, len(args)))
	}
	if p.Title != nil {
		add("title = $%d", *p.Title)
	}
	if p.ClearBody {
		set = append(set, "body = NULL")
	} else if p.Body != nil {
		add("body = $%d", *p.Body)
	}
	if p.Position != nil {
		add("position = $%d", *p.Position)
	}
	if len(set) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}
	query := fmt.Sprintf(`
		UPDATE academic.contents t
		SET %s
		WHERE t.id = $1 AND EXISTS (
			SELECT 1 FROM academic.subjects s
			JOIN academic.curricula c ON c.id = s.curriculum_id
			WHERE s.id = t.subject_id AND c.user_id = $2
		)
		RETURNING `+qualify(contentColumns, "t"), joinSet(set))
	c, err := scanContent(r.pool.QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("update content: %w", err)
	}
	return c, nil
}

// Delete borra un contenido propio. False si no existe o es ajeno.
func (r *ContentRepository) Delete(ctx context.Context, id, userID string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM academic.contents t
		WHERE t.id = $1 AND EXISTS (
			SELECT 1 FROM academic.subjects s
			JOIN academic.curricula c ON c.id = s.curriculum_id
			WHERE s.id = t.subject_id AND c.user_id = $2
		)
	`, id, userID)
	if err != nil {
		return false, fmt.Errorf("delete content: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
