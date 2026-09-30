package stream

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRESTClient_Exito(t *testing.T) {
	var gotAuth, gotAuthType, gotCT, gotPath, gotQuery string
	var gotBody struct {
		State bool              `json:"state"`
		Data  map[string]string `json:"data"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAuthType = r.Header.Get("Stream-Auth-Type")
		gotCT = r.Header.Get("Content-Type")
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("api_key")
		if r.Method != http.MethodPost {
			t.Errorf("método debe ser POST, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("body JSON inválido: %v", err)
		}
		w.WriteHeader(201)
		w.Write([]byte(`{"channel":{"id":"group-abc"}}`))
	}))
	defer srv.Close()

	c := &RESTClient{httpClient: srv.Client(), baseURL: srv.URL, apiKey: "key123", apiSecret: "secret123"}
	// Get-or-create oficial: POST /channels/{type}/{id}/query (POST /channels
	// a secas es QueryChannels y NO crea nada aunque responda 200).
	if err := c.CreateChannel(t.Context(), "messaging", "group-abc", "Grupo A", "u1"); err != nil {
		t.Fatalf("esperado éxito, got %v", err)
	}
	if gotPath != "/channels/messaging/group-abc/query" {
		t.Fatalf("path inesperado: %q", gotPath)
	}
	if gotQuery != "key123" {
		t.Fatalf("api_key inesperado: %q", gotQuery)
	}
	if gotAuthType != "jwt" {
		t.Fatalf("Stream-Auth-Type esperado jwt, got %q", gotAuthType)
	}
	if gotCT != "application/json" {
		t.Fatalf("Content-Type esperado application/json, got %q", gotCT)
	}
	if !gotBody.State || gotBody.Data["name"] != "Grupo A" || gotBody.Data["created_by_id"] != "u1" {
		t.Fatalf("payload inesperado: %+v", gotBody)
	}
	// El server token debe ser un JWT HS256 de 3 partes verificable con el secret
	parts := strings.Split(gotAuth, ".")
	if len(parts) != 3 {
		t.Fatalf("Authorization debe ser JWT de 3 partes, got %q", gotAuth)
	}
	if _, err := serverToken("secret123"); err != nil {
		t.Fatalf("serverToken no debe fallar: %v", err)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !strings.Contains(string(payload), `"server":true`) {
		t.Fatalf("payload del server token inesperado: %q", string(payload))
	}
}

func TestRESTClient_EnsureMember(t *testing.T) {
	var gotPath string
	var gotBody struct {
		AddMembers []string `json:"add_members"`
	}
	var rawBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Errorf("método debe ser POST, got %s", r.Method)
		}
		raw, _ := io.ReadAll(r.Body)
		rawBody = string(raw)
		if err := json.Unmarshal([]byte(rawBody), &gotBody); err != nil {
			t.Errorf("body JSON inválido: %v", err)
		}
		w.WriteHeader(200)
		w.Write([]byte(`{"members":[{"user_id":"u1"}]}`))
	}))
	defer srv.Close()

	c := &RESTClient{httpClient: srv.Client(), baseURL: srv.URL, apiKey: "key123", apiSecret: "secret123"}
	if err := c.EnsureMember(t.Context(), "messaging", "group-abc", "u1"); err != nil {
		t.Fatalf("esperado éxito, got %v", err)
	}
	if gotPath != "/channels/messaging/group-abc" {
		t.Fatalf("path inesperado: %q", gotPath)
	}
	if len(gotBody.AddMembers) != 1 || gotBody.AddMembers[0] != "u1" {
		t.Fatalf("add_members inesperado: %+v", gotBody)
	}
	// El secret jamás viaja en el body; solo firma el Authorization server-side.
	if strings.Contains(rawBody, "secret123") {
		t.Fatalf("el secret no debe ir en el body: %q", rawBody)
	}
	if err := c.EnsureMember(t.Context(), "messaging", "group-abc", "  "); err == nil {
		t.Fatal("user vacío debe fallar sin HTTP")
	}
}

func TestRESTClient_ErrorStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"message":"canal inválido"}`))
	}))
	defer srv.Close()
	c := &RESTClient{httpClient: srv.Client(), baseURL: srv.URL, apiKey: "k", apiSecret: "s"}
	err := c.CreateChannel(t.Context(), "messaging", "group-x", "G", "u1")
	var se *StreamError
	if !errors.As(err, &se) || se.Code != 400 {
		t.Fatalf("se esperaba StreamError 400, got %v", err)
	}
}

func TestRESTClient_ValidacionLocal_SinHTTP(t *testing.T) {
	c := NewRESTClient("k", "s")
	if err := c.CreateChannel(t.Context(), "", "group-x", "G", "u1"); err == nil {
		t.Fatal("type vacío debe fallar sin HTTP")
	}
	if err := c.CreateChannel(t.Context(), "messaging", "  ", "G", "u1"); err == nil {
		t.Fatal("id vacío debe fallar sin HTTP")
	}
	if err := NewRESTClient("", "s").CreateChannel(t.Context(), "messaging", "group-x", "G", "u1"); err == nil {
		t.Fatal("sin api key debe fallar sin HTTP")
	}
	if err := NewRESTClient("k", "").CreateChannel(t.Context(), "messaging", "group-x", "G", "u1"); err == nil {
		t.Fatal("sin secret debe fallar sin HTTP")
	}
}
