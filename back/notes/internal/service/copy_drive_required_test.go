package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
)

// Copiar un apunte ajeno exige el Google Drive propio: sin él no se puede
// completar y antes quedaba un clon 'failed_sync' sin archivo («copiado»
// pero ilegible). Estas pruebas fijan la regla nueva.

func TestCopyWithoutDrive_RejectedBeforeAnySideEffect(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author, copier := uuid.NewString(), uuid.NewString()
	note, err := svc.Create(ctx, author, "Origen", nil, "public", stringPtr("# Hola"), "")
	if err != nil {
		t.Fatal(err)
	}
	files, notes := driveMock.FileCount(), countNotes(noteStore)
	driveMock.Disconnect(copier)

	// Con y sin clave de idempotencia, repetido: nunca deja rastro.
	for _, key := range []string{"", "k1", "k1"} {
		_, err := svc.Copy(ctx, copier, note.ID, key)
		var se *ServiceError
		if !errors.As(err, &se) || se.Code != "drive_not_connected" {
			t.Fatalf("clave %q: esperaba drive_not_connected, got %v", key, err)
		}
	}
	if driveMock.FileCount() != files || countNotes(noteStore) != notes {
		t.Fatalf("sin Drive no debe crear archivos ni filas")
	}

	// Al conectar Drive, la misma clave funciona y la copia es real.
	driveMock.Reconnect(copier)
	clone, err := svc.Copy(ctx, copier, note.ID, "k1")
	if err != nil {
		t.Fatalf("con Drive la copia debe completarse: %v", err)
	}
	if clone.SyncStatus != "synced" || clone.ExternalFileID == nil || *clone.ExternalFileID == "" {
		t.Fatalf("la copia debe quedar sincronizada con archivo: %+v", clone)
	}
}

func TestCopyWithoutDrive_PermissionsStillFirst(t *testing.T) {
	svc, driveMock, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author, copier := uuid.NewString(), uuid.NewString()
	private, err := svc.Create(ctx, author, "Privado", nil, "private", stringPtr("x"), "")
	if err != nil {
		t.Fatal(err)
	}
	driveMock.Disconnect(copier)
	// Apunte privado ajeno o inexistente: 404 canónico aunque falte Drive
	// (no se revela nada del recurso ni del estado de Drive).
	for _, id := range []string{private.ID, uuid.NewString()} {
		_, err := svc.Copy(ctx, copier, id, "")
		var se *ServiceError
		if !errors.As(err, &se) || se.Code != "not_found" {
			t.Fatalf("esperaba not_found, got %v", err)
		}
	}
}

// revokeDuringCopy simula una credencial revocada entre la comprobación previa
// y la copia: CheckAccess dice que hay Drive, pero CopyFile falla por OAuth.
type revokeDuringCopy struct{ *drive.MockClient }

func (revokeDuringCopy) CheckAccess(context.Context, string) error { return nil }

func TestCopyOAuthFailureMidway_NoGhostClone(t *testing.T) {
	_, driveMock, noteStore, attStore, savedStore, likeStore, sharedStore, social := newTestService()
	svc := NewNoteService(noteStore, attStore, savedStore, likeStore, sharedStore, revokeDuringCopy{driveMock}, social)
	ctx := context.Background()
	author, copier := uuid.NewString(), uuid.NewString()
	note, err := svc.Create(ctx, author, "Origen", nil, "public", stringPtr("# Hola"), "")
	if err != nil {
		t.Fatal(err)
	}
	notes := countNotes(noteStore)
	driveMock.Disconnect(copier) // CopyFile fallará por OAuth

	_, err = svc.Copy(ctx, copier, note.ID, "k2")
	var se *ServiceError
	if !errors.As(err, &se) || se.Code != "drive_not_connected" {
		t.Fatalf("esperaba drive_not_connected, got %v", err)
	}
	if countNotes(noteStore) != notes {
		t.Fatalf("el clon recién creado sin archivo debe eliminarse")
	}
	driveMock.Reconnect(copier)
	if _, err := svc.Copy(ctx, copier, note.ID, "k2"); err != nil {
		t.Fatalf("tras reconectar, la misma clave debe poder copiar: %v", err)
	}
}

// Clones 'failed_sync' heredados de antes del arreglo: no se borran (no se
// sabe si su archivo llegó a Drive); se completan al reintentar con Drive.
func TestCopyLegacyFailedClone_KeptAndCompletedWhenDriveConnected(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author, copier := uuid.NewString(), uuid.NewString()
	note, err := svc.Create(ctx, author, "Origen", nil, "public", stringPtr("# Hola"), "")
	if err != nil {
		t.Fatal(err)
	}
	ghostID := uuid.NewString()
	if _, err := noteStore.Create(ctx, ghostID, copier, nil, note.Title, nil, "private", &note.ID, "failed_sync"); err != nil {
		t.Fatal(err)
	}
	driveMock.Disconnect(copier)
	if _, err := svc.Copy(ctx, copier, note.ID, ""); err == nil {
		t.Fatal("sin Drive debe rechazarse")
	}
	if g, _ := noteStore.GetByID(ctx, ghostID); g == nil || g.SyncStatus != "failed_sync" {
		t.Fatalf("el clon heredado debe conservarse intacto: %+v", g)
	}
	driveMock.Reconnect(copier)
	clone, err := svc.Copy(ctx, copier, note.ID, "")
	if err != nil {
		t.Fatalf("con Drive debe completarse: %v", err)
	}
	if clone.ID != ghostID || clone.SyncStatus != "synced" || clone.ExternalFileID == nil {
		t.Fatalf("debe completar el clon heredado (mismo id) y sincronizarlo: %+v", clone)
	}
}

// identityDown simula Identity caído: no se puede saber si hay Drive.
type identityDown struct{ *drive.MockClient }

func (identityDown) CheckAccess(context.Context, string) error {
	return &drive.DriveError{Code: 500, Message: "Servicio de almacenamiento no disponible"}
}

func TestCopyDriveStatusUnknown_ServiceUnavailableNotNotConnected(t *testing.T) {
	_, driveMock, noteStore, attStore, savedStore, likeStore, sharedStore, social := newTestService()
	svc := NewNoteService(noteStore, attStore, savedStore, likeStore, sharedStore, identityDown{driveMock}, social)
	ctx := context.Background()
	note, err := svc.Create(ctx, uuid.NewString(), "Origen", nil, "public", stringPtr("x"), "")
	if err != nil {
		t.Fatal(err)
	}
	notes := countNotes(noteStore)
	_, err = svc.Copy(ctx, uuid.NewString(), note.ID, "")
	if !errors.Is(err, ErrDriveUnavailable) {
		t.Fatalf("esperaba ErrDriveUnavailable (503), got %v", err)
	}
	if countNotes(noteStore) != notes {
		t.Fatal("no debe crear filas si no se pudo comprobar Drive")
	}
}
