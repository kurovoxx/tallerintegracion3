package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func authFakeServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler(w, r)
	}))
}

func TestGateway_GetToken_Exito(t *testing.T) {
	var gotKey, gotPath, gotQuery string
	srv := authFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Internal-Key")
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("user_id")
		if r.Method != http.MethodGet {
			t.Errorf("método debe ser GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"acccal-xyz"}`))
	})
	defer srv.Close()

	gw := &AuthCalendarGateway{BaseURL: srv.URL, InternalKey: "k-secreta"}
	tok, err := gw.GetToken(context.Background(), "user-1")
	if err != nil || tok != "acccal-xyz" {
		t.Fatalf("token inesperado: %q, %v", tok, err)
	}
	if gotKey != "k-secreta" {
		t.Fatalf("X-Internal-Key esperado k-secreta, got %q", gotKey)
	}
	if gotPath != "/internal/oauth/calendar-token" {
		t.Fatalf("path inesperado: %q", gotPath)
	}
	if gotQuery != "user-1" {
		t.Fatalf("user_id inesperado: %q", gotQuery)
	}
}

func TestGateway_GetToken_MapeoErrores(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"sin conexión", http.StatusNotFound, `{"error":{"code":"not_connected"}}`, ErrCalendarNotConnected},
		{"revocada", http.StatusForbidden, `{"error":{"code":"calendar_connection_invalid"}}`, ErrCalendarConnectionInvalid},
		{"auth caído", http.StatusBadGateway, `{}`, ErrCalendarGatewayUnavailable},
		{"respuesta inválida", http.StatusOK, `{no-json`, ErrCalendarGatewayUnavailable},
		{"ok sin token", http.StatusOK, `{}`, ErrCalendarGatewayUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := authFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			})
			defer srv.Close()
			gw := &AuthCalendarGateway{BaseURL: srv.URL}
			_, err := gw.GetToken(context.Background(), "user-1")
			if !errors.Is(err, tc.want) {
				t.Fatalf("esperado %v, got %v", tc.want, err)
			}
		})
	}
}

func TestGateway_GetToken_SinHTTP(t *testing.T) {
	gw := &AuthCalendarGateway{BaseURL: "http://unused"}
	if _, err := gw.GetToken(context.Background(), "  "); !errors.Is(err, ErrCalendarNotConnected) {
		t.Fatalf("user vacío no debe hacer HTTP, got %v", err)
	}
	if _, err := (&AuthCalendarGateway{}).GetToken(context.Background(), "user-1"); !errors.Is(err, ErrCalendarGatewayUnavailable) {
		t.Fatalf("sin BaseURL no debe hacer HTTP, got %v", err)
	}
}

func TestGateway_ReportRevoked_PostCorrecto(t *testing.T) {
	var gotMethod, gotPath, gotKey, gotBody string
	srv := authFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotKey = r.Header.Get("X-Internal-Key")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusNoContent)
	})
	defer srv.Close()

	gw := &AuthCalendarGateway{BaseURL: srv.URL, InternalKey: "k-secreta"}
	if err := gw.ReportRevoked(context.Background(), "user-9"); err != nil {
		t.Fatalf("report no debe fallar con 204, got %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/internal/oauth/calendar-revoked" {
		t.Fatalf("request inesperado: %s %s", gotMethod, gotPath)
	}
	if gotKey != "k-secreta" {
		t.Fatalf("X-Internal-Key esperado, got %q", gotKey)
	}
	if !strings.Contains(gotBody, "user-9") {
		t.Fatalf("body debe llevar el user_id, got %q", gotBody)
	}
}

func TestGateway_ReportRevoked_UsuarioVacio_NoOp(t *testing.T) {
	gw := &AuthCalendarGateway{BaseURL: "http://unused"}
	if err := gw.ReportRevoked(context.Background(), "  "); err != nil {
		t.Fatalf("usuario vacío no debe hacer HTTP ni fallar, got %v", err)
	}
}
