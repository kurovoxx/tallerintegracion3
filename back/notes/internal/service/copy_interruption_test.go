package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/utils"
)

// Una copia iniciada no depende de que el cliente siga esperando. Antes, al
// volver atrás en la app se cancelaba la petición, CopyFile abortaba con
// «context canceled», también fallaba marcar 'failed_sync' y el clon quedaba
// 'pending_drive' para siempre, bloqueado como «ya copiado».

// cancelMidCopy cancela el contexto de la petición justo cuando empieza la
// copia en Drive (como cuando la app cierra la conexión) y comprueba que el
// trabajo de Notes sigue con un contexto vivo.
type cancelMidCopy struct {
	*drive.MockClient
	cancel      context.CancelFunc
	ctxAliveErr error
}

func (c *cancelMidCopy) CopyFile(ctx context.Context, srcUserID, srcFileID, dstUserID, newNoteID, newTitle string) (string, error) {
	c.cancel()
	time.Sleep(10 * time.Millisecond)
	c.ctxAliveErr = ctx.Err()
	return c.MockClient.CopyFile(ctx, srcUserID, srcFileID, dstUserID, newNoteID, newTitle)
}

func TestCopyContinuesWhenClientDisconnects(t *testing.T) {
	_, driveMock, noteStore, attStore, savedStore, likeStore, sharedStore, social := newTestService()
	wrapped := &cancelMidCopy{MockClient: driveMock}
	svc := NewNoteService(noteStore, attStore, savedStore, likeStore, sharedStore, wrapped, social)
	author, copier := uuid.NewString(), uuid.NewString()
	src, err := svc.Create(context.Background(), author, "Origen", nil, "public", stringPtr("# Hola"), "")
	if err != nil {
		t.Fatal(err)
	}

	reqCtx, cancel := context.WithCancel(context.Background())
	wrapped.cancel = cancel
	clone, err := svc.Copy(reqCtx, copier, src.ID, "k-back")
	if err != nil {
		t.Fatalf("la copia debe terminar aunque el cliente se vaya: %v", err)
	}
	if wrapped.ctxAliveErr != nil {
		t.Fatalf("Drive recibió un contexto cancelado: %v", wrapped.ctxAliveErr)
	}
	stored, _ := noteStore.GetByID(context.Background(), clone.ID)
	if stored == nil || stored.SyncStatus != "synced" || stored.ExternalFileID == nil {
		t.Fatalf("el clon debe quedar sincronizado y con archivo: %+v", stored)
	}
	// La respuesta tardía/reintento con la misma clave devuelve el mismo clon.
	again, err := svc.Copy(context.Background(), copier, src.ID, "k-back")
	if err != nil || again.ID != clone.ID {
		t.Fatalf("el reintento debe devolver el mismo clon: %v %+v", err, again)
	}
}

func pendingClone(t *testing.T, store *MemoryNoteStore, owner, origID, title string, age time.Duration) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := store.Create(context.Background(), id, owner, nil, title, nil, "private", &origID, "pending_drive"); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	store.notes[id].UpdatedAt = time.Now().UTC().Add(-age)
	store.mu.Unlock()
	return id
}

func TestCopyRecentPendingIsInProgressNotCopied(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author, copier := uuid.NewString(), uuid.NewString()
	src, err := svc.Create(ctx, author, "Origen", nil, "public", stringPtr("x"), "")
	if err != nil {
		t.Fatal(err)
	}
	pendingID := pendingClone(t, noteStore, copier, src.ID, src.Title, 5*time.Second)
	files, notes := driveMock.FileCount(), countNotes(noteStore)

	_, err = svc.Copy(ctx, copier, src.ID, "")
	var se *ServiceError
	if !errors.As(err, &se) || se.Code != utils.ErrCopyInProgress {
		t.Fatalf("una copia en curso debe responder copy_in_progress, got %v", err)
	}
	if utils.StatusForCode(se.Code) != 409 {
		t.Fatalf("copy_in_progress debe ser 409")
	}
	if driveMock.FileCount() != files || countNotes(noteStore) != notes {
		t.Fatal("no debe duplicar ni tocar la copia en curso")
	}
	if p, _ := noteStore.GetByID(ctx, pendingID); p == nil || p.SyncStatus != "pending_drive" {
		t.Fatalf("la copia en curso no debe alterarse: %+v", p)
	}
}

func TestCopyStalePendingIsRecoveredOnRetry(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author, copier := uuid.NewString(), uuid.NewString()
	src, err := svc.Create(ctx, author, "Origen", nil, "public", stringPtr("# Hola"), "")
	if err != nil {
		t.Fatal(err)
	}
	// Clon interrumpido antes del arreglo: 'pending_drive' sin archivo, viejo.
	stuckID := pendingClone(t, noteStore, copier, src.ID, src.Title, copyStaleAfter+time.Minute)
	notes := countNotes(noteStore)

	clone, err := svc.Copy(ctx, copier, src.ID, "")
	if err != nil {
		t.Fatalf("una copia interrumpida debe poder reintentarse: %v", err)
	}
	if clone.ID != stuckID || clone.SyncStatus != "synced" || clone.ExternalFileID == nil {
		t.Fatalf("debe completar el mismo clon (sin duplicar): %+v", clone)
	}
	if countNotes(noteStore) != notes {
		t.Fatal("no debe crear un segundo clon")
	}
	// Completa: ahora sí es «ya copiado».
	_, err = svc.Copy(ctx, copier, src.ID, "")
	var se *ServiceError
	if !errors.As(err, &se) || se.Code != utils.ErrAlreadyCopied {
		t.Fatalf("tras completarse debe ser already_copied, got %v", err)
	}
	if driveMock.FileCount() == 0 {
		t.Fatal("debe existir el archivo del clon")
	}
}

// Fallo ambiguo (timeout/5xx): la fila queda 'failed_sync' y el siguiente
// intento la completa sin duplicar.
func TestCopyAmbiguousFailureIsRetryable(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author, copier := uuid.NewString(), uuid.NewString()
	src, err := svc.Create(ctx, author, "Origen", nil, "public", stringPtr("# Hola"), "")
	if err != nil {
		t.Fatal(err)
	}
	driveMock.CopyErr = &drive.DriveError{Code: 503, Message: "no disponible"}
	_, err = svc.Copy(ctx, copier, src.ID, "")
	if !errors.Is(err, ErrDriveUnavailable) {
		t.Fatalf("esperaba ErrDriveUnavailable, got %v", err)
	}
	mine, _, _ := noteStore.ListByUser(ctx, copier, "", 10)
	if len(mine) != 1 || mine[0].SyncStatus != "failed_sync" || mine[0].ExternalFileID != nil {
		t.Fatalf("debe quedar un único clon failed_sync sin archivo: %+v", mine)
	}
	driveMock.CopyErr = nil
	clone, err := svc.Copy(ctx, copier, src.ID, "")
	if err != nil || clone.ID != mine[0].ID || clone.SyncStatus != "synced" {
		t.Fatalf("el reintento debe completar el mismo clon: %v %+v", err, clone)
	}
}
