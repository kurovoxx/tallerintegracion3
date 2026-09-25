package calendar

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testEvent() Event {
	return Event{
		Summary:     "Reunión prueba",
		Description: "descripción prueba",
		Start:       time.Date(2026, 9, 25, 15, 0, 0, 0, time.UTC),
	}
}

func clientAgainst(srv *httptest.Server) *RESTClient {
	return &RESTClient{httpClient: srv.Client(), baseURL: srv.URL}
}

func TestRESTClient_Exito(t *testing.T) {
	var gotAuth, gotCT, gotPath string
	var gotBody struct {
		Summary     string `json:"summary"`
		Description string `json:"description"`
		Start       struct {
			DateTime string `json:"dateTime"`
		} `json:"start"`
		End struct {
			DateTime string `json:"dateTime"`
		} `json:"end"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		gotPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Errorf("método debe ser POST, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("body JSON inválido: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"evt_google_123"}`))
	}))
	defer srv.Close()

	id, err := clientAgainst(srv).CreateEvent(t.Context(), "tok-abc", testEvent())
	if err != nil {
		t.Fatalf("esperado éxito, got %v", err)
	}
	if id != "evt_google_123" {
		t.Fatalf("event id inesperado: %q", id)
	}
	if gotAuth != "Bearer tok-abc" {
		t.Fatalf("Authorization esperado Bearer tok-abc, got %q", gotAuth)
	}
	if gotCT != "application/json" {
		t.Fatalf("Content-Type esperado application/json, got %q", gotCT)
	}
	if gotPath != "/calendar/v3/calendars/primary/events" {
		t.Fatalf("path inesperado: %q", gotPath)
	}
	if gotBody.Summary != "Reunión prueba" || gotBody.Description != "descripción prueba" {
		t.Fatalf("payload inesperado: %+v", gotBody)
	}
	if !strings.Contains(gotBody.Start.DateTime, "2026-09-25T15:00:00") {
		t.Fatalf("start debe ser RFC3339 con la fecha de la reunión, got %q", gotBody.Start.DateTime)
	}
	// Sin fin explícito -> default 1h
	start, _ := time.Parse(time.RFC3339, gotBody.Start.DateTime)
	end, _ := time.Parse(time.RFC3339, gotBody.End.DateTime)
	if end.Sub(start) != time.Hour {
		t.Fatalf("duración default debe ser 1h, got %v", end.Sub(start))
	}
}

func TestRESTClient_NoAutorizado_Reportable(t *testing.T) {
	for _, code := range []int{401, 403} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
			w.Write([]byte(`{"error":"revocado"}`))
		}))
		_, err := clientAgainst(srv).CreateEvent(t.Context(), "tok-malo", testEvent())
		srv.Close()
		if err == nil || !IsUnauthorized(err) {
			t.Fatalf("código %d debe ser reportable (IsUnauthorized), got %v", code, err)
		}
	}
}

func TestRESTClient_ErrorGoogle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()
	_, err := clientAgainst(srv).CreateEvent(t.Context(), "tok", testEvent())
	var ce *CalendarError
	if !errors.As(err, &ce) || ce.Code != 500 {
		t.Fatalf("se esperaba CalendarError 500, got %v", err)
	}
	if IsUnauthorized(err) {
		t.Fatalf("un 500 no debe marcarse como revocación")
	}
}

func TestRESTClient_RespuestaInvalida(t *testing.T) {
	// JSON malformado
	badJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{no-json`))
	}))
	_, err := clientAgainst(badJSON).CreateEvent(t.Context(), "tok", testEvent())
	badJSON.Close()
	if err == nil {
		t.Fatal("JSON malformado debe fallar")
	}
	// 200 sin id
	noID := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"kind":"calendar#event"}`))
	}))
	_, err = clientAgainst(noID).CreateEvent(t.Context(), "tok", testEvent())
	noID.Close()
	if err == nil {
		t.Fatal("respuesta sin id debe fallar")
	}
}

func TestRESTClient_ValidacionLocal_SinHTTP(t *testing.T) {
	c := NewRESTClient()
	if _, err := c.CreateEvent(t.Context(), "  ", testEvent()); !IsUnauthorized(err) {
		t.Fatalf("token vacío debe ser 401 local, got %v", err)
	}
	if _, err := c.CreateEvent(t.Context(), "tok", Event{}); err == nil {
		t.Fatal("summary vacío debe fallar sin HTTP")
	}
}
