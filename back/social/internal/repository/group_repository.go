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

// requireAdminTx valida dentro de la transacción que userID sea admin del grupo.
// Devuelve "forbidden" si no es miembro o no es admin.
func requireAdminTx(ctx context.Context, tx pgx.Tx, groupID, userID string) error {
	// El mismo lock que LeaveGroup: no permitir expulsar/degradar al sucesor
	// mientras una salida decide quién administrará el grupo.
	var locked int
	if err := tx.QueryRow(ctx, `SELECT 1 FROM social.groups WHERE id = $1 FOR UPDATE`, groupID).Scan(&locked); err != nil {
		if err == pgx.ErrNoRows {
			return errors.New("forbidden")
		}
		return err
	}

	var role string
	err := tx.QueryRow(ctx, `SELECT role FROM social.group_memberships WHERE group_id = $1 AND user_id = $2`, groupID, userID).Scan(&role)
	if err != nil {
		if err == pgx.ErrNoRows {
			return errors.New("forbidden")
		}
		return err
	}
	if role != model.RoleAdmin {
		return errors.New("forbidden")
	}
	return nil
}

// requireRemovableTargetTx valida que el objetivo sea miembro y no admin.
// Devuelve "target_not_found" o "target_is_admin".
func requireRemovableTargetTx(ctx context.Context, tx pgx.Tx, groupID, targetUserID string) error {
	var role string
	err := tx.QueryRow(ctx, `SELECT role FROM social.group_memberships WHERE group_id = $1 AND user_id = $2`, groupID, targetUserID).Scan(&role)
	if err != nil {
		if err == pgx.ErrNoRows {
			return errors.New("target_not_found")
		}
		return err
	}
	if role == model.RoleAdmin {
		return errors.New("target_is_admin")
	}
	return nil
}

// KickMember elimina a un usuario del grupo si el solicitante es admin.
// No se puede expulsar a uno mismo ni a otro admin.
func (r *GroupRepository) KickMember(ctx context.Context, groupID, adminID, targetUserID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := requireAdminTx(ctx, tx, groupID, adminID); err != nil {
		return err
	}
	if adminID == targetUserID {
		return errors.New("cannot_modify_self")
	}
	if err := requireRemovableTargetTx(ctx, tx, groupID, targetUserID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM social.group_memberships WHERE group_id = $1 AND user_id = $2`, groupID, targetUserID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// BanMember elimina y bloquea a un usuario si el solicitante es admin.
// Mismas reglas que KickMember, más el registro en banned_users
// (banned_by_user_id es NOT NULL en el schema).
func (r *GroupRepository) BanMember(ctx context.Context, groupID, adminID, targetUserID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := requireAdminTx(ctx, tx, groupID, adminID); err != nil {
		return err
	}
	if adminID == targetUserID {
		return errors.New("cannot_modify_self")
	}
	if err := requireRemovableTargetTx(ctx, tx, groupID, targetUserID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM social.group_memberships WHERE group_id = $1 AND user_id = $2`, groupID, targetUserID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO social.banned_users (group_id, user_id, banned_by_user_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (group_id, user_id) DO NOTHING
	`, groupID, targetUserID, adminID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ChangeMemberRole actualiza el rol de un miembro (solo admin puede hacerlo)
func (r *GroupRepository) ChangeMemberRole(ctx context.Context, groupID, adminID, targetUserID, newRole string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Primero la autorización: un no-admin nunca debe distinguir 400 de 403
	if err := requireAdminTx(ctx, tx, groupID, adminID); err != nil {
		return err
	}
	if newRole != model.RoleAdmin && newRole != model.RoleMember {
		return errors.New("invalid_role")
	}
	if adminID == targetUserID {
		return errors.New("cannot_modify_self")
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
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := requireAdminTx(ctx, tx, groupID, currentAdminID); err != nil {
		return err
	}
	if currentAdminID == newAdminID {
		return errors.New("cannot_modify_self")
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

// ListMyGroupsDetailed devuelve los grupos del usuario con descripción,
// fecha de ingreso y cantidad de miembros (vista principal, tarea 2_3_14).
func (r *GroupRepository) ListMyGroupsDetailed(ctx context.Context, userID string) ([]*model.GroupCard, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT g.id, g.name, g.description, m.role, m.joined_at,
		       (SELECT count(*) FROM social.group_memberships x WHERE x.group_id = g.id) AS member_count
		FROM social.groups g
		JOIN social.group_memberships m ON m.group_id = g.id
		WHERE m.user_id = $1
		ORDER BY g.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cards := []*model.GroupCard{}
	for rows.Next() {
		var c model.GroupCard
		if err := rows.Scan(&c.GroupID, &c.Name, &c.Description, &c.Role, &c.JoinedAt, &c.MemberCount); err != nil {
			return nil, err
		}
		cards = append(cards, &c)
	}
	return cards, rows.Err()
}

// DeleteGroup elimina el grupo; membresías, baneos, tableros, hojas y
// reuniones caen por ON DELETE CASCADE.
func (r *GroupRepository) DeleteGroup(ctx context.Context, groupID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM social.groups WHERE id = $1`, groupID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("group_not_found")
	}
	return nil
}

// HandleAccountDeletion saca al usuario de todos sus grupos con sucesión
// automática de admin y borra sus baneos. Cada grupo se procesa en su propia
// transacción, así que un fallo a medias se corrige reintentando (idempotente).
func (r *GroupRepository) HandleAccountDeletion(ctx context.Context, userID string) ([]model.SuccessionResult, error) {
	rows, err := r.pool.Query(ctx, `SELECT group_id FROM social.group_memberships WHERE user_id = $1 ORDER BY joined_at`, userID)
	if err != nil {
		return nil, err
	}
	var groupIDs []string
	for rows.Next() {
		var gid string
		if err := rows.Scan(&gid); err != nil {
			rows.Close()
			return nil, err
		}
		groupIDs = append(groupIDs, gid)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	results := make([]model.SuccessionResult, 0, len(groupIDs))
	for _, gid := range groupIDs {
		res, err := r.removeUserFromGroup(ctx, gid, userID, false)
		if err != nil {
			return results, err
		}
		results = append(results, res)
	}

	if _, err := r.pool.Exec(ctx, `DELETE FROM social.banned_users WHERE user_id = $1`, userID); err != nil {
		return results, err
	}
	return results, nil
}

// removeUserFromGroup quita la membresía y aplica la sucesión de admin.
// Bloquea la fila del grupo (FOR UPDATE) para que dos admins que eliminan su
// cuenta a la vez no dejen el grupo sin administrador.
func (r *GroupRepository) removeUserFromGroup(ctx context.Context, groupID, userID string, strict bool) (model.SuccessionResult, error) {
	res := model.SuccessionResult{GroupID: groupID}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return res, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var locked int
	if err := tx.QueryRow(ctx, `SELECT 1 FROM social.groups WHERE id = $1 FOR UPDATE`, groupID).Scan(&locked); err != nil {
		if err == pgx.ErrNoRows {
			if strict {
				return res, errors.New("group_not_found")
			}
			return res, nil // el grupo ya no existe
		}
		return res, err
	}

	var role string
	err = tx.QueryRow(ctx, `SELECT role FROM social.group_memberships WHERE group_id = $1 AND user_id = $2`, groupID, userID).Scan(&role)
	if err != nil {
		if err == pgx.ErrNoRows {
			if strict {
				return res, errors.New("not_member")
			}
			return res, nil // otro proceso ya lo sacó
		}
		return res, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM social.group_memberships WHERE group_id = $1 AND user_id = $2`, groupID, userID); err != nil {
		return res, err
	}

	if role == model.RoleAdmin {
		var admins int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM social.group_memberships WHERE group_id = $1 AND role = 'admin'`, groupID).Scan(&admins); err != nil {
			return res, err
		}
		if admins == 0 {
			// Sucesión: el miembro más antiguo pasa a admin (usa idx_group_memberships_succession)
			var successor string
			err := tx.QueryRow(ctx, `
				SELECT user_id FROM social.group_memberships
				WHERE group_id = $1
				ORDER BY joined_at ASC, user_id ASC
				LIMIT 1`, groupID).Scan(&successor)
			if err == pgx.ErrNoRows {
				// era el último miembro: el grupo se elimina
				if _, err := tx.Exec(ctx, `DELETE FROM social.groups WHERE id = $1`, groupID); err != nil {
					return res, err
				}
				res.GroupDeleted = true
				return res, tx.Commit(ctx)
			}
			if err != nil {
				return res, err
			}
			if _, err := tx.Exec(ctx, `UPDATE social.group_memberships SET role = 'admin' WHERE group_id = $1 AND user_id = $2`, groupID, successor); err != nil {
				return res, err
			}
			res.PromotedUserID = &successor
		}
	}

	// Si el usuario era el dueño, el dueño pasa al admin más antiguo restante
	if _, err := tx.Exec(ctx, `
		UPDATE social.groups g SET owner_user_id = a.user_id
		FROM (
			SELECT user_id FROM social.group_memberships
			WHERE group_id = $1 AND role = 'admin'
			ORDER BY joined_at ASC, user_id ASC
			LIMIT 1
		) a
		WHERE g.id = $1 AND g.owner_user_id = $2`, groupID, userID); err != nil {
		return res, err
	}

	return res, tx.Commit(ctx)
}

// LeaveGroup comparte transacción y lock con la eliminación de cuenta.
func (r *GroupRepository) LeaveGroup(ctx context.Context, groupID, userID string) (model.SuccessionResult, error) {
	return r.removeUserFromGroup(ctx, groupID, userID, true)
}
