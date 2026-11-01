package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MemberUserIDs expone los user_id de los miembros de un grupo. La fuente
// autorizada es Social (quien resuelve membresía); en producción la implementan
// SocialHTTPAdapter y SocialGRPCAdapter, sin cambios en Social.
type MemberUserIDs interface {
	ListMemberUserIDs(ctx context.Context, groupID string) ([]string, error)
}

// GoogleDriveEmailStore lee el correo de la cuenta Google Drive conectada por
// usuario (identity.oauth_connections.external_account_email). Lee ÚNICAMENTE
// ese campo: nunca access_token, refresh_token ni secretos.
type GoogleDriveEmailStore interface {
	GoogleDriveEmails(ctx context.Context, userIDs []string) (map[string]string, error)
}

// DriveEmailDirectory resuelve los correos de la cuenta Google Drive de los
// miembros de un grupo, para conceder los permisos Drive `restricted` a las
// identidades Google realmente conectadas (no al correo de registro en Sigma
// Academy).
type DriveEmailDirectory interface {
	// ListMemberDriveEmails devuelve (correosGoogle, miembrosSinDrive, error).
	// "miembrosSinDrive" cuenta a los miembros SIN conexión Drive vigente: no
	// se les concede permiso (no tienen Drive) y NO se sustituye su correo.
	ListMemberDriveEmails(ctx context.Context, groupID string) ([]string, int, error)
}

// driveEmailDirectory compone Social (user_id de miembros) con la lectura de
// identity.oauth_connections (correo Google). No conoce roles ni permisos: solo
// mapea user_id -> correo de la cuenta Drive conectada.
type driveEmailDirectory struct {
	members MemberUserIDs
	store   GoogleDriveEmailStore
}

// NewDriveEmailDirectory crea el resolvedor. Cualquier dependencia nil hace que
// las llamadas fallen cerrado (ErrInternalDatabase).
func NewDriveEmailDirectory(members MemberUserIDs, store GoogleDriveEmailStore) DriveEmailDirectory {
	return &driveEmailDirectory{members: members, store: store}
}

func (d *driveEmailDirectory) ListMemberDriveEmails(ctx context.Context, groupID string) ([]string, int, error) {
	if d.members == nil || d.store == nil {
		return nil, 0, ErrInternalDatabase
	}
	ids, err := d.members.ListMemberUserIDs(ctx, groupID)
	if err != nil {
		return nil, 0, err
	}
	if len(ids) == 0 {
		return nil, 0, nil
	}
	byUser, err := d.store.GoogleDriveEmails(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	emails := make([]string, 0, len(ids))
	withoutDrive := 0
	for _, id := range ids {
		email := strings.TrimSpace(byUser[id])
		if email == "" {
			// Miembro sin conexión Drive: no hay identidad Google a la que
			// conceder; NO se cae al correo de registro.
			withoutDrive++
			continue
		}
		emails = append(emails, email)
	}
	return emails, withoutDrive, nil
}

// PGDriveEmailStore implementa GoogleDriveEmailStore sobre PostgreSQL
// (identity.oauth_connections). Solo lee external_account_email de conexiones
// google_drive vigentes (revoked_at IS NULL).
type PGDriveEmailStore struct {
	pool *pgxpool.Pool
}

func NewPGDriveEmailStore(pool *pgxpool.Pool) *PGDriveEmailStore {
	return &PGDriveEmailStore{pool: pool}
}

func (s *PGDriveEmailStore) GoogleDriveEmails(ctx context.Context, userIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(userIDs))
	if s.pool == nil || len(userIDs) == 0 {
		return out, nil
	}
	clean := make([]string, 0, len(userIDs))
	for _, id := range userIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, err := uuid.Parse(id); err != nil {
			continue
		}
		clean = append(clean, id)
	}
	if len(clean) == 0 {
		return out, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT user_id, external_account_email
		  FROM identity.oauth_connections
		 WHERE provider = 'google_drive'
		   AND revoked_at IS NULL
		   AND external_account_email IS NOT NULL
		   AND external_account_email <> ''
		   AND user_id = ANY($1::uuid[])`, clean)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var userID, email string
		if err := rows.Scan(&userID, &email); err != nil {
			return nil, err
		}
		out[userID] = email
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
