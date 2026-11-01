package service

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
)

// --- XSS completo: sanitizeMarkdown ---

// sanitizeMarkdown debe neutralizar la cobertura completa de vectores XSS:
// <script>, <iframe>, <svg>, <object>, <embed> (con cualquier atributo y caso),
// atributos de evento on\w+= (onclick, onmouseover, ondblclick, ...) y los
// esquemas javascript:, data:, vbscript: además de expression(...) en CSS.
func TestSanitizeMarkdownFullXSSCoverage(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		forbid  []string
		contain []string
	}{
		{
			name:    "tags svg object embed",
			input:   "a<svg onload=alert(1)><circle/></svg>b<object data=x></object>c<embed src=y>d",
			forbid:  []string{"<svg", "</svg", "svg>", "<object", "object>", "<embed", "embed>", "onload="},
			contain: []string{"&lt;svg&gt;", "&lt;object&gt;", "&lt;embed&gt;"},
		},
		{
			name:    "evento onclick en imagen",
			input:   `<img src=x onclick="alert(1)" ondblclick=evil>`,
			forbid:  []string{"onclick=", "ondblclick=", "onclick =", "ondblclick ="},
			contain: []string{"blocked="},
		},
		{
			name:    "eventos onmouseover onfocus variados",
			input:   "x onmouseover=evil y onfocus = evil z onmousemove=1",
			forbid:  []string{"onmouseover=", "onfocus =", "onmousemove="},
			contain: []string{"blocked="},
		},
		{
			name:    "onload dentro de tag no bloqueado",
			input:   `<body onload = "alert(1)">hola</body>`,
			forbid:  []string{"onload =", "onload="},
			contain: []string{"blocked="},
		},
		{
			name:    "esquemas data y vbscript",
			input:   `[a](data:text/html;base64,PHNjcmlwdD4=) y [b](VbScRiPt:msgbox(1))`,
			forbid:  []string{"data:", "VbScRiPt:", "vbscript:"},
			contain: []string{"blocked:"},
		},
		{
			name:    "expresion CSS",
			input:   `style="width: expression(alert(1))" y expression ( x )`,
			forbid:  []string{"expression("},
			contain: []string{"blocked("},
		},
		{
			name:    "cualquier on+w= generico",
			input:   `<a onanything="x">y</a>`,
			forbid:  []string{"onanything="},
			contain: []string{"blocked="},
		},
		{
			name:    "markdown normal intacto",
			input:   "# Titulo\n\n[link](https://ejemplo.com) y **negrita**",
			forbid:  []string{"&lt;", "blocked", "on"},
			contain: []string{"# Titulo", "[link](https://ejemplo.com)"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeMarkdown(tc.input)
			for _, f := range tc.forbid {
				if strings.Contains(got, f) {
					t.Fatalf("sanitizeMarkdown(%q) = %q no debe contener %q", tc.input, got, f)
				}
			}
			for _, c := range tc.contain {
				if !strings.Contains(got, c) {
					t.Fatalf("sanitizeMarkdown(%q) = %q debe contener %q", tc.input, got, c)
				}
			}
		})
	}
}

// --- X-Idempotency-Key en Update (servicio) ---

// Un replay de red con la misma X-Idempotency-Key y el mismo payload devuelve
// el resultado cacheado sin volver a mutar PG/Drive; la misma clave con un body
// distinto es 409 Conflict; un error purga la clave permitiendo el reintento.
func TestUpdateIdempotencyReplayAndErrorCleanup(t *testing.T) {
	svc, driveMock, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	key := "update-spec95-1"
	note, err := svc.Create(ctx, author, "Idem Update", nil, "private", stringPtr("v1"), "")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	fileID := *note.ExternalFileID

	// Primera actualización con clave: debe aplicar y cachear el resultado.
	v2 := "v2"
	upd1, err := svc.Update(ctx, author, note.ID, stringPtr("Título v2"), nil, &v2, key)
	if err != nil {
		t.Fatalf("primer update con key failed: %v", err)
	}

	// Replay con la misma clave y el mismo payload: responde desde el registro
	// 'completed' con exactamente el resultado anterior y no vuelve a tocar Drive.
	v2Replay := "v2"
	upd2, err := svc.Update(ctx, author, note.ID, stringPtr("Título v2"), nil, &v2Replay, key)
	if err != nil {
		t.Fatalf("replay update failed: %v", err)
	}
	if upd2.ID != upd1.ID {
		t.Fatalf("replay debe devolver el mismo registro cacheado: %s vs %s", upd2.ID, upd1.ID)
	}
	if upd2.Title != "Título v2" {
		t.Fatalf("replay debe devolver la respuesta cacheada (Título v2), got %q", upd2.Title)
	}
	got, err := driveMock.GetFileContent(ctx, author, fileID)
	if err != nil {
		t.Fatalf("lectura Drive failed: %v", err)
	}
	if got != "v2" {
		t.Fatalf("Drive no debe re-escribirse en un replay, got %q", got)
	}

	// Misma clave con un body distinto: 409 Conflict sin re-escribir Drive.
	v3 := "v3 que no debe persistir"
	_, err = svc.Update(ctx, author, note.ID, stringPtr("Título v3"), nil, &v3, key)
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "conflict" {
		t.Fatalf("misma clave con body distinto debe dar 409 conflict, got %v", err)
	}
	if got, _ := driveMock.GetFileContent(ctx, author, fileID); got != "v2" {
		t.Fatalf("el conflicto no debe re-escribir Drive, got %q", got)
	}

	// Error con otra clave: la clave durable se libera y el reintento con la
	// misma clave (tras el fallo) vuelve a ejecutarse sin quedar bloqueada.
	badKey := "update-spec95-fail"
	missing := uuid.NewString()
	if _, err := svc.Update(ctx, author, missing, stringPtr("x"), nil, nil, badKey); err == nil {
		t.Fatal("update de nota inexistente debe fallar")
	}
	if rec, loaded := idemRecordStored(svc, author, "update", badKey); loaded {
		t.Fatalf("la clave de un update fallido debe liberarse para permitir reintento, got %+v", rec)
	}
	ok2, err := svc.Update(ctx, author, note.ID, stringPtr("Título v4"), nil, &v3, badKey)
	if err != nil {
		t.Fatalf("reintento con la misma clave tras el fallo debe proceder: %v", err)
	}
	if ok2.Title != "Título v4" {
		t.Fatalf("el reintento debe aplicar el cambio, got %q", ok2.Title)
	}
}

// El mismo contrato durable vale para el estado in_progress de Update: una
// solicitud concurrente con la misma clave se rechaza con 409 "solicitud en
// progreso" y no toca el storage.
func TestUpdateIdempotencyInFlightRejectsConcurrent(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	note, err := svc.Create(ctx, author, "In-flight update", nil, "private", stringPtr("v0"), "")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	key := "update-spec95-inflight"
	seedIdemClaim(svc, author, "update", key, "hash-en-vuelo", model.IdempotencyStatusInProgress, time.Now().Add(idemClaimTTL))
	beforeFiles := driveMock.FileCount()
	body := "v1"
	_, err = svc.Update(ctx, author, note.ID, stringPtr("T"), nil, &body, key)
	if err == nil {
		t.Fatal("clave in-flight debe rechazar la actualización concurrente")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "conflict" || se.Message != "solicitud en progreso" {
		t.Fatalf("esperaba conflict/solicitud en progreso, got %v", err)
	}
	if driveMock.FileCount() != beforeFiles {
		t.Fatal("la solicitud rechazada por in-flight no debe tener efectos")
	}
	stored, _ := noteStore.GetByID(ctx, note.ID)
	if stored.Title != "In-flight update" {
		t.Fatalf("PG no debe mutar con in-flight vigente, got %q", stored.Title)
	}
}

// --- ReconcilePendingNotes: mutex de reconciliación ---

// countingNoteStore cuenta las invocaciones de Delete para verificar que la
// serialización por mutex evita dobles compensaciones.
type countingNoteStore struct {
	*MemoryNoteStore
	mu      sync.Mutex
	deletes int
}

func (c *countingNoteStore) Delete(ctx context.Context, id string) error {
	c.mu.Lock()
	c.deletes++
	c.mu.Unlock()
	return c.MemoryNoteStore.Delete(ctx, id)
}

// Concurrent reconciliation preserves valid empty Markdown and repairs status.
func TestReconcilePendingNotesConcurrentSerializedByMutex(t *testing.T) {
	driveMock := drive.NewMockClient()
	base := &countingNoteStore{MemoryNoteStore: NewMemoryNoteStore()}
	likeStore := NewMemoryLikeStore()
	likeStore.SetNoteStore(base.MemoryNoteStore)
	svc := NewNoteService(base, NewMemoryAttachmentStore(), NewMemorySavedStore(), likeStore, NewMemorySharedStore(), driveMock, NewMemorySocialResolver())
	ctx := context.Background()
	userID := uuid.NewString()

	// Empty Markdown is a valid note and must never trigger compensation.
	fileID, _ := driveMock.CreateFile(ctx, userID, "", "concurrent.md", "")
	n, err := base.Create(ctx, "", userID, nil, "concurrent", &fileID, "private", nil, "pending_drive")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	old := time.Now().UTC().Add(-20 * time.Minute)
	base.mu.Lock()
	base.notes[n.ID].CreatedAt = old
	base.notes[n.ID].UpdatedAt = old
	base.mu.Unlock()

	const workers = 8
	var wg sync.WaitGroup
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = svc.ReconcilePendingNotes(ctx)
		}(i)
	}
	wg.Wait()
	for i, e := range errs {
		if e != nil {
			t.Fatalf("reconcile %d falló: %v", i, e)
		}
	}
	if got, _ := base.GetByID(ctx, n.ID); got == nil || got.SyncStatus != "synced" {
		t.Fatal("empty Markdown must survive and become synced")
	}
	if !driveMock.HasFile(fileID) {
		t.Fatal("empty Markdown file must survive reconciliation")
	}
	base.mu.Lock()
	defer base.mu.Unlock()
	if base.deletes != 0 {
		t.Fatalf("empty Markdown must never be deleted, got %d deletes", base.deletes)
	}
}
