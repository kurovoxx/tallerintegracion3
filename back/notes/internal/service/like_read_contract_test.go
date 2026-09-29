package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/utils"
)

func TestLikeReadAccess(t *testing.T) {
	for _, tc := range []struct {
		name       string
		visibility string
		share      string
		owner      bool
		member     bool
		allowed    bool
	}{
		{name: "private outsider", visibility: "private"},
		{name: "private owner", visibility: "private", owner: true, allowed: true},
		{name: "public outsider", visibility: "public", allowed: true},
		{name: "link outsider", visibility: "private", share: "link", allowed: true},
		{name: "restricted outsider", visibility: "private", share: "restricted"},
		{name: "restricted member", visibility: "private", share: "restricted", member: true, allowed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, notes, _, _, likes, shares, social := newTestService()
			ctx := context.Background()
			owner, user, group := uuid.NewString(), uuid.NewString(), uuid.NewString()
			if tc.owner {
				user = owner
			}
			note, err := notes.Create(ctx, "", owner, nil, "Access", nil, tc.visibility, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			if tc.share != "" {
				if _, err := shares.Create(ctx, note.ID, group, true, tc.share, 0); err != nil {
					t.Fatal(err)
				}
			}
			if tc.member {
				social.AddMember(user, group)
			}
			err = svc.Like(ctx, user, note.ID)
			wantCount := 0
			if tc.allowed {
				if err != nil {
					t.Fatal(err)
				}
				wantCount = 1
			} else {
				missingErr := svc.Like(ctx, user, uuid.NewString())
				var se *ServiceError
				if !errors.As(err, &se) || se.Code != utils.ErrNotFound || utils.StatusForCode(se.Code) != http.StatusNotFound {
					t.Fatalf("expected zero-knowledge 404, got %v", err)
				}
				if missingErr == nil || err.Error() != missingErr.Error() {
					t.Fatalf("denied and missing errors differ: %v / %v", err, missingErr)
				}
			}
			exists, err := likes.Exists(ctx, note.ID, user)
			if err != nil || exists != tc.allowed {
				t.Fatalf("like persisted=%v, error=%v", exists, err)
			}
			stored, err := notes.GetByID(ctx, note.ID)
			if err != nil || stored.LikesCount != wantCount {
				t.Fatalf("expected likes_count=%d, note=%+v, error=%v", wantCount, stored, err)
			}
		})
	}
}

func TestLikeRevokedAccessHidesExistingLike(t *testing.T) {
	svc, _, notes, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	owner, user := uuid.NewString(), uuid.NewString()
	note, err := notes.Create(ctx, "", owner, nil, "Revoked", nil, "public", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Like(ctx, user, note.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := notes.Update(ctx, note.ID, nil, stringPtr("private"), note.Version); err != nil {
		t.Fatal(err)
	}
	var se *ServiceError
	if err := svc.Like(ctx, user, note.ID); !errors.As(err, &se) || se.Code != utils.ErrNotFound {
		t.Fatalf("expected not_found instead of already_liked, got %v", err)
	}
}

func TestLikeUnlikeOnlyDecrementsDeletedLike(t *testing.T) {
	svc, _, notes, _, _, likes, _, _ := newTestService()
	ctx := context.Background()
	owner, user, other := uuid.NewString(), uuid.NewString(), uuid.NewString()
	note, err := notes.Create(ctx, "", owner, nil, "Counts", nil, "public", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Like(ctx, user, note.ID); err != nil {
		t.Fatal(err)
	}
	// Removing an absent like must preserve another user's positive count.
	for i := 0; i < 2; i++ {
		if err := svc.Unlike(ctx, other, note.ID); err != nil {
			t.Fatal(err)
		}
	}
	stored, err := notes.GetByID(ctx, note.ID)
	if err != nil || stored.LikesCount != 1 {
		t.Fatalf("absent like decremented count: %+v, %v", stored, err)
	}
	if err := svc.Unlike(ctx, user, note.ID); err != nil {
		t.Fatal(err)
	}
	// A legacy inconsistent row with count zero must not become negative.
	if _, err := likes.Create(ctx, note.ID, user); err != nil {
		t.Fatal(err)
	}
	if err := svc.Unlike(ctx, user, note.ID); err != nil {
		t.Fatal(err)
	}
	stored, err = notes.GetByID(ctx, note.ID)
	if err != nil || stored.LikesCount != 0 {
		t.Fatalf("expected zero count, got %+v, %v", stored, err)
	}
	exists, err := likes.Exists(ctx, note.ID, user)
	if err != nil || exists {
		t.Fatalf("like was not removed: exists=%v, error=%v", exists, err)
	}
}

func TestGetMissingRemoteFile(t *testing.T) {
	for _, file := range []struct {
		name string
		id   *string
	}{
		{name: "nil"},
		{name: "empty", id: stringPtr("")},
		{name: "whitespace", id: stringPtr(" \t\r\n")},
	} {
		for _, access := range []string{"owner", "public", "denied"} {
			t.Run(file.name+"/"+access, func(t *testing.T) {
				svc, _, notes, _, _, _, _, _ := newTestService()
				ctx := context.Background()
				owner, requester := uuid.NewString(), uuid.NewString()
				visibility := "private"
				if access == "owner" {
					requester = owner
				} else if access == "public" {
					visibility = "public"
				}
				note, err := notes.Create(ctx, "", owner, nil, "Unavailable", file.id, visibility, nil, "")
				if err != nil {
					t.Fatal(err)
				}
				got, err := svc.Get(ctx, requester, note.ID)
				var se *ServiceError
				if got != nil || !errors.As(err, &se) {
					t.Fatalf("expected no note and service error, got %+v, %v", got, err)
				}
				wantCode := utils.ErrNoteUnavailable
				if access == "denied" {
					wantCode = utils.ErrNotFound
				}
				if se.Code != wantCode || utils.StatusForCode(se.Code) != http.StatusNotFound {
					t.Fatalf("expected 404 %s, got %v", wantCode, err)
				}
				if access != "denied" && se.Message != "Nota no disponible en almacenamiento remoto" {
					t.Fatalf("unexpected unavailable message: %q", se.Message)
				}
			})
		}
	}
}
