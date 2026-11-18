package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type fakeAuthorPublicIDs struct {
	codes map[string]string
	err   error
	calls int
}

func (f *fakeAuthorPublicIDs) LookupPublicIDs(_ context.Context, ids []string) (map[string]string, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	out := map[string]string{}
	for _, id := range ids {
		if c, ok := f.codes[id]; ok {
			out[id] = c
		}
	}
	return out, nil
}

// Guardados y públicos muestran el código público del autor (una llamada a
// Identity por página) y nunca el UUID como sustituto.
func TestAuthorPublicID_GuardadosYPublicos(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	me, ana, beto := uuid.NewString(), uuid.NewString(), uuid.NewString()
	fake := &fakeAuthorPublicIDs{codes: map[string]string{ana: "A8KP92MQWX7R"}}
	svc.SetAuthorPublicIDResolver(fake)

	n1, err := svc.Create(ctx, ana, "De Ana", nil, "public", stringPtr("x"), "")
	if err != nil {
		t.Fatal(err)
	}
	n2, err := svc.Create(ctx, beto, "De Beto", nil, "public", stringPtr("x"), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{n1.ID, n2.ID} {
		if _, err := svc.Save(ctx, me, id); err != nil {
			t.Fatal(err)
		}
	}

	fake.calls = 0
	saved, _, err := svc.ListSaved(ctx, me, SavedFilter{}, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if fake.calls != 1 {
		t.Fatalf("debe resolver los autores en una sola llamada, hubo %d", fake.calls)
	}
	byAuthor := map[string]string{}
	for _, v := range saved {
		byAuthor[v.AuthorID] = v.AuthorPublicID
	}
	if byAuthor[ana] != "a8kp92mqwx7r" {
		t.Fatalf("código público de Ana (normalizado): %q", byAuthor[ana])
	}
	if byAuthor[beto] != "" {
		t.Fatalf("sin código debe quedar vacío, nunca el UUID: %q", byAuthor[beto])
	}

	fake.calls = 0
	pub, _, err := svc.ListPublic(ctx, me, "", "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if fake.calls != 1 {
		t.Fatalf("públicos: una sola llamada por página, hubo %d", fake.calls)
	}
	for _, v := range pub {
		if v.UserID == ana && v.AuthorPublicID != "a8kp92mqwx7r" {
			t.Fatalf("público de Ana sin código: %+v", v)
		}
		if strings.Contains(v.AuthorPublicID, "-") {
			t.Fatalf("author_public_id con forma de UUID: %q", v.AuthorPublicID)
		}
	}
}

// Identity caído: los listados siguen funcionando sin author_public_id.
func TestAuthorPublicID_IdentityCaido_Degrada(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	me, ana := uuid.NewString(), uuid.NewString()
	svc.SetAuthorPublicIDResolver(&fakeAuthorPublicIDs{err: errors.New("caído")})
	n, err := svc.Create(ctx, ana, "De Ana", nil, "public", stringPtr("x"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Save(ctx, me, n.ID); err != nil {
		t.Fatal(err)
	}
	saved, _, err := svc.ListSaved(ctx, me, SavedFilter{}, "", 0)
	if err != nil || len(saved) != 1 || saved[0].AuthorPublicID != "" {
		t.Fatalf("debe degradar sin código: %+v, %v", saved, err)
	}
	pub, _, err := svc.ListPublic(ctx, me, "", "", "", 0)
	if err != nil || len(pub) != 1 || pub[0].AuthorPublicID != "" {
		t.Fatalf("debe degradar sin código: %+v, %v", pub, err)
	}
}
