package calendar

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRESTClient_AttendeesEnPayload(t *testing.T) {
	var gotNoAttendeesKey = true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Attendees []map[string]string `json:"attendees"`
		}
		raw := make(map[string]any)
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatal(err)
		}
		if _, ok := raw["attendees"]; !ok {
			gotNoAttendeesKey = true
		} else {
			gotNoAttendeesKey = false
		}
		_ = body
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"evt-1"}`))
	}))
	defer srv.Close()

	c := &RESTClient{httpClient: srv.Client(), baseURL: srv.URL}
	// Sin attendees: la clave debe omitirse (omitempty).
	if _, err := c.CreateEvent(t.Context(), "tok", Event{Summary: "S", Start: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if !gotNoAttendeesKey {
		t.Fatal("sin invitados la clave attendees debe omitirse")
	}
}

func TestRESTClient_AttendeesConEmails(t *testing.T) {
	var got []map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Attendees []map[string]string `json:"attendees"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		got = body.Attendees
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"evt-2"}`))
	}))
	defer srv.Close()

	c := &RESTClient{httpClient: srv.Client(), baseURL: srv.URL}
	ev := Event{Summary: "S", Start: time.Now().Add(time.Hour), Attendees: []string{"a@x.cl", "b@y.cl"}}
	if _, err := c.CreateEvent(t.Context(), "tok", ev); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0]["email"] != "a@x.cl" || got[1]["email"] != "b@y.cl" {
		t.Fatalf("attendees inesperados: %v", got)
	}
}
