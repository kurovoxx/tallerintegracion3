package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
)

// publicNoteID genera UUIDs deterministas y válidos para poder comprobar el
// desempate id DESC sin depender del orden aleatorio de uuid.NewString().
func publicNoteID(n int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", n)
}

// testAuthorPublicID es el public_id de prueba que el resolver traduce al
// autor consultado en cada caso.
const testAuthorPublicID = "a8kp92mqwx7r"

// listPublicByAuthor recorre el listado público real (GET
// /public-users/:public_id/notes) para el autor indicado: un resolver de
// prueba traduce testAuthorPublicID a su UUID con perfil público y el
// solicitante es un tercero. Así estas pruebas cubren el filtrado, el orden y
// la paginación del store a través del único endpoint expuesto.
func listPublicByAuthor(ctx context.Context, svc *NoteService, authorID, cursor string, limit int) ([]*model.PublicProfileNoteView, string, error) {
	svc.SetPublicProfileResolver(&fakePublicProfileResolver{ids: map[string]string{testAuthorPublicID: authorID}})
	return svc.ListPublicByPublicID(ctx, uuid.NewString(), testAuthorPublicID, cursor, limit)
}

// seedPublicListNote inserta una nota directamente en el store en memoria para
// controlar visibility y created_at sin pasar por Drive.
func seedPublicListNote(t *testing.T, store *MemoryNoteStore, id, userID, title, visibility string, createdAt time.Time) *model.Note {
	t.Helper()
	external := "drive-" + id
	sync := "synced"
	forked := "00000000-0000-4000-8000-999999999999"
	n := &model.Note{
		ID:               id,
		UserID:           userID,
		SubjectID:        nil,
		Title:            title,
		ExternalFileID:   &external,
		Visibility:       visibility,
		LikesCount:       7,
		ForkedFromNoteID: &forked,
		SyncStatus:       sync,
		Version:          3,
		CreatedAt:        createdAt,
		UpdatedAt:        createdAt.Add(time.Minute),
		Content:          stringPtr("# contenido privado"),
	}
	store.mu.Lock()
	store.notes[id] = n
	store.mu.Unlock()
	return n
}

// seedPublicListNoteSinArchivo crea una nota pública sin external_file_id:
// representa metadata pública cuyo contenido no está disponible en Drive.
func seedPublicListNoteSinArchivo(t *testing.T, store *MemoryNoteStore, id, userID, title string, createdAt time.Time) *model.Note {
	t.Helper()
	n := seedPublicListNote(t, store, id, userID, title, "public", createdAt)
	store.mu.Lock()
	n.ExternalFileID = nil
	store.mu.Unlock()
	return n
}

func TestListPublicByUser_SoloPublicosYExcluyeCompartidosPrivados(t *testing.T) {
	svc, _, noteStore, _, _, _, shared, social := newTestService()
	ctx := context.Background()
	owner := uuid.NewString()
	group := uuid.NewString()
	social.AddMember(owner, group)
	now := time.Now().UTC()

	pub := seedPublicListNote(t, noteStore, publicNoteID(1), owner, "Público", "public", now)
	priv := seedPublicListNote(t, noteStore, publicNoteID(2), owner, "Privado", "private", now.Add(time.Second))
	other := seedPublicListNote(t, noteStore, publicNoteID(3), uuid.NewString(), "Otro autor", "public", now.Add(2*time.Second))

	// Aunque el apunte privado esté compartido por enlace con un grupo, el
	// listado público debe excluirlo: no reutiliza las reglas de lectura.
	if _, err := shared.Create(ctx, priv.ID, group, false, "link", 0); err != nil {
		t.Fatalf("crear share: %v", err)
	}

	notes, next, err := listPublicByAuthor(ctx, svc, owner, "", 20)
	if err != nil {
		t.Fatalf("esperado sin error, got %v", err)
	}
	if next != "" {
		t.Fatalf("next_cursor = %q, want vacío", next)
	}
	if len(notes) != 1 || notes[0].ID != pub.ID {
		t.Fatalf("solo se esperaba el apunte público %s, got %+v", pub.ID, notes)
	}
	if notes[0].Visibility != "public" {
		t.Fatalf("visibility = %q, want public", notes[0].Visibility)
	}
	for _, n := range notes {
		if n.ID == priv.ID || n.ID == other.ID {
			t.Fatalf("se filtró una nota que no corresponde: %+v", n)
		}
	}
}

// Un apunte público sin archivo Drive no es listable: el perfil solo muestra
// apuntes que se pueden abrir. El caso privado con archivo tampoco aparece.
func TestListPublicByUser_ExcluyePublicosSinArchivoDrive(t *testing.T) {
	svc, _, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	owner := uuid.NewString()
	now := time.Now().UTC()

	conArchivo := seedPublicListNote(t, noteStore, publicNoteID(1), owner, "Público con archivo", "public", now)
	sinArchivo := seedPublicListNoteSinArchivo(t, noteStore, publicNoteID(2), owner, "Público sin archivo", now.Add(time.Second))
	privada := seedPublicListNote(t, noteStore, publicNoteID(3), owner, "Privada con archivo", "private", now.Add(2*time.Second))

	notes, next, err := listPublicByAuthor(ctx, svc, owner, "", 20)
	if err != nil {
		t.Fatalf("esperado sin error, got %v", err)
	}
	if next != "" {
		t.Fatalf("next_cursor = %q, want vacío", next)
	}
	if len(notes) != 1 || notes[0].ID != conArchivo.ID {
		t.Fatalf("solo se esperaba el apunte público con archivo %s, got %+v", conArchivo.ID, notes)
	}
	for _, n := range notes {
		if n.ID == sinArchivo.ID || n.ID == privada.ID {
			t.Fatalf("se filtró una nota no listable: %+v", n)
		}
	}
}

// La paginación debe ignorar por completo los apuntes sin archivo: no consumen
// cupo y next_cursor solo apunta a filas que cumplen todos los filtros.
func TestListPublicByUser_PaginacionIgnoraSinArchivoYCalculaNextCursor(t *testing.T) {
	svc, _, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	owner := uuid.NewString()
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// No listables: uno más nuevo que todos y otro entre la segunda y la tercera
	// fila listable. Si la paginación los contara, next_cursor sería incorrecto.
	seedPublicListNoteSinArchivo(t, noteStore, publicNoteID(90), owner, "Sin archivo reciente", base.Add(10*time.Minute))
	seedPublicListNoteSinArchivo(t, noteStore, publicNoteID(91), owner, "Sin archivo intermedio", base.Add(210*time.Second))

	ids := make([]string, 0, 5)
	for i := 1; i <= 5; i++ {
		created := base.Add(time.Duration(6-i) * time.Minute)
		n := seedPublicListNote(t, noteStore, publicNoteID(i), owner, fmt.Sprintf("Nota %d", i), "public", created)
		ids = append(ids, n.ID)
	}

	page1, next1, err := listPublicByAuthor(ctx, svc, owner, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page1) != 2 || page1[0].ID != ids[0] || page1[1].ID != ids[1] {
		t.Fatalf("página 1 inesperada: %+v", page1)
	}
	if next1 != ids[1] {
		t.Fatalf("next_cursor = %q, want %q (última fila listable entregada)", next1, ids[1])
	}

	page2, next2, err := listPublicByAuthor(ctx, svc, owner, next1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 2 || page2[0].ID != ids[2] || page2[1].ID != ids[3] {
		t.Fatalf("página 2 inesperada: %+v", page2)
	}
	if next2 != ids[3] {
		t.Fatalf("next_cursor 2 = %q, want %q", next2, ids[3])
	}

	page3, next3, err := listPublicByAuthor(ctx, svc, owner, next2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page3) != 1 || page3[0].ID != ids[4] || next3 != "" {
		t.Fatalf("página 3 inesperada: %+v next=%q", page3, next3)
	}

	// Un cursor que apunta a un apunte sin archivo no es válido: fail-closed.
	notes, next, err := listPublicByAuthor(ctx, svc, owner, publicNoteID(91), 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 0 || next != "" {
		t.Fatalf("cursor sin archivo debe producir lista vacía, got %+v next=%q", notes, next)
	}
}

func TestListPublicByUser_PaginacionVariasPaginas(t *testing.T) {
	svc, _, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	owner := uuid.NewString()
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	ids := make([]string, 0, 5)
	for i := 1; i <= 5; i++ {
		// created_at descendente: n1 es el más reciente.
		created := base.Add(time.Duration(6-i) * time.Minute)
		n := seedPublicListNote(t, noteStore, publicNoteID(i), owner, fmt.Sprintf("Nota %d", i), "public", created)
		ids = append(ids, n.ID)
	}

	var got []string
	cursor := ""
	for page := 0; page < 4; page++ {
		notes, next, err := listPublicByAuthor(ctx, svc, owner, cursor, 2)
		if err != nil {
			t.Fatalf("página %d: %v", page, err)
		}
		for _, n := range notes {
			got = append(got, n.ID)
		}
		if next == "" {
			break
		}
		cursor = next
	}
	if len(got) != len(ids) {
		t.Fatalf("se esperaban %d ids, got %v", len(ids), got)
	}
	for i := range ids {
		if got[i] != ids[i] {
			t.Fatalf("orden/paginación incorrectos: got %v, want %v", got, ids)
		}
	}
}

func TestListPublicByUser_OrdenEstableConFechasIguales(t *testing.T) {
	svc, _, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	owner := uuid.NewString()
	same := time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC)
	seedPublicListNote(t, noteStore, publicNoteID(1), owner, "Uno", "public", same)
	seedPublicListNote(t, noteStore, publicNoteID(2), owner, "Dos", "public", same)
	seedPublicListNote(t, noteStore, publicNoteID(3), owner, "Tres", "public", same)

	first, next, err := listPublicByAuthor(ctx, svc, owner, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || first[0].ID != publicNoteID(3) || first[1].ID != publicNoteID(2) {
		t.Fatalf("primera página inesperada: %+v", first)
	}
	if next != publicNoteID(2) {
		t.Fatalf("next_cursor = %q, want %q", next, publicNoteID(2))
	}
	second, next2, err := listPublicByAuthor(ctx, svc, owner, next, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || second[0].ID != publicNoteID(1) {
		t.Fatalf("segunda página inesperada: %+v", second)
	}
	if next2 != "" {
		t.Fatalf("next_cursor = %q, want vacío", next2)
	}
}

func TestListPublicByUser_CursoresNoValidosNoSaltanVisibilidad(t *testing.T) {
	svc, _, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	owner := uuid.NewString()
	now := time.Now().UTC()
	pub := seedPublicListNote(t, noteStore, publicNoteID(1), owner, "Público", "public", now)
	priv := seedPublicListNote(t, noteStore, publicNoteID(2), owner, "Privado", "private", now.Add(time.Second))
	other := seedPublicListNote(t, noteStore, publicNoteID(3), uuid.NewString(), "Otro", "public", now.Add(2*time.Second))
	sinArchivo := seedPublicListNoteSinArchivo(t, noteStore, publicNoteID(4), owner, "Público sin archivo", now.Add(3*time.Second))

	cases := []struct {
		name   string
		cursor string
	}{
		{"inexistente", uuid.NewString()},
		{"de otro autor", other.ID},
		{"de apunte privado", priv.ID},
		{"de apunte público sin archivo", sinArchivo.ID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			notes, next, err := listPublicByAuthor(ctx, svc, owner, tc.cursor, 20)
			if err != nil {
				t.Fatalf("cursor %s no debe dar error: %v", tc.name, err)
			}
			if len(notes) != 0 || next != "" {
				t.Fatalf("cursor %s debe producir lista vacía fail-closed, got %+v next=%q", tc.name, notes, next)
			}
			for _, n := range notes {
				if n.ID == priv.ID || n.ID == other.ID || n.ID == pub.ID {
					t.Fatalf("cursor %s filtró una nota: %+v", tc.name, n)
				}
			}
		})
	}
}

func TestListPublicByUser_LimitePorDefectoYMaximo(t *testing.T) {
	svc, _, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	owner := uuid.NewString()
	base := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	for i := 1; i <= 105; i++ {
		seedPublicListNote(t, noteStore, publicNoteID(i), owner, fmt.Sprintf("Nota %d", i), "public", base.Add(time.Duration(i)*time.Second))
	}

	defaultPage, nextDefault, err := listPublicByAuthor(ctx, svc, owner, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(defaultPage) != 20 {
		t.Fatalf("límite por defecto: got %d notas, want 20", len(defaultPage))
	}
	if nextDefault == "" {
		t.Fatal("límite por defecto: se esperaba next_cursor con 105 notas")
	}

	maxPage, nextMax, err := listPublicByAuthor(ctx, svc, owner, "", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(maxPage) != 100 {
		t.Fatalf("límite máximo: got %d notas, want 100", len(maxPage))
	}
	if nextMax == "" {
		t.Fatal("límite máximo: se esperaba next_cursor con 105 notas")
	}
}

func assertNotesServiceCode(t *testing.T, err error, code string) {
	t.Helper()
	var se *ServiceError
	if !errors.As(err, &se) || se.Code != code {
		t.Fatalf("esperado ServiceError %q, got %v", code, err)
	}
}
