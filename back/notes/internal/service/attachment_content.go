package service

import (
	"context"
	"errors"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/utils"
)

// GetAttachmentContent uses the same read policy as Get before resolving the attachment.
func (s *NoteService) GetAttachmentContent(ctx context.Context, requesterID, noteID, attachmentID string) ([]byte, string, *model.Attachment, error) {
	if !utils.ValidateUUID(noteID) || !utils.ValidateUUID(attachmentID) {
		return nil, "", nil, notFoundNote()
	}
	note, err := s.observedNotesGetByID(ctx, noteID)
	if err != nil {
		return nil, "", nil, ErrInternalDatabase
	}
	if note == nil {
		return nil, "", nil, notFoundNote()
	}
	allowed, err := s.hasReadAccess(ctx, note, requesterID)
	if err != nil {
		return nil, "", nil, err
	}
	if !allowed {
		return nil, "", nil, notFoundNote()
	}
	att, err := s.observedAttachmentsGetByID(ctx, attachmentID)
	if err != nil {
		return nil, "", nil, ErrInternalDatabase
	}
	if att == nil || att.NoteID != noteID {
		return nil, "", nil, notFoundNote()
	}
	driveCtx, cancel := withDriveTimeout(ctx)
	defer cancel()
	var data []byte
	var contentType string
	err = retryDriveOperation(driveCtx, driveRetryMaxAttempts, func() error {
		var downloadErr error
		data, contentType, downloadErr = s.observedDriveDownloadAttachment(driveCtx, note.UserID, att.ExternalFileID)
		return downloadErr
	})
	if err != nil {
		if drive.IsOAuthError(err) || drive.IsForbidden(err) || drive.IsNotFound(err) {
			return nil, "", nil, newServiceErrorMsg(utils.ErrNoteUnavailable, "Adjunto no disponible")
		}
		var de *drive.DriveError
		if errors.As(err, &de) && de.Code == 413 {
			return nil, "", nil, newServiceError(utils.ErrFileTooLarge)
		}
		return nil, "", nil, ErrDriveUnavailable
	}
	if len(data) > drive.MaxAttachmentBytes {
		return nil, "", nil, newServiceError(utils.ErrFileTooLarge)
	}
	return data, contentType, att, nil
}
