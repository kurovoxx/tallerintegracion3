package drive

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// --- refreshLocks: un mutex por usuario ---

func TestLockForSameUserSameMutex(t *testing.T) {
	s := &PGOAuthTokenStore{}
	m1 := s.lockFor("user-1")
	m2 := s.lockFor("user-1")
	if m1 != m2 {
		t.Fatal("mismo userID debe devolver el mismo *Mutex")
	}
}

func TestLockForDifferentUsersDifferentMutex(t *testing.T) {
	s := &PGOAuthTokenStore{}
	m1 := s.lockFor("user-a")
	m2 := s.lockFor("user-b")
	if m1 == m2 {
		t.Fatal("distintos userID deben tener mutex distintos")
	}
}

func TestRefreshLocksConcurrentSerialization(t *testing.T) {
	s := &PGOAuthTokenStore{}
	const workers = 50
	counter := 0
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			mu := s.lockFor("user-concurrent")
			mu.Lock()
			// Sección crítica: leer-modificar-escribir sin race.
			v := counter
			v++
			counter = v
			mu.Unlock()
		}()
	}
	wg.Wait()
	if counter != workers {
		t.Fatalf("serialización por refreshLocks falló: esperado %d, got %d", workers, counter)
	}
}

func TestRefreshLocksIndependentPerUser(t *testing.T) {
	s := &PGOAuthTokenStore{}
	// Dos usuarios incrementando en paralelo no deben bloquearse entre sí
	// de forma incorrecta ni compartir mutex.
	var wg sync.WaitGroup
	counts := map[string]*int{}
	mu := sync.Mutex{}
	for _, u := range []string{"u1", "u2"} {
		v := 0
		counts[u] = &v
	}
	wg.Add(20)
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			m := s.lockFor("u1")
			m.Lock()
			mu.Lock()
			*counts["u1"]++
			mu.Unlock()
			m.Unlock()
		}()
		go func() {
			defer wg.Done()
			m := s.lockFor("u2")
			m.Lock()
			mu.Lock()
			*counts["u2"]++
			mu.Unlock()
			m.Unlock()
		}()
	}
	wg.Wait()
	if *counts["u1"] != 10 || *counts["u2"] != 10 {
		t.Fatalf("locks por usuario deben ser independientes, got %v %v", *counts["u1"], *counts["u2"])
	}
	if s.lockFor("u1") == s.lockFor("u2") {
		t.Fatal("mutex de u1 y u2 deben ser distintos")
	}
}

// --- refresh con servidor OAuth simulado ---

func fakeTokenServer(handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(handler))
}

func TestRefreshInvalidGrant(t *testing.T) {
	srv := fakeTokenServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_grant",
			"error_description": "Token has been expired or revoked.",
		})
	})
	defer srv.Close()
	s := &PGOAuthTokenStore{clientID: "id", clientSecret: "secret", tokenURL: srv.URL, httpClient: srv.Client()}
	_, _, err := s.refresh(context.Background(), "refresh-token-viejo")
	if err == nil {
		t.Fatal("invalid_grant debe devolver error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "invalid") {
		t.Fatalf("error debe mencionar invalid_grant/reconexión, got %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "reconecte") {
		t.Fatalf("error invalid_grant debe pedir reconexión, got %v", err)
	}
}

func TestRefreshSuccess(t *testing.T) {
	srv := fakeTokenServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "nuevo-access-token",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	})
	defer srv.Close()
	s := &PGOAuthTokenStore{clientID: "id", clientSecret: "secret", tokenURL: srv.URL, httpClient: srv.Client()}
	tok, exp, err := s.refresh(context.Background(), "refresh-ok")
	if err != nil {
		t.Fatalf("refresh válido no debe fallar: %v", err)
	}
	if tok != "nuevo-access-token" {
		t.Fatalf("token inesperado, got %q", tok)
	}
	if exp.IsZero() {
		t.Fatal("expiry no debe ser cero")
	}
}

func TestRefreshServerErrorMapsUnavailable(t *testing.T) {
	srv := fakeTokenServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "server_error"})
	})
	defer srv.Close()
	s := &PGOAuthTokenStore{clientID: "id", clientSecret: "secret", tokenURL: srv.URL, httpClient: srv.Client()}
	_, _, err := s.refresh(context.Background(), "rt")
	if err == nil {
		t.Fatal("500 del token server debe fallar")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "google_unavailable") {
		t.Fatalf("error 500 debe mapear a google_unavailable, got %v", err)
	}
}

func TestRefreshEmptyAccessToken(t *testing.T) {
	srv := fakeTokenServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	})
	defer srv.Close()
	s := &PGOAuthTokenStore{clientID: "id", clientSecret: "secret", tokenURL: srv.URL, httpClient: srv.Client()}
	_, _, err := s.refresh(context.Background(), "rt")
	if err == nil {
		t.Fatal("access_token vacío debe fallar")
	}
}

// --- GetValidAccessToken: validación sin BD ---

func TestGetValidAccessTokenEmptyUser(t *testing.T) {
	s := &PGOAuthTokenStore{}
	for _, uid := range []string{"", "   "} {
		_, err := s.GetValidAccessToken(context.Background(), uid)
		if err == nil || !IsOAuthError(err) {
			t.Fatalf("user vacío debe dar *OAuthError tipado (403), got %v", err)
		}
		if strings.Contains(err.Error(), "no oauth connection found") {
			t.Fatalf("el mensaje no debe exponer texto interno al cliente: %v", err)
		}
	}
}

func TestGetValidAccessTokenNilPool(t *testing.T) {
	s := &PGOAuthTokenStore{}
	_, err := s.GetValidAccessToken(context.Background(), "user-x")
	if err == nil || !IsOAuthError(err) {
		t.Fatalf("sin pool debe dar *OAuthError tipado (403), got %v", err)
	}
}
