package service

import (
	"context"
	"fmt"
	"log"

	notesv1 "github.com/kurovoxx/tallerintegracion3/back/notes/internal/gen/notesv1/v1"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/utils"
)

type AccountDeletionStore interface {
	Begin(context.Context, string) (complete bool, err error)
	Complete(context.Context, string) error
	SaveGroupWork(context.Context, string, string, []model.AccountDeletionGroupNote, bool) error
	PendingGroupWork(context.Context, string, string) ([]model.AccountDeletionGroupNote, error)
	CompleteGroupWork(context.Context, string, string, string) error
}

func (s *NoteService) BeginAccountDeletion(ctx context.Context, userID string) notesv1.AccountCleanupStatus {
	if err := s.accountDeletionPrecondition(userID); err != nil {
		logAccountDeletionFailure("begin", userID, err)
		return notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_ERROR
	}
	complete, err := s.accountDeletion.Begin(ctx, userID)
	if err != nil {
		logAccountDeletionFailure("begin", userID, err)
		return notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_ERROR
	}
	if complete {
		return notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_COMPLETE
	}
	return notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_PENDING
}

// AccountDeletionCleanup revokes access created by the account but preserves
// its own notes and attachments. No product rule authorizes deleting their
// Drive content as part of account deletion.
func (s *NoteService) AccountDeletionCleanup(ctx context.Context, userID string) notesv1.AccountCleanupStatus {
	if err := s.accountDeletionPrecondition(userID); err != nil {
		logAccountDeletionFailure("cleanup", userID, err)
		return notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_ERROR
	}
	complete, err := s.accountDeletion.Begin(ctx, userID)
	if err != nil {
		logAccountDeletionFailure("cleanup begin", userID, err)
		return notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_ERROR
	}
	if complete {
		return notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_COMPLETE
	}
	notes, err := s.listOwnedNotes(ctx, userID)
	if err != nil {
		logAccountDeletionFailure("cleanup list notes", userID, err)
		return notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_ERROR
	}
	for _, note := range notes {
		if err := ctx.Err(); err != nil {
			logAccountDeletionFailure("cleanup", userID, err)
			return notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_PENDING
		}
		if err := s.cleanNoteShares(ctx, userID, note.ID, "", false); err != nil {
			logAccountDeletionFailure("cleanup note "+note.ID, userID, err)
			return notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_PENDING
		}
	}
	if err := s.accountDeletion.Complete(ctx, userID); err != nil {
		logAccountDeletionFailure("cleanup complete", userID, err)
		return notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_ERROR
	}
	return notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_COMPLETE
}

// UnshareAccountGroup handles Social's durable per-group task. For a deleted
// last-member group, every share row for that group is removed; for a surviving
// group, only shares owned by the departing user are removed. Other authors,
// groups and public/link sources remain part of the desired Drive ACL.
func (s *NoteService) UnshareAccountGroup(ctx context.Context, userID, groupID string, groupDeleted bool) notesv1.AccountCleanupStatus {
	stage := "unshare group " + groupID
	if err := s.accountDeletionPrecondition(userID); err != nil || !utils.ValidateUUID(groupID) {
		if err == nil {
			err = fmt.Errorf("group_id inválido")
		}
		logAccountDeletionFailure(stage, userID, err)
		return notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_ERROR
	}
	// Queue every note before deleting any share row. If a later Drive revoke
	// fails, this row retains the note ID even though ListByGroup can no longer
	// discover the deleted share after a process restart.
	workByNote := make(map[string]model.AccountDeletionGroupNote)
	pending, err := s.accountDeletion.PendingGroupWork(ctx, userID, groupID)
	if err != nil {
		logAccountDeletionFailure(stage, userID, err)
		return notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_ERROR
	}
	for _, item := range pending {
		item.GroupDeleted = item.GroupDeleted || groupDeleted
		workByNote[item.NoteID] = item
	}
	cursor := ""
	for {
		shares, next, err := s.shared.ListByGroup(ctx, groupID, 100, cursor)
		if err != nil {
			logAccountDeletionFailure(stage, userID, err)
			return notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_ERROR
		}
		for _, share := range shares {
			if share != nil {
				workByNote[share.NoteID] = model.AccountDeletionGroupNote{NoteID: share.NoteID, GroupDeleted: groupDeleted}
			}
		}
		if next == "" {
			break
		}
		cursor = next
	}
	work := make([]model.AccountDeletionGroupNote, 0, len(workByNote))
	for _, item := range workByNote {
		work = append(work, item)
	}
	if err := s.accountDeletion.SaveGroupWork(ctx, userID, groupID, work, groupDeleted); err != nil {
		logAccountDeletionFailure(stage, userID, err)
		return notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_ERROR
	}
	pending, err = s.accountDeletion.PendingGroupWork(ctx, userID, groupID)
	if err != nil {
		logAccountDeletionFailure(stage, userID, err)
		return notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_ERROR
	}
	for _, item := range pending {
		if err := ctx.Err(); err != nil {
			logAccountDeletionFailure(stage, userID, err)
			return notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_PENDING
		}
		if err := s.cleanNoteShares(ctx, userID, item.NoteID, groupID, item.GroupDeleted); err != nil {
			logAccountDeletionFailure(stage+" note "+item.NoteID, userID, err)
			return notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_PENDING
		}
		if err := s.accountDeletion.CompleteGroupWork(ctx, userID, groupID, item.NoteID); err != nil {
			logAccountDeletionFailure(stage+" note "+item.NoteID, userID, err)
			return notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_PENDING
		}
	}
	return notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_COMPLETE
}

func (s *NoteService) accountDeletionPrecondition(userID string) error {
	if !utils.ValidateUUID(userID) {
		return fmt.Errorf("user_id inválido")
	}
	if s.accountDeletion == nil {
		return fmt.Errorf("repositorio de borrado de cuenta no configurado")
	}
	return nil
}

// logAccountDeletionFailure deja la causa en el log de Notes. El contrato gRPC
// solo devuelve un estado (Identity ve code=Internal), así que sin este log un
// fallo persistente (p. ej. la tabla del tombstone sin migrar) no se puede
// diagnosticar desde Notes. Solo registra el UUID, nunca datos personales.
func logAccountDeletionFailure(stage, userID string, err error) {
	log.Printf("notes: borrado de cuenta [%s] user_id=%s: %v", stage, userID, err)
}

func (s *NoteService) listOwnedNotes(ctx context.Context, userID string) ([]*model.Note, error) {
	var out []*model.Note
	cursor := ""
	for {
		page, next, err := s.notes.ListByUser(ctx, userID, cursor, 100)
		if err != nil {
			return nil, ErrInternalDatabase
		}
		out = append(out, page...)
		if next == "" {
			return out, nil
		}
		cursor = next
	}
}

func (s *NoteService) cleanNoteShares(ctx context.Context, userID, noteID, groupID string, removeAllGroupShares bool) error {
	return s.shared.WithNoteLock(ctx, noteID, func(lockCtx context.Context) error {
		note, err := s.notes.GetByID(lockCtx, noteID)
		if err != nil {
			return ErrInternalDatabase
		}
		if note == nil {
			// Keep the durable group task if a deleted note still has managed
			// Drive permissions. Without the note row we cannot confirm or safely
			// revoke them here; another cleanup path must resolve that ACL first.
			managed, err := s.shared.ListManagedPermissions(lockCtx, noteID)
			if err != nil {
				return ErrInternalDatabase
			}
			if len(managed) != 0 {
				return fmt.Errorf("missing note still has managed Drive permissions")
			}
			return nil
		}
		shares, err := s.shared.ListByNote(lockCtx, noteID)
		if err != nil {
			return ErrInternalDatabase
		}
		removed := make(map[string]bool)
		for _, share := range shares {
			if share == nil {
				continue
			}
			match := note.UserID == userID
			if groupID != "" && share.GroupID != groupID {
				match = false
			}
			if removeAllGroupShares && share.GroupID == groupID {
				match = true
			}
			if !match {
				continue
			}
			if err := s.shared.Delete(lockCtx, share.ID); err != nil {
				return ErrInternalDatabase
			}
			removed[share.ID] = true
		}
		if len(removed) > 0 {
			if err := s.shared.MarkManagedPermissionsPending(lockCtx, noteID); err != nil {
				return ErrInternalDatabase
			}
		}
		remaining, err := s.shared.ListByNote(lockCtx, noteID)
		if err != nil {
			return ErrInternalDatabase
		}
		desired, err := s.ComputeDesiredDrivePermissions(lockCtx, note.Visibility, remaining)
		if err != nil {
			return err
		}
		if err := s.syncDrivePermissions(lockCtx, note, desired, true); err != nil {
			return err
		}
		remaining, err = s.shared.ListByNote(lockCtx, noteID)
		if err != nil {
			return ErrInternalDatabase
		}
		for _, share := range remaining {
			if share.PermissionSyncStatus != model.PermissionSyncInSync {
				return fmt.Errorf("note permission state pending")
			}
		}
		managed, err := s.shared.ListManagedPermissions(lockCtx, noteID)
		if err != nil {
			return ErrInternalDatabase
		}
		for _, permission := range managed {
			if permission.SyncStatus != model.PermissionSyncInSync {
				return fmt.Errorf("managed permission state pending")
			}
		}
		return nil
	})
}
