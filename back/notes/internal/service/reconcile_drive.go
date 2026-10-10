package service

import (
	"context"
	"errors"
	"log"
	"regexp"

	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/utils"
)

// ReconcileDriveDeletions detecta eliminaciones hechas directamente en Google
// Drive (papelera o borrado definitivo del .md o de adjuntos) y reconcilia la
// metadata local. Es la contraparte Drive→App del botón manual: eficiente
// (1 listado + confirmaciones solo de candidatos) y fail-closed.
//
// Solo el missing CONFIRMADO (404 de la API o trashed=true, documentado como
// eliminado porque el usuario lo borró en Drive) provoca borrados, reutilizando
// los flujos normales Delete/DeleteWithDriveCleanup y
// DeleteAttachmentWithDriveCleanup. Cualquier error temporal (timeout, red,
// OAuth, 403, 429, 5xx) aborta (OAuth/lista) o se cuenta como pendiente
// (confirmaciones puntuales) SIN tocar Postgres ni cache.
func (s *NoteService) ReconcileDriveDeletions(ctx context.Context, userID string) (*model.ReconcileSummary, error) {
	summary := &model.ReconcileSummary{}
	driveCtx, cancelDrive := withDriveTimeout(ctx)
	activeList, err := s.observedDriveListAppFileIDs(driveCtx, userID)
	cancelDrive()
	if err != nil {
		if drive.IsOAuthError(err) {
			return nil, err
		}
		return nil, ErrDriveUnavailable
	}
	active := make(map[string]struct{}, len(activeList))
	for _, id := range activeList {
		active[id] = struct{}{}
	}
	cursor := ""
	for {
		notes, next, err := s.observedNotesListByUser(ctx, userID, cursor, 100)
		if err != nil {
			return nil, ErrInternalDatabase
		}
		for _, n := range notes {
			if err := s.reconcileOneNote(ctx, userID, n, active, summary); err != nil {
				return nil, err
			}
		}
		if next == "" {
			break
		}
		cursor = next
	}
	return summary, nil
}

// reconcileOneNote verifica una nota propia. Devuelve error solo para abortar
// todo el run (OAuth); los fallos puntuales se cuentan como pendientes.
func (s *NoteService) reconcileOneNote(ctx context.Context, userID string, n *model.Note, active map[string]struct{}, summary *model.ReconcileSummary) error {
	mdID := ""
	if n.ExternalFileID != nil {
		mdID = *n.ExternalFileID
	}
	if mdID == "" {
		return nil // Nunca sincronizada: la crea el pipeline, no este run.
	}
	if _, ok := active[mdID]; !ok {
		gone, err := s.confirmDriveGone(ctx, userID, mdID)
		if err != nil {
			if drive.IsOAuthError(err) {
				return err
			}
			summary.Pending++
			return nil
		}
		if !gone {
			return nil
		}
		// Mismo flujo normal de borrado: PG + outbox durable + Drive
		// best-effort de los attachments restantes (el .md ya no existe:
		// 404 idempotente).
		if err := s.Delete(ctx, userID, n.ID); err != nil {
			if isNotFoundServiceError(err) {
				summary.RemovedNotes++
				summary.RemovedNoteIDs = append(summary.RemovedNoteIDs, n.ID)
				return nil
			}
			summary.Pending++
			return nil
		}
		summary.RemovedNotes++
		summary.RemovedNoteIDs = append(summary.RemovedNoteIDs, n.ID)
		return nil
	}
	atts, err := s.ListAttachments(ctx, n.ID)
	if err != nil {
		summary.Pending++
		return nil
	}
	for _, att := range atts {
		if att.ExternalFileID == "" {
			continue
		}
		if _, ok := active[att.ExternalFileID]; ok {
			continue
		}
		gone, err := s.confirmDriveGone(ctx, userID, att.ExternalFileID)
		if err != nil {
			if drive.IsOAuthError(err) {
				return err
			}
			summary.Pending++
			continue
		}
		if !gone {
			continue
		}
		noteRemoved, err := s.reconcileMissingAttachment(ctx, userID, n, att, mdID)
		if err != nil {
			if drive.IsOAuthError(err) {
				return err
			}
			summary.Pending++
			continue
		}
		if noteRemoved {
			summary.RemovedNotes++
			summary.RemovedNoteIDs = append(summary.RemovedNoteIDs, n.ID)
			return nil
		}
		summary.RemovedAttachments++
	}
	return nil
}

// reconcileMissingAttachment elimina la metadata de un adjunto confirmado
// como borrado en Drive y limpia su referencia inline del Markdown.
// Devuelve noteRemoved=true si, por una carrera (el .md desapareció entre la
// verificación y la lectura), se escaló al borrado de la nota completa.
func (s *NoteService) reconcileMissingAttachment(ctx context.Context, userID string, note *model.Note, att *model.Attachment, mdID string) (bool, error) {
	if _, err := s.observedAttachmentsDeleteAttachmentWithDriveCleanup(ctx, note.ID, att.ID, userID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	driveCtx, cancelDrive := withDriveTimeout(ctx)
	content, err := s.observedDriveGetFileContent(driveCtx, userID, mdID)
	cancelDrive()
	if err != nil {
		if drive.IsNotFound(err) {
			// Carrera: el .md se borró tras la verificación. Escalar al
			// flujo normal de borrado (idempotente ante el 404).
			if delErr := s.Delete(ctx, userID, note.ID); delErr != nil && !isNotFoundServiceError(delErr) {
				return false, delErr
			}
			return true, nil
		}
		// Fallo temporal leyendo el .md: la metadata ya se borró (fuente de
		// verdad); la referencia stale restante muestra fallback en la UI.
		log.Printf("[Notes] RECONCILE note=%s attachment=%s markdown unreadable: %v", note.ID, att.ID, err)
		return false, nil
	}
	stripped := stripAttachmentRefs(content, att.ID)
	if stripped == content {
		return false, nil
	}
	driveCtx, cancelDrive = withDriveTimeout(ctx)
	updErr := retryDriveOperation(driveCtx, driveRetryMaxAttempts, func() error {
		return s.observedDriveUpdateFile(driveCtx, userID, mdID, &stripped, nil)
	})
	cancelDrive()
	if updErr != nil {
		log.Printf("[Notes] RECONCILE note=%s attachment=%s markdown update pending: %v", note.ID, att.ID, updErr)
	}
	return false, nil
}

// confirmDriveGone confirma ausencia definitiva con timeout+reintento.
// err==nil => veredicto en gone. err!=nil => el llamador aborta (OAuth) o
// cuenta pendiente (cualquier otro temporal): jamás equivale a eliminado.
func (s *NoteService) confirmDriveGone(ctx context.Context, userID, fileID string) (bool, error) {
	driveCtx, cancelDrive := withDriveTimeout(ctx)
	defer cancelDrive()
	var gone bool
	err := retryDriveOperation(driveCtx, driveRetryMaxAttempts, func() error {
		var opErr error
		gone, opErr = s.observedDriveFileGone(driveCtx, userID, fileID)
		return opErr
	})
	if err != nil {
		return false, err
	}
	return gone, nil
}

// isNotFoundServiceError detecta el 404 de dominio (nota/adjunto inexistente)
// sin inspeccionar strings.
func isNotFoundServiceError(err error) bool {
	var se *ServiceError
	if errors.As(err, &se) {
		return se.Code == utils.ErrNotFound
	}
	return false
}

// stripAttachmentRefs elimina del Markdown las referencias inline
// `![label](attachment:<id>)` / `[label](attachment:<id>)` de un adjunto.
// UUIDs no contienen metacaracteres fuera de clase, pero se escapan igual.
func stripAttachmentRefs(content, attachmentID string) string {
	if attachmentID == "" {
		return content
	}
	re := regexp.MustCompile(`!?\[[^\]]*\]\(attachment:` + regexp.QuoteMeta(attachmentID) + `\)`)
	return re.ReplaceAllString(content, "")
}
