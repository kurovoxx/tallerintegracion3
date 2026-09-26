package discord

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFormatMeetingMessage(t *testing.T) {
	text := FormatMeetingMessage(Message{
		Title:       "Reunión X",
		Description: "detalle",
		ScheduledAt: time.Date(2026, 9, 25, 15, 0, 0, 0, time.UTC),
		HasSchedule: true,
	})
	if !strings.Contains(text, "Reunión X") || !strings.Contains(text, "detalle") || !strings.Contains(text, "2026-09-25") {
		t.Fatalf("mensaje incompleto: %q", text)
	}
	plain := FormatMeetingMessage(Message{Title: "Solo título"})
	if strings.Contains(plain, "🕒") {
		t.Fatalf("sin fecha no debe llevar hora: %q", plain)
	}
}

func TestWebhookClient_Exito(t *testing.T) {
	var gotMethod, gotCT string
	var gotBody struct {
		Content string `json:"content"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotCT = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("body JSON inválido: %v", err)
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()

	cli := NewWebhookClient()
	err := cli.SendMeetingCreated(t.Context(), srv.URL, Message{Title: "Reunión", HasSchedule: false})
	if err != nil {
		t.Fatalf("esperado éxito, got %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("método debe ser POST, got %s", gotMethod)
	}
	if gotCT != "application/json" {
		t.Fatalf("Content-Type esperado application/json, got %q", gotCT)
	}
	if !strings.Contains(gotBody.Content, "Reunión") {
		t.Fatalf("content inesperado: %q", gotBody.Content)
	}
}

func TestWebhookClient_ErrorDiscord(t *testing.T) {
	for _, code := range []int{400, 401, 404, 429} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
			w.Write([]byte(`{"message":"webhook inválido"}`))
		}))
		err := NewWebhookClient().SendMeetingCreated(t.Context(), srv.URL, Message{Title: "X"})
		srv.Close()
		var we *WebhookError
		if !errors.As(err, &we) || we.Code != code {
			t.Fatalf("código %d debe dar WebhookError, got %v", code, err)
		}
	}
}

func TestWebhookClient_ValidacionLocal_SinHTTP(t *testing.T) {
	cli := NewWebhookClient()
	if err := cli.SendMeetingCreated(t.Context(), "  ", Message{Title: "X"}); err == nil {
		t.Fatal("webhook vacía debe fallar sin HTTP")
	}
	if err := cli.SendMeetingCreated(t.Context(), "https://discord.com/api/webhooks/1/x", Message{}); err == nil {
		t.Fatal("título vacío debe fallar sin HTTP")
	}
}
