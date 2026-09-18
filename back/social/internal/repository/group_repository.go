package repository

import (
	"context"
	"errors" // <--- Añade esta línea aquí
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
)

// GroupRepository es la única capa que toca SQL para social.groups y
// social.group_memberships.
type GroupRepository struct {
	pool *pgxpool.Pool
}

func NewGroupRepository(pool *pgxpool.Pool) *GroupRepository {
	return &GroupRepository{pool: pool}
}



// CreateWithOwner inserta el grupo y, en la misma transacción, la membresía
// del creador como "admin". Si cualquiera de las dos falla, no queda ni
// grupo ni membresía huérfana (rollback).
func (r *GroupRepository) CreateWithOwner(ctx context.Context, name string, description *string, ownerUserID string) (*model.Group, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) // no-op si ya se hizo Commit

	var g model.Group
	insertGroup := `
		INSERT INTO social.groups (name, description, owner_user_id)
		VALUES ($1, $2, $3)
		RETURNING id, name, description, owner_user_id, notes_restricted_to_staff, invite_token, created_at
	`
	err = tx.QueryRow(ctx, insertGroup, name, description, ownerUserID).Scan(
		&g.ID, &g.Name, &g.Description, &g.OwnerUserID, &g.NotesRestrictedToStaff, &g.InviteToken, &g.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert group: %w", err)
	}

	insertMembership := `
		INSERT INTO social.group_memberships (group_id, user_id, role)
		VALUES ($1, $2, $3)
	`
	if _, err := tx.Exec(ctx, insertMembership, g.ID, ownerUserID, model.RoleAdmin); err != nil {
		return nil, fmt.Errorf("insert owner membership: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return &g, nil
}

func (r *GroupRepository) GetByID(ctx context.Context, id string) (*model.Group, error) {
	var g model.Group
	query := `
		SELECT id, name, description, owner_user_id, notes_restricted_to_staff, invite_token, created_at 
		FROM social.groups 
		WHERE id = $1`
		
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&g.ID, &g.Name, &g.Description, &g.OwnerUserID, &g.NotesRestrictedToStaff, &g.InviteToken, &g.CreatedAt,
	)
	
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Se maneja el 404 en la capa de servicio
		}
		return nil, err
	}
	return &g, nil
}

func (r *GroupRepository) ListMyGroups(ctx context.Context, userID string) ([]*model.MyGroup, error) {
	query := `
		SELECT g.id, g.name, m.role
		FROM social.groups g
		JOIN social.group_memberships m ON g.id = m.group_id
		WHERE m.user_id = $1
		ORDER BY g.created_at DESC`
		
	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*model.MyGroup
	for rows.Next() {
		var mg model.MyGroup
		if err := rows.Scan(&mg.GroupID, &mg.Name, &mg.Role); err != nil {
			return nil, err
		}
		groups = append(groups, &mg)
	}
	
	if err := rows.Err(); err != nil {
		return nil, err
	}
	
	return groups, nil
}

// JoinWithInviteToken valida token, baneo e inserta la membresía como 'member'
func (r *GroupRepository) JoinWithInviteToken(ctx context.Context, groupID string, inviteToken string, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1. Verificar si el usuario está baneado en este grupo
	var isBanned bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM social.banned_users WHERE group_id = $1 AND user_id = $2)
	`, groupID, userID).Scan(&isBanned)
	if err != nil {
		return err
	}
	if isBanned {
		return errors.New("user_banned")
	}

	// 2. Verificar que el grupo exista y el invite_token sea correcto
	var validGroup bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM social.groups WHERE id = $1 AND invite_token = $2)
	`, groupID, inviteToken).Scan(&validGroup)
	if err != nil {
		return err
	}
	if !validGroup {
		return errors.New("invalid_invite_token")
	}

	// 3. Insertar la membresía (si ya existe por unique constraint, devolverá error controlado)
	_, err = tx.Exec(ctx, `
		INSERT INTO social.group_memberships (group_id, user_id, role)
		VALUES ($1, $2, 'member')
		ON CONFLICT (group_id, user_id) DO NOTHING
	`, groupID, userID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// RegenerateInviteToken valida que el usuario sea admin y genera un nuevo invite_token
func (r *GroupRepository) RegenerateInviteToken(ctx context.Context, groupID string, userID string) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1. Verificar si el usuario es admin del grupo
	var role string
	err = tx.QueryRow(ctx, `
		SELECT role FROM social.group_memberships WHERE group_id = $1 AND user_id = $2
	`, groupID, userID).Scan(&role)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", errors.New("forbidden")
		}
		return "", err
	}
	if role != "admin" {
		return "", errors.New("forbidden")
	}

	// 2. Actualizar el invite_token del grupo y retornar el nuevo
	var newToken string
	err = tx.QueryRow(ctx, `
		UPDATE social.groups 
		SET invite_token = gen_random_uuid() 
		WHERE id = $1 
		RETURNING invite_token::text
	`, groupID).Scan(&newToken)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", errors.New("group_not_found")
		}
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return newToken, nil
}

// ListMembers retorna el listado de miembros de un grupo
// IsMember verifica si un usuario pertenece al grupo
func (r *GroupRepository) IsMember(ctx context.Context, groupID string, userID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM social.group_memberships WHERE group_id = $1 AND user_id = $2)
	`, groupID, userID).Scan(&exists)
	return exists, err
}

// ListMembers retorna el listado de miembros de un grupo con sus roles
func (r *GroupRepository) ListMembers(ctx context.Context, groupID string) ([]*model.GroupMembership, error) {
	var exists bool
	_ = r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM social.groups WHERE id = $1)`, groupID).Scan(&exists)
	if !exists {
		return nil, errors.New("group_not_found")
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, group_id, user_id, role, joined_at 
		FROM social.group_memberships 
		WHERE group_id = $1 
		ORDER BY joined_at ASC
	`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []*model.GroupMembership
	for rows.Next() {
		var m model.GroupMembership
		if err := rows.Scan(&m.ID, &m.GroupID, &m.UserID, &m.Role, &m.JoinedAt); err != nil {
			return nil, err
		}
		members = append(members, &m)
	}

	if members == nil {
		return []*model.GroupMembership{}, nil
	}
	return members, nil
}

// KickMember elimina a un usuario del grupo si el solicitante es admin
func (r *GroupRepository) KickMember(ctx context.Context, groupID, adminID, targetUserID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1. Validar que el solicitante sea admin
	var adminRole string
	err = tx.QueryRow(ctx, `SELECT role FROM social.group_memberships WHERE group_id = $1 AND user_id = $2`, groupID, adminID).Scan(&adminRole)
	if err != nil {
		if err == pgx.ErrNoRows {
			return errors.New("forbidden")
		}
		return err
	}
	if adminRole != "admin" {
		return errors.New("forbidden")
	}

	// 2. Prevenir auto-expulsión
	if adminID == targetUserID {
		return errors.New("cannot_modify_self")
	}

	// 3. Eliminar membresía (protegiendo a otros admins)
	res, err := tx.Exec(ctx, `DELETE FROM social.group_memberships WHERE group_id = $1 AND user_id = $2 AND role != 'admin'`, groupID, targetUserID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return errors.New("target_not_found_or_is_admin")
	}

	return tx.Commit(ctx)
}

// BanMember elimina y bloquea a un usuario si el solicitante es admin
func (r *GroupRepository) BanMember(ctx context.Context, groupID, adminID, targetUserID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1. Validar que el solicitante sea admin
	var adminRole string
	err = tx.QueryRow(ctx, `SELECT role FROM social.group_memberships WHERE group_id = $1 AND user_id = $2`, groupID, adminID).Scan(&adminRole)
	if err != nil {
		if err == pgx.ErrNoRows {
			return errors.New("forbidden")
		}
		return err
	}
	if adminRole != "admin" {
		return errors.New("forbidden")
	}

	if adminID == targetUserID {
		return errors.New("cannot_modify_self")
	}

	// 2. Eliminar membresía
	_, err = tx.Exec(ctx, `DELETE FROM social.group_memberships WHERE group_id = $1 AND user_id = $2 AND role != 'admin'`, groupID, targetUserID)
	if err != nil {
		return err
	}

	// 3. Registrar baneo
	_, err = tx.Exec(ctx, `
		INSERT INTO social.banned_users (group_id, user_id) 
		VALUES ($1, $2) 
		ON CONFLICT DO NOTHING
	`, groupID, targetUserID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ChangeMemberRole actualiza el rol de un miembro (solo admin puede hacerlo)
func (r *GroupRepository) ChangeMemberRole(ctx context.Context, groupID, adminID, targetUserID, newRole string) error {
	if newRole != "admin" && newRole != "member" {
		return errors.New("invalid_role")
	}
	if adminID == targetUserID {
		return errors.New("cannot_modify_self")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Validar que quien pide el cambio sea admin
	var role string
	err = tx.QueryRow(ctx, `SELECT role FROM social.group_memberships WHERE group_id = $1 AND user_id = $2`, groupID, adminID).Scan(&role)
	if err != nil {
		if err == pgx.ErrNoRows {
			return errors.New("forbidden")
		}
		return err
	}
	if role != "admin" {
		return errors.New("forbidden")
	}

	// Ejecutar el cambio
	tag, err := tx.Exec(ctx, `UPDATE social.group_memberships SET role = $1 WHERE group_id = $2 AND user_id = $3`, newRole, groupID, targetUserID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("target_not_found")
	}

	return tx.Commit(ctx)
}

// TransferAdmin cede la administración a otro miembro y degrada al admin actual
func (r *GroupRepository) TransferAdmin(ctx context.Context, groupID, currentAdminID, newAdminID string) error {
	if currentAdminID == newAdminID {
		return errors.New("cannot_modify_self")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Validar que quien transfiere sea admin
	var role string
	err = tx.QueryRow(ctx, `SELECT role FROM social.group_memberships WHERE group_id = $1 AND user_id = $2`, groupID, currentAdminID).Scan(&role)
	if err != nil {
		if err == pgx.ErrNoRows {
			return errors.New("forbidden")
		}
		return err
	}
	if role != "admin" {
		return errors.New("forbidden")
	}

	// Validar que el destinatario realmente sea miembro del grupo
	var newAdminExists bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM social.group_memberships WHERE group_id = $1 AND user_id = $2)`, groupID, newAdminID).Scan(&newAdminExists)
	if err != nil {
		return err
	}
	if !newAdminExists {
		return errors.New("target_not_found")
	}

	// Ascender al nuevo usuario
	_, err = tx.Exec(ctx, `UPDATE social.group_memberships SET role = 'admin' WHERE group_id = $1 AND user_id = $2`, groupID, newAdminID)
	if err != nil {
		return err
	}

	// Degradar al usuario original
	_, err = tx.Exec(ctx, `UPDATE social.group_memberships SET role = 'member' WHERE group_id = $1 AND user_id = $2`, groupID, currentAdminID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *GroupRepository) GetMemberRole(ctx context.Context, groupID, userID string) (string, error) {
	var role string
	err := r.pool.QueryRow(ctx, `SELECT role FROM social.group_memberships WHERE group_id = $1 AND user_id = $2`, groupID, userID).Scan(&role)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", errors.New("not_member")
		}
		return "", err
	}
	return role, nil
}

// CountAdmins cuenta cuántos administradores activos tiene el grupo
func (r *GroupRepository) CountAdmins(ctx context.Context, groupID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM social.group_memberships WHERE group_id = $1 AND role = 'admin'`, groupID).Scan(&count)
	return count, err
}

// RemoveMember elimina la membresía de un usuario
func (r *GroupRepository) RemoveMember(ctx context.Context, groupID, userID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM social.group_memberships WHERE group_id = $1 AND user_id = $2`, groupID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("target_not_found")
	}
	return nil
}