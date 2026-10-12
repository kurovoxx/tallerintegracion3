package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// Identity falso para el lookup batch de 3_5_6/3_5_4.
func fakeLookupServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Key") != "k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLookupEmailsStrict_Normaliza(t *testing.T) {
	srv := fakeLookupServer(t, http.StatusOK, `[
		{"user_id":"u1","email":"A@X.com"},
		{"user_id":"u2","email":"a@x.com"},
		{"user_id":"u3","email":"  "},
		{"user_id":"u4","email":"b@x.com"}]`)
	d := NewHTTPUserDirectory(srv.URL, "k")

	got, err := d.LookupEmailsStrict(context.Background(), []string{"u1", "u2", "u3", "u4", "u9"})
	if err != nil {
		t.Fatalf("no debe fallar: %v", err)
	}
	// minúsculas, dedupe, en orden de miembros, sin vacíos ni ausentes.
	want := []string{"a@x.com", "b@x.com"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestLookupEmailsStrict_FailClosed(t *testing.T) {
	srv500 := fakeLookupServer(t, http.StatusInternalServerError, `[]`)
	d := NewHTTPUserDirectory(srv500.URL, "k")
	if _, err := d.LookupEmailsStrict(context.Background(), []string{"u1"}); err == nil {
		t.Fatal("status 500 debe ser error")
	}

	srv := fakeLookupServer(t, http.StatusOK, `[]`)
	url := srv.URL
	srv.Close() // caído
	dDown := NewHTTPUserDirectory(url, "k")
	if _, err := dDown.LookupEmailsStrict(context.Background(), []string{"u1"}); err == nil {
		t.Fatal("red caída debe ser error")
	}
}

func TestListMemberEmailsInternal(t *testing.T) {
	svc, store := newTestSvc()
	g := newGroup(t, svc, uAdmin)
	joinAs(t, svc, store, g.ID, uMember)
	svc.SetUserDirectory(&fakeUserDirectory{data: map[string]UserPublic{
		uAdmin:  {UserID: uAdmin, Email: "admin@example.com"},
		uMember: {UserID: uMember, Email: "member@example.com"},
	}})

	got, err := svc.ListMemberEmailsInternal(ctx, g.ID)
	if err != nil {
		t.Fatalf("no debe fallar: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("se esperaban 2 emails, got %v", got)
	}

	if _, err := svc.ListMemberEmailsInternal(ctx, uuid.NewString()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("grupo inexistente debe dar ErrNotFound, got %v", err)
	}

	svcSinDir, _ := newTestSvc()
	g2 := newGroup(t, svcSinDir, uAdmin)
	if _, err := svcSinDir.ListMemberEmailsInternal(ctx, g2.ID); err == nil {
		t.Fatal("sin directorio debe fallar (fail closed)")
	}
}

func TestListMembersInternal(t *testing.T) {
	svc, store := newTestSvc()
	g := newGroup(t, svc, uAdmin)
	joinAs(t, svc, store, g.ID, uMember)

	ms, err := svc.ListMembersInternal(ctx, g.ID)
	if err != nil || len(ms) != 2 {
		t.Fatalf("se esperaban 2 miembros: %v %v", len(ms), err)
	}
	if _, err := svc.ListMembersInternal(ctx, uuid.NewString()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("grupo inexistente debe dar ErrNotFound, got %v", err)
	}
	if _, err := svc.ListMembersInternal(ctx, "no-uuid"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("id malo debe dar ErrNotFound, got %v", err)
	}
}
