package service

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/utils"
)

// Un usuario solo puede copiar una vez el mismo apunte: la segunda copia es
// 409 already_copied y no crea fila ni archivo. Borrar la copia habilita otra.
func TestCopySecondAttemptRejectedAsAlreadyCopied(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	copier := uuid.NewString()
	other := uuid.NewString()

	src, err := svc.Create(ctx, author, "Una sola copia", nil, "public", stringPtr("contenido"), "")
	if err != nil {
		t.Fatalf("create origen: %v", err)
	}
	first, err := svc.Copy(ctx, copier, src.ID, "copy-first")
	if err != nil {
		t.Fatalf("primera copia: %v", err)
	}
	beforeNotes := countNotes(noteStore)
	beforeFiles := driveMock.FileCount()

	second, err := svc.Copy(ctx, copier, src.ID, "copy-second")
	if second != nil {
		t.Fatalf("la segunda copia no debe devolver nota: %+v", second)
	}
	if code := serviceCode(t, err); code != utils.ErrAlreadyCopied {
		t.Fatalf("esperaba %s, got %s", utils.ErrAlreadyCopied, code)
	}
	if got := utils.StatusForCode(utils.ErrAlreadyCopied); got != 409 {
		t.Fatalf("already_copied debe mapear a 409, got %d", got)
	}
	if countNotes(noteStore) != beforeNotes || driveMock.FileCount() != beforeFiles {
		t.Fatalf("la segunda copia no debe crear fila ni archivo (notes %d->%d, files %d->%d)",
			beforeNotes, countNotes(noteStore), beforeFiles, driveMock.FileCount())
	}

	// Otro usuario sí puede copiar el mismo origen.
	if _, err := svc.Copy(ctx, other, src.ID, "copy-other"); err != nil {
		t.Fatalf("otro usuario debe poder copiar: %v", err)
	}

	// Si el copiador elimina su copia, puede volver a copiar el apunte.
	if err := svc.Delete(ctx, copier, first.ID); err != nil {
		t.Fatalf("delete del clon: %v", err)
	}
	third, err := svc.Copy(ctx, copier, src.ID, "copy-third")
	if err != nil {
		t.Fatalf("tras borrar la copia debe poder copiarse de nuevo: %v", err)
	}
	if third.ID == first.ID {
		t.Fatal("la nueva copia debe tener un id distinto al clon eliminado")
	}
}

// Concurrencia: N copias simultáneas del mismo origen y usuario dejan
// exactamente un clon; el resto recibe already_copied (nunca 500).
func TestCopyConcurrentSameUserCreatesSingleClone(t *testing.T) {
	svc, _, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	copier := uuid.NewString()
	src, err := svc.Create(ctx, author, "Carrera de copias", nil, "public", stringPtr("data"), "")
	if err != nil {
		t.Fatalf("create origen: %v", err)
	}

	const attempts = 8
	var wg sync.WaitGroup
	wg.Add(attempts)
	errs := make([]error, attempts)
	for i := 0; i < attempts; i++ {
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = svc.Copy(ctx, copier, src.ID, fmt.Sprintf("copy-race-%d", idx))
		}(i)
	}
	wg.Wait()

	success, duplicated := 0, 0
	for _, err := range errs {
		if err == nil {
			success++
			continue
		}
		if serviceCode(t, err) == utils.ErrAlreadyCopied {
			duplicated++
			continue
		}
		t.Fatalf("error inesperado en copia concurrente: %v", err)
	}
	if success != 1 || duplicated != attempts-1 {
		t.Fatalf("esperaba 1 éxito y %d already_copied, got %d/%d", attempts-1, success, duplicated)
	}
	if got := countNotes(noteStore); got != 2 {
		t.Fatalf("debe haber origen + un solo clon, got %d notas", got)
	}
}

// Copiar un apunte con adjuntos clona físicamente los binarios, reescribe las
// referencias `attachment:<uuid>` y sirve el contenido desde la copia (no desde
// el adjunto del autor, que sigue perteneciendo al apunte original).
func TestCopyClonesAttachmentsAndRewritesMarkdown(t *testing.T) {
	svc, driveMock, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	copier := uuid.NewString()
	imageBytes := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x01, 0x02}

	src, err := svc.Create(ctx, author, "Con imagen", nil, "public", stringPtr("texto"), "")
	if err != nil {
		t.Fatalf("create origen: %v", err)
	}
	att, err := svc.AddAttachment(ctx, author, src.ID, "foto.png", "image/png", imageBytes, true)
	if err != nil {
		t.Fatalf("AddAttachment: %v", err)
	}
	content := "antes\n\n![foto](attachment:" + att.ID + ")\n\ndespués"
	if _, err := svc.Update(ctx, author, src.ID, nil, nil, &content, ""); err != nil {
		t.Fatalf("update contenido: %v", err)
	}

	clone, err := svc.Copy(ctx, copier, src.ID, "copy-attachments")
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	if clone.ExternalFileID == nil || *clone.ExternalFileID == *src.ExternalFileID {
		t.Fatal("el clon debe tener su propio .md en Drive")
	}
	cloneContent, err := driveMock.GetFileContent(ctx, copier, *clone.ExternalFileID)
	if err != nil {
		t.Fatalf("leer .md clonado: %v", err)
	}
	if strings.Contains(cloneContent, "attachment:"+att.ID) {
		t.Fatalf("el Markdown clonado no debe conservar el adjunto del autor: %q", cloneContent)
	}

	cloneAtts, err := svc.ListAttachments(ctx, clone.ID)
	if err != nil {
		t.Fatalf("ListAttachments clon: %v", err)
	}
	if len(cloneAtts) != 1 {
		t.Fatalf("el clon debe tener 1 adjunto, got %d", len(cloneAtts))
	}
	cloneAtt := cloneAtts[0]
	if cloneAtt.ID == att.ID {
		t.Fatal("el adjunto clonado debe ser una fila nueva")
	}
	if cloneAtt.ExternalFileID == att.ExternalFileID {
		t.Fatal("el adjunto clonado debe ser un archivo nuevo en Drive")
	}
	if !strings.Contains(cloneContent, "attachment:"+cloneAtt.ID) {
		t.Fatalf("el Markdown clonado debe referenciar el adjunto nuevo %s: %q", cloneAtt.ID, cloneContent)
	}

	data, mime, gotAtt, err := svc.GetAttachmentContent(ctx, copier, clone.ID, cloneAtt.ID)
	if err != nil {
		t.Fatalf("contenido del adjunto clonado: %v", err)
	}
	if gotAtt == nil || gotAtt.ID != cloneAtt.ID {
		t.Fatalf("adjunto devuelto inesperado: %+v", gotAtt)
	}
	if mime != "image/png" || string(data) != string(imageBytes) {
		t.Fatalf("binario clonado alterado: mime=%q bytes=%v", mime, data)
	}

	// El adjunto del autor no queda accesible a través del clon (pertenece al
	// apunte original), y el original tampoco expone el adjunto clonado.
	if _, _, _, err := svc.GetAttachmentContent(ctx, copier, clone.ID, att.ID); err == nil {
		t.Fatal("el clon no debe servir adjuntos del apunte original")
	}
	if _, _, _, err := svc.GetAttachmentContent(ctx, author, src.ID, cloneAtt.ID); err == nil {
		t.Fatal("el original no debe servir adjuntos del clon")
	}
}

// Copiar un apunte con PDF conserva los bytes y el MIME del adjunto, y el clon
// privado no queda accesible a terceros ni al autor del apunte original. Es el
// caso que en la app se abre como «documento adjunto» desde el visor.
func TestCopyPDFAttachmentPreservesBytesAndMIMEType(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	copier := uuid.NewString()
	third := uuid.NewString()
	pdfBytes := []byte("%PDF-1.4\n1 0 obj << /Type /Catalog >> endobj\n%%EOF")

	src, err := svc.Create(ctx, author, "Apunte con PDF", nil, "public", stringPtr("texto"), "")
	if err != nil {
		t.Fatalf("create origen: %v", err)
	}
	att, err := svc.AddAttachment(ctx, author, src.ID, "guia.pdf", "application/pdf", pdfBytes, false)
	if err != nil {
		t.Fatalf("AddAttachment PDF: %v", err)
	}
	content := "Documento adjunto: [Guia PDF](attachment:" + att.ID + ")"
	if _, err := svc.Update(ctx, author, src.ID, nil, nil, &content, ""); err != nil {
		t.Fatalf("update contenido: %v", err)
	}

	clone, err := svc.Copy(ctx, copier, src.ID, "copy-pdf")
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	cloneAtts, err := svc.ListAttachments(ctx, clone.ID)
	if err != nil {
		t.Fatalf("ListAttachments clon: %v", err)
	}
	if len(cloneAtts) != 1 {
		t.Fatalf("el clon debe tener 1 adjunto, got %d", len(cloneAtts))
	}
	cloneAtt := cloneAtts[0]
	if cloneAtt.FileType != "application/pdf" || cloneAtt.FileName == nil || *cloneAtt.FileName != "guia.pdf" {
		t.Fatalf("metadatos del PDF clonado alterados: %+v", cloneAtt)
	}

	data, mime, gotAtt, err := svc.GetAttachmentContent(ctx, copier, clone.ID, cloneAtt.ID)
	if err != nil {
		t.Fatalf("contenido del PDF clonado: %v", err)
	}
	if mime != "application/pdf" || !bytes.Equal(data, pdfBytes) {
		t.Fatalf("PDF clonado alterado: mime=%q bytes=%d", mime, len(data))
	}
	if gotAtt == nil || gotAtt.ID != cloneAtt.ID {
		t.Fatalf("adjunto devuelto inesperado: %+v", gotAtt)
	}

	// El clon nace privado: ni un tercero ni el autor del original pueden leer
	// sus bytes aunque conozcan los UUID.
	if _, _, _, err := svc.GetAttachmentContent(ctx, third, clone.ID, cloneAtt.ID); err == nil {
		t.Fatal("un tercero no debe leer el PDF de un clon privado")
	}
	if _, _, _, err := svc.GetAttachmentContent(ctx, author, clone.ID, cloneAtt.ID); err == nil {
		t.Fatal("el autor del original no debe leer el PDF del clon ajeno")
	}

	// Borrar la copia elimina su acceso y permite una nueva copia con su propio
	// adjunto, sin reutilizar la fila eliminada.
	if err := svc.Delete(ctx, copier, clone.ID); err != nil {
		t.Fatalf("delete del clon: %v", err)
	}
	if _, _, _, err := svc.GetAttachmentContent(ctx, copier, clone.ID, cloneAtt.ID); err == nil {
		t.Fatal("el adjunto del clon eliminado no debe seguir sirviéndose")
	}
	second, err := svc.Copy(ctx, copier, src.ID, "copy-pdf-again")
	if err != nil {
		t.Fatalf("recopy tras borrar: %v", err)
	}
	secondAtts, err := svc.ListAttachments(ctx, second.ID)
	if err != nil {
		t.Fatalf("ListAttachments segunda copia: %v", err)
	}
	if len(secondAtts) != 1 || secondAtts[0].ID == cloneAtt.ID {
		t.Fatalf("la segunda copia debe tener un adjunto nuevo: %+v", secondAtts)
	}
	data, mime, _, err = svc.GetAttachmentContent(ctx, copier, second.ID, secondAtts[0].ID)
	if err != nil || mime != "application/pdf" || !bytes.Equal(data, pdfBytes) {
		t.Fatalf("PDF de la segunda copia alterado: mime=%q err=%v", mime, err)
	}
}
