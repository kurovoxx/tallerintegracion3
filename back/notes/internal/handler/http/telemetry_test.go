package http

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNotesTelemetryPreservesResponses(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	router, _, _, _ := setupRouter()
	userID := uuid.NewString()
	token := genToken(userID, "student")
	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	created := request(http.MethodPost, "/notes", `{"title":"Telemetry","visibility":"private","content":"private-markdown-must-not-be-logged"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(created.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{http.MethodGet, "/notes/me", http.StatusOK},
		{http.MethodGet, "/notes/invalid-id", http.StatusNotFound},
		{http.MethodDelete, "/notes/" + body["note_id"], http.StatusNoContent},
	} {
		if response := request(tc.method, tc.path, ""); response.Code != tc.status {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, response.Code, response.Body.String())
		}
	}
	wanted := map[string]bool{
		"[NOTES] INICIO POST /notes":                 false,
		"[NOTES] OK POST /notes | status: 201":       false,
		"[NOTES] DB notes.Create":                    false,
		"[NOTES] DRIVE UPLOAD":                       false,
		"[NOTES] OK GET /notes/me | status: 200":     false,
		"[NOTES] ERROR GET /notes/:id":               false,
		"[NOTES] OK DELETE /notes/:id | status: 204": false,
	}
	logText := logs.String()
	decoder := json.NewDecoder(strings.NewReader(logText))
	for decoder.More() {
		var record map[string]any
		if err := decoder.Decode(&record); err != nil {
			t.Fatal(err)
		}
		message, _ := record["msg"].(string)
		for prefix := range wanted {
			if strings.HasPrefix(message, prefix) {
				wanted[prefix] = true
				if record["user_id"] != userID || record["duration_ms"] == nil && !strings.Contains(message, "INICIO") {
					t.Fatalf("missing correlation or duration: %+v", record)
				}
			}
		}
	}
	for event, found := range wanted {
		if !found {
			t.Errorf("missing event %s", event)
		}
	}
	if strings.Contains(logText, "private-markdown-must-not-be-logged") {
		t.Fatal("note content leaked into telemetry")
	}
}
