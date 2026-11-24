package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
)

// GroupInvitationRepository gestiona invitaciones a grupos en
// social.group_invitations.
type GroupInvitationRepository struct {
	pool *pgxpool.Pool
}

var ErrInvitationAlreadyPending = errors.New("invitación ya pendiente para este usuario en este grupo")

func NewGroupInvitationRepository(pool *pgxpool.Pool) *GroupInvitationRepository {
	return &GroupInvitationRepository{pool: pool}
}

// parseUUID convierte un string a pgtype.UUID.
// CreateInvitation crea una invitación de grupo. Devuelve false si ya existe
// una invitación pendiente para ese usuario en ese grupo.
func (r *GroupInvitationRepository) CreateInvitation(ctx context.Context, groupID, invitedUserID, invitedByUserID pgtype.UUID) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO social.group_invitations (group_id, invited_user_id, invited_by_user_id, status)
		VALUES ($1, $2, $3, 'pending')
		RETURNING id::text
	`, groupID, invitedUserID, invitedByUserID).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return "", ErrInvitationAlreadyPending
		}
		return "", fmt.Errorf("create invitation: %w", err)
	}
	return id, nil
}

// AcceptInvitation cambia el status a 'accepted' y añade al invitado como
// member del grupo. Devuelve false si la invitación no existe o no está
// pendiente.
func (r *GroupInvitationRepository) AcceptInvitation(ctx context.Context, id, userID pgtype.UUID) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Verificar que la invitación exista, esté pendiente y pertenezca al usuario.
	var groupID pgtype.UUID
	err = tx.QueryRow(ctx, `
		SELECT gi.group_id
		FROM social.group_invitations gi
		WHERE gi.id = $1 AND gi.invited_user_id = $2 AND gi.status = 'pending'
		FOR UPDATE
	`, id, userID).Scan(&groupID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil // no existe o no está pendiente
		}
		return false, fmt.Errorf("verify invitation: %w", err)
	}

	// Cambiar status a accepted.
	if _, err := tx.Exec(ctx, `
		UPDATE social.group_invitations
		SET status = 'accepted', responded_at = now()
		WHERE id = $1
	`, id); err != nil {
		return false, fmt.Errorf("update invitation: %w", err)
	}

	// Insertar al usuario como member del grupo (idempotente).
	if _, err := tx.Exec(ctx, `
		INSERT INTO social.group_memberships (group_id, user_id, role)
		VALUES ($1, $2, 'member')
		ON CONFLICT (group_id, user_id) DO NOTHING
	`, groupID, userID); err != nil {
		return false, fmt.Errorf("insert membership: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}
	return true, nil
}

// RejectInvitation cambia el status a 'rejected'. Devuelve false si la
// invitación no existe o no está pendiente.
func (r *GroupInvitationRepository) RejectInvitation(ctx context.Context, id, userID pgtype.UUID) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE social.group_invitations
		SET status = 'rejected', responded_at = now()
		WHERE id = $1 AND invited_user_id = $2 AND status = 'pending'
	`, id, userID)
	if err != nil {
		return false, fmt.Errorf("reject invitation: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// ListPendingInvitations devuelve las invitaciones pendientes para un usuario,
// con los datos del grupo y del invitador.
func (r *GroupInvitationRepository) ListPendingInvitations(ctx context.Context, userID pgtype.UUID) ([]model.GroupInvitationView, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT gi.id::text, gi.group_id::text, g.name, gi.invited_by_user_id::text,
		       p.display_name, gi.created_at
		FROM social.group_invitations gi
		JOIN social.groups g ON g.id = gi.group_id
		LEFT JOIN identity.profiles p ON p.user_id = gi.invited_by_user_id
		WHERE gi.invited_user_id = $1 AND gi.status = 'pending'
		ORDER BY gi.created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list invitations: %w", err)
	}
	defer rows.Close()

	out := []model.GroupInvitationView{}
	for rows.Next() {
		var inv model.GroupInvitationView
		if err := rows.Scan(
			&inv.ID, &inv.GroupID, &inv.GroupName,
			&inv.InvitedByUserID, &inv.InvitedByName, &inv.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan invitation: %w", err)
		}
		inv.Status = "pending"
		out = append(out, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	if out == nil {
		out = []model.GroupInvitationView{}
	}
	return out, nil
}

// IsAdmin verifica que el usuario sea admin del grupo.
func (r *GroupInvitationRepository) IsAdmin(ctx context.Context, groupID, userID pgtype.UUID) (bool, error) {
	var role string
	err := r.pool.QueryRow(ctx, `
		SELECT role FROM social.group_memberships
		WHERE group_id = $1 AND user_id = $2
	`, groupID, userID).Scan(&role)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return role == "admin", nil
}

// IsMember verifica que el usuario sea miembro del grupo.
func (r *GroupInvitationRepository) IsMember(ctx context.Context, groupID, userID pgtype.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM social.group_memberships WHERE group_id = $1 AND user_id = $2)
	`, groupID, userID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// IsBanned verifica que el usuario esté baneado del grupo.
func (r *GroupInvitationRepository) IsBanned(ctx context.Context, groupID, userID pgtype.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM social.banned_users WHERE group_id = $1 AND user_id = $2)
	`, groupID, userID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// IsFollowing verifica que quien invita siga al destinatario.
func (r *GroupInvitationRepository) IsFollowing(ctx context.Context, inviterID, invitedUserID pgtype.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM identity.user_follows
			WHERE follower_id = $1 AND following_id = $2
		)
	`, inviterID, invitedUserID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check followed user: %w", err)
	}
	return exists, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "group_invitations_unique_pending")
}
