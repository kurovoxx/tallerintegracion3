package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/utils"
)

// logNotesResult records the actual upstream result, before domain errors are
// translated. In particular, Drive 403/404 remain visible even when HTTP uses
// the same 404 response for unavailable and unauthorized resources.
func logNotesResult(ctx context.Context, started time.Time, event, fileID string, err error, attrs ...any) {
	logger := utils.NotesLogger(ctx)
	attrs = append(attrs, "event", event, "duration_ms", float64(time.Since(started).Microseconds())/1000)
	message := "[NOTES] " + event
	if len(event) >= 5 && event[:5] == "DRIVE" {
		message += fmt.Sprintf(" | fileID: %s", fileID)
		attrs = append(attrs, "drive_file_id", fileID)
	}
	level := slog.LevelInfo
	if err != nil {
		level = slog.LevelError
		message = "[NOTES] ERROR " + message[len("[NOTES] "):] + fmt.Sprintf(" | error: %v", err)
		attrs = append(attrs, "error", err)
		var driveErr *drive.DriveError
		if errors.As(err, &driveErr) {
			attrs = append(attrs, "drive_status", driveErr.Code)
		} else if drive.IsOAuthError(err) {
			attrs = append(attrs, "drive_status", 403)
		}
	}
	logger.Log(ctx, level, message, attrs...)
}

// The observed calls delegate once and preserve every store/client result.
// Logging contains identifiers and metadata only, never file contents or tokens.

func (s *NoteService) observedDriveDownloadAttachment(ctx context.Context, ownerID, fileID string) ([]byte, string, error) {
	started := time.Now()
	result0, result1, err := s.drive.DownloadAttachment(ctx, ownerID, fileID)
	logNotesResult(ctx, started, "DRIVE DOWNLOAD", fileID, err, "drive_owner_user_id", ownerID, "drive_file_id", fileID)
	return result0, result1, err
}

func (s *NoteService) observedDriveCreateFile(ctx context.Context, userID string, noteID string, title string, content string) (string, error) {
	started := time.Now()
	result0, err := s.drive.CreateFile(ctx, userID, noteID, title, content)
	logNotesResult(ctx, started, "DRIVE UPLOAD", result0, err, "drive_owner_user_id", userID, "note_id", noteID)
	return result0, err
}

func (s *NoteService) observedDriveGetFileContent(ctx context.Context, userID string, driveFileID string) (string, error) {
	started := time.Now()
	result0, err := s.drive.GetFileContent(ctx, userID, driveFileID)
	logNotesResult(ctx, started, "DRIVE DOWNLOAD", driveFileID, err, "drive_owner_user_id", userID, "drive_file_id", driveFileID)
	return result0, err
}

func (s *NoteService) observedDriveUpdateFile(ctx context.Context, userID string, driveFileID string, newContent *string, newTitle *string) error {
	started := time.Now()
	err := s.drive.UpdateFile(ctx, userID, driveFileID, newContent, newTitle)
	logNotesResult(ctx, started, "DRIVE SYNC", driveFileID, err, "drive_owner_user_id", userID, "drive_file_id", driveFileID)
	return err
}

func (s *NoteService) observedDriveDeleteFile(ctx context.Context, userID string, driveFileID string) error {
	started := time.Now()
	err := s.drive.DeleteFile(ctx, userID, driveFileID)
	logNotesResult(ctx, started, "DRIVE DELETE", driveFileID, err, "drive_owner_user_id", userID, "drive_file_id", driveFileID)
	return err
}

func (s *NoteService) observedDriveUploadAttachment(ctx context.Context, userID string, noteID string, fileName string, fileType string, data []byte, isInline bool) (string, string, error) {
	started := time.Now()
	result0, result1, err := s.drive.UploadAttachment(ctx, userID, noteID, fileName, fileType, data, isInline)
	logNotesResult(ctx, started, "DRIVE UPLOAD", result0, err, "drive_owner_user_id", userID, "note_id", noteID, "file_type", fileType, "file_size_bytes", len(data), "is_inline", isInline)
	return result0, result1, err
}

func (s *NoteService) observedDriveDeleteAttachment(ctx context.Context, userID string, externalFileID string) error {
	started := time.Now()
	err := s.drive.DeleteAttachment(ctx, userID, externalFileID)
	logNotesResult(ctx, started, "DRIVE DELETE", externalFileID, err, "drive_owner_user_id", userID, "drive_file_id", externalFileID)
	return err
}

func (s *NoteService) observedDriveCopyFile(ctx context.Context, srcUserID string, srcFileID string, dstUserID string, newNoteID string, newTitle string) (string, error) {
	started := time.Now()
	result0, err := s.drive.CopyFile(ctx, srcUserID, srcFileID, dstUserID, newNoteID, newTitle)
	logNotesResult(ctx, started, "DRIVE UPLOAD", result0, err, "source_user_id", srcUserID, "source_drive_file_id", srcFileID, "user_id", dstUserID, "note_id", newNoteID)
	return result0, err
}

func (s *NoteService) observedDriveFindFileByNoteID(ctx context.Context, ownerUserID string, noteID string) (string, error) {
	started := time.Now()
	result0, err := s.drive.FindFileByNoteID(ctx, ownerUserID, noteID)
	logNotesResult(ctx, started, "DRIVE LOOKUP", result0, err, "drive_owner_user_id", ownerUserID, "note_id", noteID)
	return result0, err
}

func (s *NoteService) observedDriveGrantPermission(ctx context.Context, ownerUserID string, fileID string, granteeEmail string, role string) error {
	started := time.Now()
	err := s.drive.GrantPermission(ctx, ownerUserID, fileID, granteeEmail, role)
	logNotesResult(ctx, started, "DRIVE SYNC", fileID, err, "drive_owner_user_id", ownerUserID, "drive_file_id", fileID, "email", granteeEmail)
	return err
}

func (s *NoteService) observedDriveGrantLinkPermission(ctx context.Context, ownerUserID string, fileID string) (string, error) {
	started := time.Now()
	result0, err := s.drive.GrantLinkPermission(ctx, ownerUserID, fileID)
	logNotesResult(ctx, started, "DRIVE SYNC", fileID, err, "drive_owner_user_id", ownerUserID, "drive_file_id", fileID)
	return result0, err
}

func (s *NoteService) observedDriveRevokePermissionByID(ctx context.Context, ownerUserID string, fileID string, permissionID string) error {
	started := time.Now()
	err := s.drive.RevokePermissionByID(ctx, ownerUserID, fileID, permissionID)
	logNotesResult(ctx, started, "DRIVE DELETE", fileID, err, "drive_owner_user_id", ownerUserID, "drive_file_id", fileID, "permission_id", permissionID)
	return err
}

func (s *NoteService) observedDriveRevokePermission(ctx context.Context, ownerUserID string, fileID string, granteeEmail string) error {
	started := time.Now()
	err := s.drive.RevokePermission(ctx, ownerUserID, fileID, granteeEmail)
	logNotesResult(ctx, started, "DRIVE DELETE", fileID, err, "drive_owner_user_id", ownerUserID, "drive_file_id", fileID, "email", granteeEmail)
	return err
}

func (s *NoteService) observedDriveRevokeAllPermissions(ctx context.Context, ownerUserID string, fileID string) error {
	started := time.Now()
	err := s.drive.RevokeAllPermissions(ctx, ownerUserID, fileID)
	logNotesResult(ctx, started, "DRIVE DELETE", fileID, err, "drive_owner_user_id", ownerUserID, "drive_file_id", fileID)
	return err
}

func (s *NoteService) observedDriveVerifyFileAccess(ctx context.Context, userID string, driveFileID string) error {
	started := time.Now()
	err := s.drive.VerifyFileAccess(ctx, userID, driveFileID)
	logNotesResult(ctx, started, "DRIVE ACCESS", driveFileID, err, "drive_owner_user_id", userID, "drive_file_id", driveFileID)
	return err
}

func (s *NoteService) observedDriveListAppFileIDs(ctx context.Context, userID string) ([]string, error) {
	started := time.Now()
	result0, err := s.drive.ListAppFileIDs(ctx, userID)
	logNotesResult(ctx, started, "DRIVE LIST", "", err, "drive_owner_user_id", userID, "files_count", len(result0))
	return result0, err
}

func (s *NoteService) observedDriveFileGone(ctx context.Context, userID string, driveFileID string) (bool, error) {
	started := time.Now()
	result0, err := s.drive.FileGone(ctx, userID, driveFileID)
	logNotesResult(ctx, started, "DRIVE EXISTENCE", driveFileID, err, "drive_owner_user_id", userID, "drive_file_id", driveFileID, "missing", result0)
	return result0, err
}

func (s *NoteService) observedNotesCreate(ctx context.Context, noteID string, userID string, subjectID *string, title string, externalFileID *string, visibility string, forkedFrom *string, syncStatus string) (*model.Note, error) {
	started := time.Now()
	result0, err := s.notes.Create(ctx, noteID, userID, subjectID, title, externalFileID, visibility, forkedFrom, syncStatus)
	attrs := []any{"note_id", noteID, "user_id", userID, "drive_file_id", derefOrEmpty(externalFileID), "sync_status", syncStatus, "forked_from_note_id", derefOrEmpty(forkedFrom), "visibility", visibility}
	if result0 != nil {
		attrs = append(attrs, "note_id", result0.ID)
	}
	logNotesResult(ctx, started, "DB notes.Create", derefOrEmpty(externalFileID), err, attrs...)
	return result0, err
}

func (s *NoteService) observedNotesGetByID(ctx context.Context, id string) (*model.Note, error) {
	started := time.Now()
	result0, err := s.notes.GetByID(ctx, id)
	attrs := []any{"note_id", id}
	if result0 != nil {
		attrs = append(attrs, "note_id", result0.ID)
	}
	logNotesResult(ctx, started, "DB notes.GetByID", "", err, attrs...)
	return result0, err
}

func (s *NoteService) observedNotesListByUser(ctx context.Context, userID, cursor string, limit int) ([]*model.Note, string, error) {
	started := time.Now()
	result0, result1, err := s.notes.ListByUser(ctx, userID, cursor, limit)
	logNotesResult(ctx, started, "DB notes.ListByUser", "", err, "user_id", userID, "limit", limit, "notes_count", len(result0))
	return result0, result1, err
}

func (s *NoteService) observedNotesUpdate(ctx context.Context, id string, title *string, visibility *string, expectedVersion int64) (*model.Note, error) {
	started := time.Now()
	result0, err := s.notes.Update(ctx, id, title, visibility, expectedVersion)
	attrs := []any{"note_id", id, "expected_version", expectedVersion}
	if result0 != nil {
		attrs = append(attrs, "note_id", result0.ID)
	}
	logNotesResult(ctx, started, "DB notes.Update", "", err, attrs...)
	return result0, err
}

func (s *NoteService) observedNotesUpdateExternalFileID(ctx context.Context, noteID, fileID string) error {
	started := time.Now()
	err := s.notes.UpdateExternalFileID(ctx, noteID, fileID)
	logNotesResult(ctx, started, "DB notes.UpdateExternalFileID", fileID, err, "note_id", noteID, "drive_file_id", fileID)
	return err
}

func (s *NoteService) observedNotesUpdateSyncStatus(ctx context.Context, noteID, syncStatus string) error {
	started := time.Now()
	err := s.notes.UpdateSyncStatus(ctx, noteID, syncStatus)
	logNotesResult(ctx, started, "DB notes.UpdateSyncStatus", "", err, "note_id", noteID, "sync_status", syncStatus)
	return err
}

func (s *NoteService) observedNotesDeleteWithDriveCleanup(ctx context.Context, noteID, requesterID string) (model.NoteDriveCleanup, error) {
	started := time.Now()
	result0, err := s.notes.DeleteWithDriveCleanup(ctx, noteID, requesterID)
	logNotesResult(ctx, started, "DB notes.DeleteWithDriveCleanup", result0.FileID, err, "note_id", noteID, "user_id", requesterID, "drive_file_id", result0.FileID, "attachments_count", len(result0.Attachments))
	return result0, err
}

func (s *NoteService) observedNotesEnqueueDriveOperation(ctx context.Context, op, noteID, attID, fileID, ownerUserID string, payload map[string]any) error {
	started := time.Now()
	err := s.notes.EnqueueDriveOperation(ctx, op, noteID, attID, fileID, ownerUserID, payload)
	logNotesResult(ctx, started, "DB notes.EnqueueDriveOperation", fileID, err, "operation", op, "note_id", noteID, "attachment_id", attID, "drive_file_id", fileID, "drive_owner_user_id", ownerUserID)
	return err
}

func (s *NoteService) observedNotesClaimDriveOperations(ctx context.Context, limit int, lockDuration time.Duration) ([]*model.DriveOperation, error) {
	started := time.Now()
	result0, err := s.notes.ClaimDriveOperations(ctx, limit, lockDuration)
	logNotesResult(ctx, started, "DB notes.ClaimDriveOperations", "", err, "limit", limit)
	return result0, err
}

func (s *NoteService) observedNotesCompleteDriveOperation(ctx context.Context, id string) error {
	started := time.Now()
	err := s.notes.CompleteDriveOperation(ctx, id)
	logNotesResult(ctx, started, "DB notes.CompleteDriveOperation", "", err, "outbox_id", id)
	return err
}

func (s *NoteService) observedNotesFailDriveOperation(ctx context.Context, id, errStr string, nextAttempt time.Time) error {
	started := time.Now()
	err := s.notes.FailDriveOperation(ctx, id, errStr, nextAttempt)
	logNotesResult(ctx, started, "DB notes.FailDriveOperation", "", err, "outbox_id", id, "cause", errStr, "next_attempt_at", nextAttempt)
	return err
}

func (s *NoteService) observedAttachmentsCreate(ctx context.Context, noteID, externalFileID, fileURL, fileType string, fileName *string, fileSize *int, isInline bool) (*model.Attachment, error) {
	started := time.Now()
	result0, err := s.attachments.Create(ctx, noteID, externalFileID, fileURL, fileType, fileName, fileSize, isInline)
	attrs := []any{"note_id", noteID, "drive_file_id", externalFileID, "file_type", fileType, "is_inline", isInline}
	if result0 != nil {
		attrs = append(attrs, "attachment_id", result0.ID)
	}
	logNotesResult(ctx, started, "DB attachments.Create", externalFileID, err, attrs...)
	return result0, err
}

func (s *NoteService) observedAttachmentsGetByID(ctx context.Context, id string) (*model.Attachment, error) {
	started := time.Now()
	result0, err := s.attachments.GetByID(ctx, id)
	attrs := []any{"attachment_id", id}
	if result0 != nil {
		attrs = append(attrs, "attachment_id", result0.ID)
	}
	logNotesResult(ctx, started, "DB attachments.GetByID", "", err, attrs...)
	return result0, err
}

func (s *NoteService) observedAttachmentsListByNote(ctx context.Context, noteID string) ([]*model.Attachment, error) {
	started := time.Now()
	result0, err := s.attachments.ListByNote(ctx, noteID)
	logNotesResult(ctx, started, "DB attachments.ListByNote", "", err, "note_id", noteID)
	return result0, err
}

func (s *NoteService) observedAttachmentsDeleteAttachmentWithDriveCleanup(ctx context.Context, noteID, attachmentID, requesterID string) (string, error) {
	started := time.Now()
	result0, err := s.attachments.DeleteAttachmentWithDriveCleanup(ctx, noteID, attachmentID, requesterID)
	logNotesResult(ctx, started, "DB attachments.DeleteAttachmentWithDriveCleanup", result0, err, "note_id", noteID, "attachment_id", attachmentID, "user_id", requesterID, "drive_file_id", result0)
	return result0, err
}

func (s *NoteService) observedSavedSave(ctx context.Context, userID, noteID string) (*model.SavedNote, error) {
	started := time.Now()
	result0, err := s.saved.Save(ctx, userID, noteID)
	attrs := []any{"user_id", userID, "note_id", noteID}
	if result0 != nil {
		attrs = append(attrs, "saved_note_id", result0.ID)
	}
	logNotesResult(ctx, started, "DB saved.Save", "", err, attrs...)
	return result0, err
}

func (s *NoteService) observedLikesLikeAtomic(ctx context.Context, noteID, userID string) error {
	started := time.Now()
	err := s.likes.LikeAtomic(ctx, noteID, userID)
	logNotesResult(ctx, started, "DB likes.LikeAtomic", "", err, "note_id", noteID, "user_id", userID)
	return err
}

func (s *NoteService) observedLikesUnlikeAtomic(ctx context.Context, noteID, userID string) error {
	started := time.Now()
	err := s.likes.UnlikeAtomic(ctx, noteID, userID)
	logNotesResult(ctx, started, "DB likes.UnlikeAtomic", "", err, "note_id", noteID, "user_id", userID)
	return err
}

func (s *NoteService) observedSharedCreate(ctx context.Context, noteID, groupID string, isAdminNote bool, accessMode string, followersSnapshot int) (*model.SharedNote, error) {
	started := time.Now()
	result0, err := s.shared.Create(ctx, noteID, groupID, isAdminNote, accessMode, followersSnapshot)
	attrs := []any{"note_id", noteID, "group_id", groupID, "access_mode", accessMode}
	if result0 != nil {
		attrs = append(attrs, "shared_note_id", result0.ID)
	}
	logNotesResult(ctx, started, "DB shared.Create", "", err, attrs...)
	return result0, err
}

func (s *NoteService) observedSharedDelete(ctx context.Context, id string) error {
	started := time.Now()
	err := s.shared.Delete(ctx, id)
	logNotesResult(ctx, started, "DB shared.Delete", "", err, "shared_note_id", id)
	return err
}

func (s *NoteService) observedSharedMarkManagedPermissionsPending(ctx context.Context, noteID string) error {
	started := time.Now()
	err := s.shared.MarkManagedPermissionsPending(ctx, noteID)
	logNotesResult(ctx, started, "DB shared.MarkManagedPermissionsPending", "", err, "note_id", noteID)
	return err
}

func (s *NoteService) observedSharedDeleteByUserAndGroup(ctx context.Context, userID, groupID string) error {
	started := time.Now()
	err := s.shared.DeleteByUserAndGroup(ctx, userID, groupID)
	logNotesResult(ctx, started, "DB shared.DeleteByUserAndGroup", "", err, "user_id", userID, "group_id", groupID)
	return err
}

func (s *NoteService) observedSharedUpsertManagedPermission(ctx context.Context, permission *model.DriveManagedPermission) (*model.DriveManagedPermission, error) {
	started := time.Now()
	result0, err := s.shared.UpsertManagedPermission(ctx, permission)
	attrs := []any{}
	if permission != nil {
		attrs = append(attrs, "note_id", permission.NoteID, "drive_file_id", permission.ExternalFileID,
			"principal_type", permission.PrincipalType, "principal_key", permission.PrincipalKey,
			"sync_status", permission.SyncStatus)
	}
	logNotesResult(ctx, started, "DB shared.UpsertManagedPermission", "", err, attrs...)
	return result0, err
}

func (s *NoteService) observedSharedUpdateNotePermissionSyncStatus(ctx context.Context, noteID, status string) error {
	started := time.Now()
	err := s.shared.UpdateNotePermissionSyncStatus(ctx, noteID, status)
	logNotesResult(ctx, started, "DB shared.UpdateNotePermissionSyncStatus", "", err, "note_id", noteID, "sync_status", status)
	return err
}
