package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSocialHTTPAdapterContract(t *testing.T) {
	groupID, memberID, adminID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	var internalKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		internalKey = r.Header.Get("X-Internal-Key")
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/internal/groups/" + groupID + "/members":
			_ = json.NewEncoder(w).Encode([]map[string]string{
				{"user_id": adminID, "role": "admin"},
				{"user_id": memberID, "role": "member"},
			})
		case "/internal/groups/" + groupID + "/member-emails":
			_ = json.NewEncoder(w).Encode(map[string]any{"emails": []string{"a@example.com", "B@example.com"}})
		case "/internal/users/" + memberID + "/followers-count":
			_ = json.NewEncoder(w).Encode(map[string]int{"followers_count": 7})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	adapter := NewSocialHTTPAdapter(srv.URL+"/", time.Second, "internal-secret")
	ctx := context.Background()
	if ok, err := adapter.IsMember(ctx, memberID, groupID); err != nil || !ok {
		t.Fatalf("member must be detected: %v %v", ok, err)
	}
	if ok, err := adapter.IsMember(ctx, uuid.NewString(), groupID); err != nil || ok {
		t.Fatalf("outsider must not be a member: %v %v", ok, err)
	}
	if ok, err := adapter.IsAdmin(ctx, adminID, groupID); err != nil || !ok {
		t.Fatalf("admin must be detected: %v %v", ok, err)
	}
	if ok, err := adapter.IsAdmin(ctx, memberID, groupID); err != nil || ok {
		t.Fatalf("plain member must not be admin: %v %v", ok, err)
	}
	emails, err := adapter.ListMemberEmails(ctx, groupID)
	if err != nil || len(emails) != 2 || emails[0] != "a@example.com" {
		t.Fatalf("member emails: %v %v", emails, err)
	}
	if followers, err := adapter.GetFollowersCount(ctx, memberID); err != nil || followers != 7 {
		t.Fatalf("followers: %d %v", followers, err)
	}
	if internalKey != "internal-secret" {
		t.Fatalf("internal key must be forwarded, got %q", internalKey)
	}
}

func TestSocialHTTPAdapterNotFoundDegradesToEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	adapter := NewSocialHTTPAdapter(srv.URL, time.Second, "")
	ctx := context.Background()
	if ok, err := adapter.IsMember(ctx, uuid.NewString(), uuid.NewString()); err != nil || ok {
		t.Fatalf("404 must mean not a member: %v %v", ok, err)
	}
	if emails, err := adapter.ListMemberEmails(ctx, uuid.NewString()); err != nil || emails != nil {
		t.Fatalf("404 must mean no emails: %v %v", emails, err)
	}
}

func TestSocialHTTPAdapterErrorsAreBounded(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	adapter := NewSocialHTTPAdapter(srv.URL, 50*time.Millisecond, "")
	start := time.Now()
	if _, err := adapter.IsMember(context.Background(), "user", "group"); err == nil {
		t.Fatal("timeout must surface as an error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("call must be bounded by the configured timeout, took %s", elapsed)
	}
}

func TestSocialHTTPAdapterMalformedPayloadFailsClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()
	adapter := NewSocialHTTPAdapter(srv.URL, time.Second, "")
	if _, err := adapter.IsMember(context.Background(), "user", "group"); err == nil {
		t.Fatal("malformed payload must fail closed")
	}
}
