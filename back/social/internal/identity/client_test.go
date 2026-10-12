package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Identity falso: verify acepta solo "bueno", users conoce solo "u1".
func fakeIdentity(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/auth/verify", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Key") != "k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Token == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if body.Token != "bueno" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"valid":true,"user_id":"u1"}`))
	})
	mux.HandleFunc("/internal/users/u1", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Key") != "k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user_id":"u1","email":"u1@example.com"}`))
	})
	mux.HandleFunc("/internal/users/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestClient_VerifyToken(t *testing.T) {
	srv := fakeIdentity(t)
	c := NewClient(srv.URL, "k")
	ctx := context.Background()

	uid, err := c.VerifyToken(ctx, "bueno")
	if err != nil || uid != "u1" {
		t.Fatalf("token bueno: uid=%q err=%v", uid, err)
	}
	if _, err := c.VerifyToken(ctx, "malo"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("token malo debe dar ErrInvalidToken, got %v", err)
	}
	if _, err := c.VerifyToken(ctx, ""); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("token vacío debe dar ErrInvalidToken, got %v", err)
	}
	// Key incorrecta → Identity 401 → ErrInvalidToken (fail closed, no éxito).
	bad := NewClient(srv.URL, "otra")
	if _, err := bad.VerifyToken(ctx, "bueno"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("key mala debe dar ErrInvalidToken, got %v", err)
	}
}

func TestClient_GetUser(t *testing.T) {
	srv := fakeIdentity(t)
	c := NewClient(srv.URL, "k")
	ctx := context.Background()

	u, err := c.GetUser(ctx, "u1")
	if err != nil || u.Email != "u1@example.com" {
		t.Fatalf("u1: %+v err=%v", u, err)
	}
	if _, err := c.GetUser(ctx, "nope"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("inexistente debe dar ErrUserNotFound, got %v", err)
	}
	// Sin baseURL → error, nunca éxito silencioso.
	empty := NewClient("", "k")
	if _, err := empty.GetUser(ctx, "u1"); err == nil {
		t.Fatal("sin baseURL debe fallar")
	}
	if _, err := empty.VerifyToken(ctx, "bueno"); err == nil {
		t.Fatal("sin baseURL debe fallar")
	}
}
