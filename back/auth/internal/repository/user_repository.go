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
// display_name es obligatorio para identity.profiles (NOT NULL) — si Register no lo recibe, se deriva del email en Service.
// visibility default private. Campos opcionales nulables se persisten NULL si nil.
// Rollback atómico si falla alguna.
func (r *UserRepository) CreateUser(ctx context.Context, email, passwordHash, displayName string, photoURL, phone, institution, description, visibility *string) (*model.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return nil, fmt.Errorf("invalid_display_name: display_name vacío")
	}
	if len(displayName) > 100 {
		// agentSql.md:40 profiles.display_name varchar(100), no 255
		displayName = displayName[:100]
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

	// El DEFAULT de PostgreSQL genera public_id. Una colisión (probabilidad
	// 1/2^60) aborta la transacción, así que se reintenta la transacción
	// completa; nunca se reutiliza el UUID interno como identificador público.
	const maxPublicIDAttempts = 5
	var lastErr error
	for attempt := 0; attempt < maxPublicIDAttempts; attempt++ {
		u, err := r.createUserOnce(ctx, email, passwordHash, displayName, photoURL, phone, institution, description, visibilityVal)
		if err == nil {
			return u, nil
		}
		if !errors.Is(err, errPublicIDCollision) {
			return nil, err
		}
		lastErr = err
	}
	return nil, fmt.Errorf("create user: no se pudo generar public_id único tras %d intentos: %w", maxPublicIDAttempts, lastErr)
}

// errPublicIDCollision señala que la UNIQUE users_public_id_key chocó y la
// transacción debe repetirse con un identificador nuevo.
var errPublicIDCollision = errors.New("public_id collision")

func (r *UserRepository) createUserOnce(ctx context.Context, email, passwordHash, displayName string, photoURL, phone, institution, description *string, visibilityVal string) (*model.User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var u model.User
	queryUser := `
		INSERT INTO identity.users (email, password_hash)
		VALUES ($1, $2)
		RETURNING id, email, public_id, created_at
	`
	err = tx.QueryRow(ctx, queryUser, email, passwordHash).Scan(&u.ID, &u.Email, &u.PublicID, &u.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if pgErr.ConstraintName == "users_public_id_key" {
				return nil, fmt.Errorf("%w: %v", errPublicIDCollision, err)
			}
			return nil, fmt.Errorf("email_taken: %w", err)
		}
		msg := err.Error()
		if strings.Contains(msg, "users_public_id_key") {
			return nil, fmt.Errorf("%w: %v", errPublicIDCollision, err)
		}
		if strings.Contains(msg, "23505") || strings.Contains(msg, "users_email_key") || strings.Contains(msg, "duplicate key") || strings.Contains(msg, "already exists") {
			return nil, fmt.Errorf("email_taken: %w", err)
		}
		return nil, fmt.Errorf("create user: %w", err)
	}

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

// GetByEmail busca usuario por email.
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var u model.User
	query := `SELECT id, email, password_hash, public_id, created_at FROM identity.users WHERE email = $1`
	err := r.pool.QueryRow(ctx, query, email).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.PublicID, &u.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get by email: %w", err)
	}
	return &u, nil
}

// GetIDByPublicID resuelve identity.users.public_id al UUID interno. Solo lo
// consumen llamadas servicio-a-servicio autorizadas; retorna ("", nil) si el
// identificador no existe para que el llamador decida NotFound.
func (r *UserRepository) GetIDByPublicID(ctx context.Context, publicID string) (string, error) {
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return "", nil
	}
	var userID string
	err := r.pool.QueryRow(ctx, `SELECT id FROM identity.users WHERE public_id = $1`, publicID).Scan(&userID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("get user by public_id: %w", err)
	}
	return userID, nil
}

// DeleteByID elimina el usuario y deja que PostgreSQL aplique las cascadas
// declaradas en Identity (perfil, refresh tokens, password resets y OAuth).
// Es idempotente para que el coordinador pueda repetirlo si perdió la respuesta
// local después de que el commit ya ocurrió.
func (r *UserRepository) DeleteByID(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return fmt.Errorf("user_id vacío")
	}
	if _, err := r.pool.Exec(ctx, `DELETE FROM identity.users WHERE id = $1`, userID); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
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

// GetEmailByID retorna el email de identity.users por id.
// Retorna ("", nil) si no existe — el Service decide cómo presentarlo.
func (r *UserRepository) GetEmailByID(ctx context.Context, userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", fmt.Errorf("user_id vacío")
	}
	var email string
	err := r.pool.QueryRow(ctx, `SELECT email FROM identity.users WHERE id = $1`, userID).Scan(&email)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("get email by id: %w", err)
	}
	return email, nil
}

// GetPublicByIDs resuelve datos públicos mínimos (email + display_name +
// miniatura de foto) para una lista de user_ids. Una sola query (sin N+1):
// join users + profiles. IDs inválidos o inexistentes simplemente no aparecen
// en el resultado. Para perfiles antiguos con URL remota se usa esa URL como
// miniatura; las data URLs completas sin miniatura se omiten para no mover
// cientos de KB entre servicios.
func (r *UserRepository) GetPublicByIDs(ctx context.Context, userIDs []string) ([]model.PublicUser, error) {
	clean := make([]string, 0, len(userIDs))
	seen := make(map[string]struct{}, len(userIDs))
	for _, id := range userIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		clean = append(clean, id)
	}
	if len(clean) == 0 {
		return []model.PublicUser{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.email, p.display_name,
		       COALESCE(
		           p.photo_thumb_url,
		           CASE
		               WHEN p.photo_url IS NOT NULL AND length(p.photo_url) <= 500
		               THEN p.photo_url
		           END
		       ) AS photo_thumb_url
		FROM identity.users u
		LEFT JOIN identity.profiles p ON p.user_id = u.id
		WHERE u.id = ANY($1::uuid[])`, clean)
	if err != nil {
		return nil, fmt.Errorf("get public by ids: %w", err)
	}
	defer rows.Close()
	out := make([]model.PublicUser, 0, len(clean))
	for rows.Next() {
		var pu model.PublicUser
		if err := rows.Scan(&pu.UserID, &pu.Email, &pu.DisplayName, &pu.PhotoThumbURL); err != nil {
			return nil, fmt.Errorf("get public by ids: %w", err)
		}
		out = append(out, pu)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get public by ids: %w", err)
	}
	return out, nil
}
