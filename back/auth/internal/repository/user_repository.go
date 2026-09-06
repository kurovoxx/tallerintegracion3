package repository

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
)

// UserRepository es la única capa que toca SQL (sqlc+pgx) según masterprompt 2.
// Por ahora SQL explícito; sqlc generará código type-safe cuando se añada db/queries/*.sql.
type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// CreateUser inserta en identity.users con hash bcrypt ya calculado y crea profile en la misma transacción.
// display_name es obligatorio (no derivado) y visibility default private. Campos opcionales nulables se persisten NULL si nil.
// Rollback atómico si falla alguna.
func (r *UserRepository) CreateUser(ctx context.Context, email, passwordHash, role, displayName string, photoURL, phone, institution, description, visibility *string) (*model.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return nil, fmt.Errorf("invalid_display_name: display_name vacío")
	}
	if len(displayName) > 255 {
		displayName = displayName[:255]
	}
	// Normalizar opcionales
	if photoURL != nil {
		v := strings.TrimSpace(*photoURL)
		if v == "" {
			photoURL = nil
		} else {
			*photoURL = v
		}
	}
	if phone != nil {
		v := strings.TrimSpace(*phone)
		if v == "" {
			phone = nil
		} else {
			*phone = v
		}
	}
	if institution != nil {
		v := strings.TrimSpace(*institution)
		if v == "" {
			institution = nil
		} else {
			*institution = v
		}
	}
	if description != nil {
		v := strings.TrimSpace(*description)
		if v == "" {
			description = nil
		} else {
			*description = v
		}
	}
	visibilityVal := "private"
	if visibility != nil {
		v := strings.TrimSpace(*visibility)
		if v == "public" || v == "private" {
			visibilityVal = v
		} else {
			return nil, fmt.Errorf("invalid_visibility: %s", v)
		}
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		// Rollback si no se hizo commit (no error si ya commiteado)
		_ = tx.Rollback(ctx)
	}()

	var u model.User
	queryUser := `
		INSERT INTO identity.users (email, password_hash, role)
		VALUES ($1, $2, $3)
		RETURNING id, email, role, created_at
	`
	err = tx.QueryRow(ctx, queryUser, email, passwordHash, role).Scan(&u.ID, &u.Email, &u.Role, &u.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, fmt.Errorf("email_taken: %w", err)
		}
		msg := err.Error()
		if strings.Contains(msg, "23505") || strings.Contains(msg, "users_email_key") || strings.Contains(msg, "duplicate key") || strings.Contains(msg, "already exists") {
			return nil, fmt.Errorf("email_taken: %w", err)
		}
		return nil, fmt.Errorf("create user: %w", err)
	}

	// Insert profile atómico — si falla, rollback user también. Campos nulables soportan NULL.
	_, err = tx.Exec(ctx, `
		INSERT INTO identity.profiles (user_id, display_name, photo_url, phone, institution, description, visibility)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, u.ID, displayName, photoURL, phone, institution, description, visibilityVal)
	if err != nil {
		log.Printf("create profile failed for user %s (%s): %v", u.ID, displayName, err)
		return nil, fmt.Errorf("create profile: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		log.Printf("commit failed for user %s: %v", u.ID, err)
		return nil, fmt.Errorf("commit: %w", err)
	}
	return &u, nil
}

// GetByEmail busca usuario por email (referencia lógica, no FK cruzada).
// Usado para validación previa opcional y para login futuro.
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var u model.User
	query := `SELECT id, email, password_hash, role, created_at FROM identity.users WHERE email = $1`
	err := r.pool.QueryRow(ctx, query, email).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get by email: %w", err)
	}
	return &u, nil
}

// ExistsByEmail verifica unicidad sin traer todo el row.
func (r *UserRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM identity.users WHERE email=$1)`, email).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}
