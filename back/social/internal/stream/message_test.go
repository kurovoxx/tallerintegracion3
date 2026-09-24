package stream

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRESTClient_SendMessage_Exito(t *testing.T) {
	var gotAuth, gotPath, gotQuery string
	var gotBody struct {
		Text   string `json:"text"`
		UserID string `json:"user_id"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("api_key")
		if r.Method != http.MethodPost {
			t.Errorf("método debe ser POST, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("body JSON inválido: %v", err)
		}
		w.WriteHeader(201)
		w.Write([]byte(`{"message":{"id":"msg-1"}}`))
	}))
	defer srv.Close()

	c := &RESTClient{httpClient: srv.Client(), baseURL: srv.URL, apiKey: "key123", apiSecret: "secret123"}
	err := c.SendMessage(t.Context(), "messaging", "group-abc", "user-9", "hola grupo")
	if err != nil {
		t.Fatalf("esperado éxito, got %v", err)
	}
	if gotPath != "/channels/messaging/group-abc/message" {
		t.Fatalf("path inesperado: %q", gotPath)
	}
	if gotQuery != "key123" {
		t.Fatalf("api_key inesperado: %q", gotQuery)
	}
	if len(strings.Split(gotAuth, ".")) != 3 {
		t.Fatalf("Authorization debe ser JWT, got %q", gotAuth)
	}
	if gotBody.Text != "hola grupo" || gotBody.UserID != "user-9" {
		t.Fatalf("payload inesperado: %+v", gotBody)
	}
}

func TestRESTClient_SendMessage_ErrorStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"message":"canal inexistente"}`))
	}))
	defer srv.Close()
	c := &RESTClient{httpClient: srv.Client(), baseURL: srv.URL, apiKey: "k", apiSecret: "s"}
	err := c.SendMessage(t.Context(), "messaging", "group-x", "user-1", "hola")
	var se *StreamError
	if !errors.As(err, &se) || se.Code != 404 {
		t.Fatalf("se esperaba StreamError 404, got %v", err)
	}
}

func TestRESTClient_SendMessage_ValidacionLocal_SinHTTP(t *testing.T) {
	c := NewRESTClient("k", "s")
	if err := c.SendMessage(t.Context(), "messaging", "  ", "u", "hola"); err == nil {
		t.Fatal("channel vacío debe fallar sin HTTP")
	}
	if err := c.SendMessage(t.Context(), "messaging", "group-x", "  ", "hola"); err == nil {
		t.Fatal("sender vacío debe fallar sin HTTP")
	}
	if err := c.SendMessage(t.Context(), "messaging", "group-x", "u", "  "); err == nil {
		t.Fatal("texto vacío debe fallar sin HTTP")
	}
}
