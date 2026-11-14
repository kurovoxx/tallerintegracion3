package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/identity"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/utils"
)

// fakePublicProfileResolver traduce public_id → UUID.
type fakePublicProfileResolver struct {
	ids          map[string]string
	err          error
	calls        int
	lastPublicID string
}

func (f *fakePublicProfileResolver) ResolvePublicProfile(_ context.Context, publicID string) (string, error) {
	f.calls++
	f.lastPublicID = publicID
	if f.err != nil {
		return "", f.err
	}
	if id, ok := f.ids[publicID]; ok {
		return id, nil
	}
	return "", identity.ErrNotFound
}

func TestListPublicByPublicID_ResuelveYNormaliza(t *testing.T) {
	svc, _, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	owner := uuid.NewString()
	requester := uuid.NewString()
	pub := seedPublicListNote(t, noteStore, publicNoteID(1), owner, "Público", "public", time.Now().UTC())
	seedPublicListNote(t, noteStore, publicNoteID(2), owner, "Privado", "private", time.Now().UTC())
	resolver := &fakePublicProfileResolver{ids: map[string]string{"a8kp92mqwx7r": owner}}
	svc.SetPublicProfileResolver(resolver)

	notes, next, err := svc.ListPublicByPublicID(ctx, requester, "  A8KP92MQWX7R  ", "", 20)
	if err != nil {
		t.Fatalf("esperado sin error, got %v", err)
	}
	if resolver.lastPublicID != "a8kp92mqwx7r" {
		t.Fatalf("public_id enviado a Identity = %q, want minúsculas", resolver.lastPublicID)
	}
	if next != "" {
		t.Fatalf("next_cursor = %q, want vacío", next)
	}
	if len(notes) != 1 || notes[0].ID != pub.ID || notes[0].Visibility != "public" {
		t.Fatalf("solo se esperaba el apunte público, got %+v", notes)
	}
	if notes[0].LikesCount != 7 || notes[0].SavesCount != 0 || notes[0].IsLiked || notes[0].IsSaved {
		t.Fatalf("interacciones iniciales inesperadas: %+v", notes[0])
	}

	if _, err := svc.Save(ctx, requester, pub.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Like(ctx, requester, pub.ID); err != nil {
		t.Fatal(err)
	}
	notes, _, err = svc.ListPublicByPublicID(ctx, requester, "a8kp92mqwx7r", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 || notes[0].LikesCount != 8 || notes[0].SavesCount != 1 || !notes[0].IsLiked || !notes[0].IsSaved {
		t.Fatalf("interacciones tras like/save: %+v", notes)
	}
}

func TestListPublicByPublicID_FormatosInvalidos_404SinLlamarIdentity(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	resolver := &fakePublicProfileResolver{ids: map[string]string{"a8kp92mqwx7r": uuid.NewString()}}
	svc.SetPublicProfileResolver(resolver)
	cases := []string{
		"",
		"a8kp92mqwx7",
		"a8kp92mqwx7rr",
		"a8kp92mqwx7l",
		"a8kp92mqwx7o",
		"550e8400-e29b-41d4-a716-446655440001",
		"Agustín Vega",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			_, _, err := svc.ListPublicByPublicID(context.Background(), uuid.NewString(), in, "", 20)
			assertNotesServiceCode(t, err, utils.ErrNotFound)
		})
	}
	if resolver.calls != 0 {
		t.Fatalf("un formato inválido no debe llegar a Identity, calls=%d", resolver.calls)
	}
}

func TestListPublicByPublicID_IdentityNoEncuentra_404(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	svc.SetPublicProfileResolver(&fakePublicProfileResolver{ids: map[string]string{}})
	_, _, err := svc.ListPublicByPublicID(context.Background(), uuid.NewString(), "a8kp92mqwx7r", "", 20)
	assertNotesServiceCode(t, err, utils.ErrNotFound)

	svc.SetPublicProfileResolver(&fakePublicProfileResolver{err: identity.ErrInvalid})
	_, _, err = svc.ListPublicByPublicID(context.Background(), uuid.NewString(), "a8kp92mqwx7r", "", 20)
	assertNotesServiceCode(t, err, utils.ErrNotFound)
}

func TestListPublicByPublicID_IdentityCaido_503(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	_, _, err := svc.ListPublicByPublicID(context.Background(), uuid.NewString(), "a8kp92mqwx7r", "", 20)
	assertNotesServiceCode(t, err, utils.ErrServiceUnavailable)

	svc.SetPublicProfileResolver(&fakePublicProfileResolver{err: identity.ErrUnavailable})
	_, _, err = svc.ListPublicByPublicID(context.Background(), uuid.NewString(), "a8kp92mqwx7r", "", 20)
	assertNotesServiceCode(t, err, utils.ErrServiceUnavailable)

	svc.SetPublicProfileResolver(&fakePublicProfileResolver{err: errors.New("boom")})
	_, _, err = svc.ListPublicByPublicID(context.Background(), uuid.NewString(), "a8kp92mqwx7r", "", 20)
	assertNotesServiceCode(t, err, utils.ErrServiceUnavailable)
}

func TestListPublicByPublicID_CursorInvalido_400(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	svc.SetPublicProfileResolver(&fakePublicProfileResolver{ids: map[string]string{"a8kp92mqwx7r": uuid.NewString()}})
	_, _, err := svc.ListPublicByPublicID(context.Background(), uuid.NewString(), "a8kp92mqwx7r", "no-es-uuid", 20)
	assertNotesServiceCode(t, err, utils.ErrBadRequest)
}

// countingSavedStore y countingLikeStore verifican que el listado nuevo usa
// las consultas batch (una por página) y no cae al camino N+1.
type countingSavedStore struct {
	*MemorySavedStore
	countByNoteCalls int
	existsCalls      int
	batchCountCalls  int
	batchExistsCalls int
}

func (c *countingSavedStore) CountByNote(ctx context.Context, noteID string) (int, error) {
	c.countByNoteCalls++
	return c.MemorySavedStore.CountByNote(ctx, noteID)
}
func (c *countingSavedStore) Exists(ctx context.Context, userID, noteID string) (bool, error) {
	c.existsCalls++
	return c.MemorySavedStore.Exists(ctx, userID, noteID)
}
func (c *countingSavedStore) CountByNotes(ctx context.Context, noteIDs []string) (map[string]int, error) {
	c.batchCountCalls++
	return c.MemorySavedStore.CountByNotes(ctx, noteIDs)
}
func (c *countingSavedStore) ExistsByNotes(ctx context.Context, userID string, noteIDs []string) (map[string]bool, error) {
	c.batchExistsCalls++
	return c.MemorySavedStore.ExistsByNotes(ctx, userID, noteIDs)
}

type countingLikeStore struct {
	*MemoryLikeStore
	existsCalls      int
	batchExistsCalls int
}

func (c *countingLikeStore) Exists(ctx context.Context, noteID, userID string) (bool, error) {
	c.existsCalls++
	return c.MemoryLikeStore.Exists(ctx, noteID, userID)
}
func (c *countingLikeStore) ExistsByNotes(ctx context.Context, userID string, noteIDs []string) (map[string]bool, error) {
	c.batchExistsCalls++
	return c.MemoryLikeStore.ExistsByNotes(ctx, userID, noteIDs)
}

func TestListPublicByPublicID_InteraccionesEnLoteSinN1(t *testing.T) {
	noteStore := NewMemoryNoteStore()
	attStore := NewMemoryAttachmentStore()
	saved := &countingSavedStore{MemorySavedStore: NewMemorySavedStore()}
	likes := &countingLikeStore{MemoryLikeStore: NewMemoryLikeStore()}
	likes.SetNoteStore(noteStore)
	shared := NewMemorySharedStore()
	svc := NewNoteService(noteStore, attStore, saved, likes, shared, drive.NewMockClient(), NewMemorySocialResolver())

	owner := uuid.NewString()
	for i := 1; i <= 5; i++ {
		seedPublicListNote(t, noteStore, publicNoteID(i), owner, "N", "public", time.Now().UTC().Add(time.Duration(i)*time.Second))
	}
	svc.SetPublicProfileResolver(&fakePublicProfileResolver{ids: map[string]string{"a8kp92mqwx7r": owner}})

	if _, _, err := svc.ListPublicByPublicID(context.Background(), uuid.NewString(), "a8kp92mqwx7r", "", 20); err != nil {
		t.Fatal(err)
	}
	if saved.countByNoteCalls != 0 || saved.existsCalls != 0 || likes.existsCalls != 0 {
		t.Fatalf("se usó el camino N+1: saved=%d/%d like=%d", saved.countByNoteCalls, saved.existsCalls, likes.existsCalls)
	}
	if saved.batchCountCalls != 1 || saved.batchExistsCalls != 1 || likes.batchExistsCalls != 1 {
		t.Fatalf("batch inesperado: saved count=%d exists=%d like=%d", saved.batchCountCalls, saved.batchExistsCalls, likes.batchExistsCalls)
	}
}

func TestListPublic_InteraccionesEnLoteSinN1(t *testing.T) {
	noteStore := NewMemoryNoteStore()
	saved := &countingSavedStore{MemorySavedStore: NewMemorySavedStore()}
	likes := &countingLikeStore{MemoryLikeStore: NewMemoryLikeStore()}
	likes.SetNoteStore(noteStore)
	svc := NewNoteService(noteStore, NewMemoryAttachmentStore(), saved, likes, NewMemorySharedStore(), drive.NewMockClient(), NewMemorySocialResolver())

	owner := uuid.NewString()
	for i := 1; i <= 5; i++ {
		seedPublicListNote(t, noteStore, publicNoteID(i), owner, "N", "public", time.Now().UTC().Add(time.Duration(i)*time.Second))
	}
	if _, _, err := svc.ListPublic(context.Background(), uuid.NewString(), "", "", "", 20); err != nil {
		t.Fatal(err)
	}
	if saved.countByNoteCalls != 0 || saved.existsCalls != 0 || likes.existsCalls != 0 {
		t.Fatalf("ListPublic usó N+1: saved=%d/%d like=%d", saved.countByNoteCalls, saved.existsCalls, likes.existsCalls)
	}
	if saved.batchCountCalls != 1 || saved.batchExistsCalls != 1 || likes.batchExistsCalls != 1 {
		t.Fatalf("batch inesperado: saved count=%d exists=%d like=%d", saved.batchCountCalls, saved.batchExistsCalls, likes.batchExistsCalls)
	}
}

// Guardas de compilación: los stores PG/memoria implementan los contratos
// opcionales que habilitan el camino batch.
var (
	_ SavedBatchStore = (*MemorySavedStore)(nil)
	_ SavedBatchStore = (*PGSavedStore)(nil)
	_ LikeBatchStore  = (*MemoryLikeStore)(nil)
	_ LikeBatchStore  = (*PGLikeStore)(nil)
)
