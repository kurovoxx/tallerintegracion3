package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"golang.org/x/oauth2"
)

// Reutiliza newTestTokenServer/newTestUserinfoServer de drive_oauth_service_test.go.

func calendarTestProvider(tokenURL, userinfoURL string) *ConfigCalendarOAuthProvider {
	return &ConfigCalendarOAuthProvider{
		ClientID:     "test-client-id",
		ClientSecret: "test-secret",
		RedirectURI:  "http://localhost/callback",
		Endpoint:     oauth2.Endpoint{TokenURL: tokenURL},
		UserinfoURL:  userinfoURL,
	}
}

func TestCalendarProvider_ExitoSimulado(t *testing.T) {
	var tokenHit, userinfoHit bool
	tokenSrv := newTestTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		tokenHit = true
		if r.Method != http.MethodPost {
			t.Errorf("token endpoint debe ser POST, got %s", r.Method)
		}
		r.ParseForm()
		if r.Form.Get("grant_type") != "authorization_code" {
			t.Errorf("grant_type esperado authorization_code, got %s", r.Form.Get("grant_type"))
		}
		if r.Form.Get("code") != "code123" {
			t.Errorf("code esperado code123, got %s", r.Form.Get("code"))
		}
		// Nota: el grant authorization_code no transmite scope en el POST
		// (se fijó en la URL de autorización del front), por eso no se aserta aquí.
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"acccal123","refresh_token":"refreshcal123","expires_in":3600,"token_type":"Bearer"}`))
	})
	defer tokenSrv.Close()
	userinfoSrv := newTestUserinfoServer(t, func(w http.ResponseWriter, r *http.Request) {
		userinfoHit = true
		if r.Header.Get("Authorization") != "Bearer acccal123" {
			t.Errorf("Authorization Bearer acccal123 esperado, got %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"email":"caluser@gmail.com"}`))
	})
	defer userinfoSrv.Close()

	provider := calendarTestProvider(tokenSrv.URL, userinfoSrv.URL)
	result, err := provider.Exchange(context.Background(), "code123")
	if err != nil {
		t.Fatalf("esperado éxito, got %v", err)
	}
	if !tokenHit || !userinfoHit {
		t.Fatal("token y userinfo deben ser llamados")
	}
	if result.AccessToken != "acccal123" || result.RefreshToken == nil || *result.RefreshToken != "refreshcal123" ||
		result.ExpiresAt == nil || result.ExternalEmail == nil || *result.ExternalEmail != "caluser@gmail.com" {
		t.Fatalf("resultado incorrecto: %+v", result)
	}
}

func TestCalendarProvider_ConfigVacia_GoogleUnavailable(t *testing.T) {
	provider := &ConfigCalendarOAuthProvider{ClientID: "", ClientSecret: "secret", RedirectURI: "http://localhost/callback"}
	_, err := provider.Exchange(context.Background(), "code123")
	if !errors.Is(err, ErrGoogleUnavailable) {
		t.Fatalf("esperado ErrGoogleUnavailable si config vacía, got %v", err)
	}
}

func TestCalendarProvider_TokenEndpoint_400_InvalidGrant(t *testing.T) {
	tokenSrv := newTestTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		w.Write([]byte(`{"error":"invalid_grant","error_description":"Bad Request"}`))
	})
	defer tokenSrv.Close()
	provider := calendarTestProvider(tokenSrv.URL, "http://unused")
	_, err := provider.Exchange(context.Background(), "bad-code")
	if !errors.Is(err, ErrInvalidOAuthCode) {
		t.Fatalf("esperado ErrInvalidOAuthCode para 400 invalid_grant, got %v", err)
	}
}

func TestCalendarProvider_TokenEndpoint_429_GoogleUnavailable(t *testing.T) {
	tokenSrv := newTestTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte(`{}`))
	})
	defer tokenSrv.Close()
	provider := calendarTestProvider(tokenSrv.URL, "http://unused")
	_, err := provider.Exchange(context.Background(), "code123")
	if !errors.Is(err, ErrGoogleUnavailable) {
		t.Fatalf("esperado ErrGoogleUnavailable para 429, got %v", err)
	}
}

func TestCalendarProvider_TokenEndpoint_500_GoogleUnavailable(t *testing.T) {
	tokenSrv := newTestTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	})
	defer tokenSrv.Close()
	provider := calendarTestProvider(tokenSrv.URL, "http://unused")
	_, err := provider.Exchange(context.Background(), "code123")
	if !errors.Is(err, ErrGoogleUnavailable) {
		t.Fatalf("esperado ErrGoogleUnavailable para 500, got %v", err)
	}
}

func TestCalendarProvider_UserinfoSinEmail_Error(t *testing.T) {
	tokenSrv := newTestTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"acc123","expires_in":3600,"token_type":"Bearer"}`))
	})
	defer tokenSrv.Close()
	userinfoSrv := newTestUserinfoServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"email":""}`))
	})
	defer userinfoSrv.Close()
	provider := calendarTestProvider(tokenSrv.URL, userinfoSrv.URL)
	_, err := provider.Exchange(context.Background(), "code123")
	if err == nil || errors.Is(err, ErrGoogleUnavailable) {
		t.Fatalf("se esperaba error de userinfo (no unavailable), got %v", err)
	}
}

func TestCalendarProvider_Refresh_Exito(t *testing.T) {
	tokenSrv := newTestTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("grant_type") != "refresh_token" {
			t.Errorf("grant_type esperado refresh_token, got %s", r.Form.Get("grant_type"))
		}
		if r.Form.Get("refresh_token") != "refreshcal123" {
			t.Errorf("refresh_token esperado refreshcal123, got %s", r.Form.Get("refresh_token"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"nuevoacc","expires_in":3600,"token_type":"Bearer"}`))
	})
	defer tokenSrv.Close()
	provider := &ConfigCalendarOAuthProvider{
		ClientID: "test-client-id", ClientSecret: "test-secret",
		Endpoint: oauth2.Endpoint{TokenURL: tokenSrv.URL},
	}
	result, err := provider.Refresh(context.Background(), "refreshcal123")
	if err != nil {
		t.Fatalf("esperado éxito, got %v", err)
	}
	if result.AccessToken != "nuevoacc" || result.ExpiresAt == nil {
		t.Fatalf("resultado incorrecto: %+v", result)
	}
}

func TestCalendarProvider_Refresh_InvalidGrant_ConexionInvalida(t *testing.T) {
	tokenSrv := newTestTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		w.Write([]byte(`{"error":"invalid_grant"}`))
	})
	defer tokenSrv.Close()
	provider := &ConfigCalendarOAuthProvider{
		ClientID: "id", ClientSecret: "secret",
		Endpoint: oauth2.Endpoint{TokenURL: tokenSrv.URL},
	}
	_, err := provider.Refresh(context.Background(), "refresh-malo")
	if !errors.Is(err, ErrCalendarConnectionInvalid) {
		t.Fatalf("esperado ErrCalendarConnectionInvalid, got %v", err)
	}
}

func TestCalendarProvider_Refresh_Vacio_SinHTTP(t *testing.T) {
	provider := &ConfigCalendarOAuthProvider{ClientID: "id", ClientSecret: "secret"}
	_, err := provider.Refresh(context.Background(), "  ")
	if !errors.Is(err, ErrCalendarConnectionInvalid) {
		t.Fatalf("esperado ErrCalendarConnectionInvalid sin HTTP, got %v", err)
	}
}

func TestCalendarProvider_Refresh_500_GoogleUnavailable(t *testing.T) {
	tokenSrv := newTestTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	})
	defer tokenSrv.Close()
	provider := &ConfigCalendarOAuthProvider{
		ClientID: "id", ClientSecret: "secret",
		Endpoint: oauth2.Endpoint{TokenURL: tokenSrv.URL},
	}
	_, err := provider.Refresh(context.Background(), "refreshcal123")
	if !errors.Is(err, ErrGoogleUnavailable) {
		t.Fatalf("esperado ErrGoogleUnavailable para 500, got %v", err)
	}
}
