package service

import (
	"context"
	"strings"

	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
)

// copyAttachmentsAndRewrite clona en el Drive del copiador todos los adjuntos
// del apunte origen y reescribe el Markdown del clon para que las referencias
// `attachment:<uuid>` apunten a los adjuntos nuevos.
//
// Por qué se copian físicamente: el visor resuelve el contenido con
// GET /notes/{nota}/attachments/{adjunto}/content, que exige que el adjunto
// pertenezca a esa nota. Un clon que conservara los IDs del original no podría
// mostrar sus imágenes. Compartir el archivo del autor o reutilizar una URL
// pública de Drive tampoco sirve: el clon debe ser independiente y no debe
// exponer enlaces del Drive del autor.
//
// La operación es idempotente: (note_id, copied_from_attachment_id) es único y
// CreateCopy devuelve la fila existente, de modo que un reintento tras un fallo
// reutiliza los archivos ya copiados en vez de duplicarlos.
func (s *NoteService) copyAttachmentsAndRewrite(ctx context.Context, userID, sourceOwnerID, sourceNoteID, cloneNoteID, cloneFileID string) error {
	atts, err := s.observedAttachmentsListByNote(ctx, sourceNoteID)
	if err != nil {
		return ErrInternalDatabase
	}
	if len(atts) == 0 {
		return nil
	}
	// El contenido se lee del archivo YA clonado (propiedad del copiador): así
	// no se depende del OAuth del autor, que puede estar revocado.
	content, err := s.observedDriveGetFileContent(ctx, userID, cloneFileID)
	if err != nil {
		return err
	}
	rewritten := content
	copiedAny := false
	for _, att := range atts {
		if att == nil || strings.TrimSpace(att.ExternalFileID) == "" {
			continue
		}
		cloneAtt, err := s.observedAttachmentsGetByNoteAndSource(ctx, cloneNoteID, att.ID)
		if err != nil {
			return ErrInternalDatabase
		}
		if cloneAtt == nil {
			fileName := "adjunto"
			if att.FileName != nil && strings.TrimSpace(*att.FileName) != "" {
				fileName = *att.FileName
			}
			copyCtx, cancel := withDriveTimeout(ctx)
			var newFileID, newFileURL string
			err = retryDriveOperation(copyCtx, driveRetryMaxAttempts, func() error {
				var opErr error
				newFileID, newFileURL, opErr = s.observedDriveCopyAttachment(copyCtx, sourceOwnerID, att.ExternalFileID, userID, cloneNoteID, fileName, att.FileType, att.IsInline)
				return opErr
			})
			cancel()
			if err != nil {
				return err
			}
			cloneAtt, err = s.observedAttachmentsCreateCopy(ctx, cloneNoteID, newFileID, newFileURL, att.FileType, att.FileName, att.FileSizeBytes, att.IsInline, att.ID)
			if err != nil {
				return ErrInternalDatabase
			}
		}
		rewritten = replaceAttachmentReference(rewritten, att, cloneAtt)
		copiedAny = true
	}
	if !copiedAny || rewritten == content {
		return nil
	}
	updateCtx, cancel := withDriveTimeout(ctx)
	defer cancel()
	return retryDriveOperation(updateCtx, driveRetryMaxAttempts, func() error {
		return s.observedDriveUpdateFile(updateCtx, userID, cloneFileID, &rewritten, nil)
	})
}

// replaceAttachmentReference sustituye las referencias al adjunto origen por su
// equivalente clonado. Cubre el formato canónico `attachment:<uuid>` y, por
// compatibilidad con apuntes antiguos, la URL de Drive que quedó incrustada en
// el Markdown (se convierte al endpoint autenticado).
func replaceAttachmentReference(content string, source *model.Attachment, clone *model.Attachment) string {
	out := strings.ReplaceAll(content, "attachment:"+source.ID, "attachment:"+clone.ID)
	if source.FileURL != "" && source.FileURL != clone.FileURL {
		out = strings.ReplaceAll(out, source.FileURL, "attachment:"+clone.ID)
	}
	return out
}
