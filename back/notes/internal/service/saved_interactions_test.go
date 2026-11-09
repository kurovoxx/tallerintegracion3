package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/utils"
)

func serviceCode(t *testing.T, err error) string {
	t.Helper()
	var se *ServiceError
	if !errors.As(err, &se) {
		t.Fatalf("se esperaba ServiceError, llegó %v", err)
	}
	return se.Code
}

func TestInteractionsCountersAreExactAndIdempotent(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author, alice, bob := uuid.NewString(), uuid.NewString(), uuid.NewString()
	note, err := svc.Create(ctx, author, "Público", nil, "public", stringPtr("# hola"), "")
	if err != nil {
		t.Fatal(err)
	}

	for _, u := range []string{alice, bob} {
		if err := svc.Like(ctx, u, note.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Save(ctx, u, note.ID); err != nil {
			t.Fatal(err)
		}
	}
	// Repetir like/save no duplica: 409 controlado y contadores intactos.
	if code := serviceCode(t, svc.Like(ctx, alice, note.ID)); code != utils.ErrAlreadyLiked {
		t.Fatalf("like duplicado: %s", code)
	}
	if _, err := svc.Save(ctx, alice, note.ID); serviceCode(t, err) != utils.ErrAlreadySaved {
		t.Fatalf("save duplicado: %v", err)
	}
	stats, err := svc.Interactions(ctx, alice, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stats.LikesCount != 2 || stats.SavesCount != 2 || !stats.IsLiked || !stats.IsSaved {
		t.Fatalf("stats inesperados: %+v", stats)
	}

	// Quitar dos veces es idempotente.
	for i := 0; i < 2; i++ {
		if err := svc.Unlike(ctx, alice, note.ID); err != nil {
			t.Fatal(err)
		}
		if err := svc.Unsave(ctx, alice, note.ID); err != nil {
			t.Fatal(err)
		}
	}
	stats, _ = svc.Interactions(ctx, alice, note.ID)
	if stats.LikesCount != 1 || stats.SavesCount != 1 || stats.IsLiked || stats.IsSaved {
		t.Fatalf("stats tras quitar: %+v", stats)
	}
}

func TestSavedNoteSurvivesOwnerDeletionAsUnavailable(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author, reader := uuid.NewString(), uuid.NewString()
	note, err := svc.Create(ctx, author, "Cálculo II", nil, "public", stringPtr("$x^2$"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Save(ctx, reader, note.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, author, note.ID); err != nil {
		t.Fatal(err)
	}

	items, _, err := svc.ListSaved(ctx, reader, SavedFilter{}, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("el guardado debía sobrevivir, items=%d", len(items))
	}
	got := items[0]
	if got.IsAvailable || got.UnavailableReason != model.UnavailableDeleted || got.Title != "Cálculo II" || got.DeletedAt == nil {
		t.Fatalf("guardado no marcado como no disponible: %+v", got)
	}

	// Abrir / copiar / guardar de nuevo => 410 note_deleted controlado.
	if _, err := svc.Get(ctx, reader, note.ID); serviceCode(t, err) != utils.ErrNoteDeleted {
		t.Fatalf("get: %v", err)
	}
	if _, err := svc.Copy(ctx, reader, note.ID, ""); serviceCode(t, err) != utils.ErrNoteDeleted {
		t.Fatalf("copy: %v", err)
	}
	// El propio autor que la borró sigue viendo 404.
	if _, err := svc.Get(ctx, author, note.ID); serviceCode(t, err) != utils.ErrNotFound {
		t.Fatalf("autor: %v", err)
	}
	// Quitar de guardados funciona aunque el original no exista.
	if err := svc.Unsave(ctx, reader, note.ID); err != nil {
		t.Fatal(err)
	}
	items, _, _ = svc.ListSaved(ctx, reader, SavedFilter{}, "", 0)
	if len(items) != 0 {
		t.Fatalf("unsave no limpió el guardado: %+v", items)
	}
}

func TestDeletedPrivateNoteStaysZeroKnowledgeForStrangers(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author, stranger := uuid.NewString(), uuid.NewString()
	note, err := svc.Create(ctx, author, "Privada", nil, "private", stringPtr("x"), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, author, note.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, stranger, note.ID); serviceCode(t, err) != utils.ErrNotFound {
		t.Fatalf("una privada ajena borrada debe seguir siendo 404: %v", err)
	}
}

func TestSavedNoteBecomesUnavailableWhenMadePrivate(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author, reader := uuid.NewString(), uuid.NewString()
	note, err := svc.Create(ctx, author, "Secreto futuro", nil, "public", stringPtr("x"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Save(ctx, reader, note.ID); err != nil {
		t.Fatal(err)
	}
	private := "private"
	if _, err := svc.Update(ctx, author, note.ID, nil, &private, nil, ""); err != nil {
		t.Fatal(err)
	}
	items, _, err := svc.ListSaved(ctx, reader, SavedFilter{}, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].IsAvailable || items[0].UnavailableReason != model.UnavailableNoAccess || items[0].Title == "Secreto futuro" {
		t.Fatalf("debía quedar no disponible sin filtrar el título: %+v", items)
	}
}

func TestShareSameGroupTwiceIsConflict(t *testing.T) {
	svc, _, _, _, _, _, shares, social := newTestService()
	ctx := context.Background()
	owner, group, other := uuid.NewString(), uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, group)
	social.AddAdmin(owner, other)
	note, err := svc.Create(ctx, owner, "Para el grupo", nil, "private", stringPtr("x"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Share(ctx, owner, note.ID, group, "link"); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"link", "restricted"} {
		_, err := svc.Share(ctx, owner, note.ID, group, mode)
		if code := serviceCode(t, err); code != utils.ErrAlreadyShared || utils.StatusForCode(code) != 409 {
			t.Fatalf("share duplicado (%s): %v", mode, err)
		}
	}
	// Otro grupo sí se permite.
	if _, err := svc.Share(ctx, owner, note.ID, other, "link"); err != nil {
		t.Fatal(err)
	}
	list, _ := shares.ListByNote(ctx, note.ID)
	if len(list) != 2 {
		t.Fatalf("se esperaban 2 vínculos, hay %d", len(list))
	}
}

func TestCopySavedNoteCreatesOwnPrivateCloneInCopierDrive(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author, reader := uuid.NewString(), uuid.NewString()
	orig, err := svc.Create(ctx, author, "Álgebra lineal", nil, "public", stringPtr("# Matrices\n$A^{-1}$"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Save(ctx, reader, orig.ID); err != nil {
		t.Fatal(err)
	}
	clone, err := svc.Copy(ctx, reader, orig.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if clone.ID == orig.ID || clone.UserID != reader || clone.Visibility != "private" {
		t.Fatalf("clon inválido: %+v", clone)
	}
	if clone.ForkedFromNoteID == nil || *clone.ForkedFromNoteID != orig.ID {
		t.Fatalf("forked_from_note_id incorrecto: %+v", clone.ForkedFromNoteID)
	}
	if clone.ExternalFileID == nil || *clone.ExternalFileID == *orig.ExternalFileID {
		t.Fatalf("el clon debe tener su propio archivo en Drive: %+v", clone.ExternalFileID)
	}
	// El contenido viaja con el archivo copiado en el Drive del lector.
	got, err := svc.Get(ctx, reader, clone.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Content == nil || *got.Content != "# Matrices\n$A^{-1}$" {
		t.Fatalf("contenido del clon: %v", got.Content)
	}
}

func TestListSavedFilters(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	me, ana, beto := uuid.NewString(), uuid.NewString(), uuid.NewString()
	math := uuid.NewString()
	mk := func(owner, title string, subject *string) string {
		t.Helper()
		n, err := svc.Create(ctx, owner, title, subject, "public", stringPtr("x"), "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Save(ctx, me, n.ID); err != nil {
			t.Fatal(err)
		}
		return n.ID
	}
	mk(ana, "Integrales dobles", &math)
	mk(beto, "Redes OSI", nil)
	mk(me, "Mi resumen de integrales", nil)
	gone := mk(beto, "Borrador", nil)
	if err := svc.Delete(ctx, beto, gone); err != nil {
		t.Fatal(err)
	}

	count := func(f SavedFilter) int {
		t.Helper()
		items, _, err := svc.ListSaved(ctx, me, f, "", 0)
		if err != nil {
			t.Fatal(err)
		}
		return len(items)
	}
	cases := map[string]struct {
		f    SavedFilter
		want int
	}{
		"todos":           {SavedFilter{}, 4},
		"de otros":        {SavedFilter{Scope: "others"}, 3},
		"propios":         {SavedFilter{Scope: "own"}, 1},
		"texto":           {SavedFilter{Query: "INTEGRALES"}, 2},
		"texto de otros":  {SavedFilter{Query: "integrales", Scope: "others"}, 1},
		"materia":         {SavedFilter{SubjectID: math}, 1},
		"autor":           {SavedFilter{AuthorID: beto}, 2},
		"disponibles":     {SavedFilter{Availability: "available"}, 3},
		"no disponibles":  {SavedFilter{Availability: "unavailable"}, 1},
		"autor + activos": {SavedFilter{AuthorID: beto, Availability: "available"}, 1},
	}
	for name, tc := range cases {
		if got := count(tc.f); got != tc.want {
			t.Errorf("%s: got %d, want %d", name, got, tc.want)
		}
	}
	if _, _, err := svc.ListSaved(ctx, me, SavedFilter{Scope: "nope"}, "", 0); serviceCode(t, err) != utils.ErrBadRequest {
		t.Fatalf("scope inválido: %v", err)
	}
}

func TestListSavedPagination(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	me, author := uuid.NewString(), uuid.NewString()
	for i := 0; i < 5; i++ {
		n, err := svc.Create(ctx, author, "n", nil, "public", stringPtr("x"), "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Save(ctx, me, n.ID); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	cursor := ""
	for page := 0; page < 5; page++ {
		items, next, err := svc.ListSaved(ctx, me, SavedFilter{}, cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range items {
			if seen[it.SavedID] {
				t.Fatalf("elemento repetido entre páginas: %s", it.SavedID)
			}
			seen[it.SavedID] = true
		}
		if next == "" {
			break
		}
		cursor = next
	}
	if len(seen) != 5 {
		t.Fatalf("se esperaban 5 guardados en total, hubo %d", len(seen))
	}
}

func TestListPublicExcludesOwnAndPrivateAndCanBeSaved(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	me, ana := uuid.NewString(), uuid.NewString()
	pub, err := svc.Create(ctx, ana, "Álgebra pública", nil, "public", stringPtr("x"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, ana, "Álgebra privada", nil, "private", stringPtr("x"), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, me, "Mi álgebra", nil, "public", stringPtr("x"), ""); err != nil {
		t.Fatal(err)
	}
	items, _, err := svc.ListPublic(ctx, me, "álgebra", "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != pub.ID || items[0].IsSaved || items[0].IsLiked {
		t.Fatalf("listado público inesperado: %+v", items)
	}
	// Cualquier público se puede guardar y likear desde la tarjeta.
	if _, err := svc.Save(ctx, me, pub.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Like(ctx, me, pub.ID); err != nil {
		t.Fatal(err)
	}
	items, _, _ = svc.ListPublic(ctx, me, "", "", "", 0)
	if len(items) != 1 || !items[0].IsSaved || !items[0].IsLiked || items[0].LikesCount != 1 || items[0].SavesCount != 1 {
		t.Fatalf("estado tras guardar/likear: %+v", items)
	}
}
