package grpc

import (
	"github.com/google/uuid"
	notesv1 "github.com/kurovoxx/tallerintegracion3/back/notes/internal/gen/notesv1/v1"
	"testing"
)

func TestListMyNotesPaginationAndIsolation(t *testing.T) {
	h := newHarness(t)
	owner, stranger := uuid.NewString(), uuid.NewString()
	ctx := h.userCtx(t, owner)
	want := map[string]bool{}
	for i := 0; i < 3; i++ {
		n, err := h.client.CreateNote(ctx, &notesv1.CreateNoteRequest{Title: "Página", Visibility: "private"})
		if err != nil {
			t.Fatal(err)
		}
		want[n.NoteId] = true
	}
	empty, err := h.client.ListMyNotes(h.userCtx(t, stranger), &notesv1.ListMyNotesRequest{})
	if err != nil || len(empty.GetNotes()) != 0 {
		t.Fatalf("aislamiento: %v %v", empty, err)
	}
	seen := map[string]bool{}
	cursor := ""
	for page := 0; page < 4; page++ {
		got, err := h.client.ListMyNotes(ctx, &notesv1.ListMyNotesRequest{Page: &notesv1.PageRequest{PageSize: 2, PageToken: cursor}})
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range got.Notes {
			if !want[n.Id] || seen[n.Id] || n.UserId != owner {
				t.Fatalf("nota duplicada/ajena: %v", n)
			}
			seen[n.Id] = true
		}
		cursor = got.GetPage().GetNextPageToken()
		if cursor == "" {
			break
		}
	}
	if cursor != "" || len(seen) != len(want) {
		t.Fatalf("paginación incompleta: %v cursor=%s", seen, cursor)
	}
}
