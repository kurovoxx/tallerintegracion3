package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestStreamToken_IssueOK(t *testing.T) {
	gid, admin, member := uuid.NewString(), uuid.NewString(), uuid.NewString()
	store := NewMemoryMeetingStore()
	store.AddGroup(gid, admin, member)
	store.SetStreamChannel(gid, "group-canal-1")
	svc := NewStreamTokenService(store, "test-secret")

	token, channelID, err := svc.IssueToken(context.Background(), gid, member)
	if err != nil {
		t.Fatalf("miembro debe obtener token: %v", err)
	}
	if channelID != "group-canal-1" {
		t.Fatalf("channel inesperado: %q", channelID)
	}
	// Verifica firma HS256 y claim user_id con el secret.
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token debe ser JWT de 3 partes, got %q", token)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("payload inválido: %v", err)
	}
	var claims map[string]string
	if err := json.Unmarshal(payload, &claims); err != nil || claims["user_id"] != member {
		t.Fatalf("claim user_id inesperado: %s", string(payload))
	}
	// Verifica firma HS256 con el secret (y que otro secret no valida).
	mac := hmac.New(sha256.New, []byte("test-secret"))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[2]), []byte(want)) {
		t.Fatal("firma inválida con el secret")
	}
	mac2 := hmac.New(sha256.New, []byte("otro-secret"))
	mac2.Write([]byte(parts[0] + "." + parts[1]))
	if hmac.Equal([]byte(parts[2]), []byte(base64.RawURLEncoding.EncodeToString(mac2.Sum(nil)))) {
		t.Fatal("firma debe fallar con otro secret")
	}
}

func TestStreamToken_SinFila_UsaDeterministico(t *testing.T) {
	gid, admin, _ := uuid.NewString(), uuid.NewString(), uuid.NewString()
	store := NewMemoryMeetingStore()
	store.AddGroup(gid, admin)
	svc := NewStreamTokenService(store, "test-secret")

	_, channelID, err := svc.IssueToken(context.Background(), gid, admin)
	if err != nil {
		t.Fatalf("sin fila debe usar determinístico, got %v", err)
	}
	if channelID != ChannelIDForGroup(gid) {
		t.Fatalf("channel inesperado: %q", channelID)
	}
}

func TestStreamToken_Errores(t *testing.T) {
	gid, admin, member, stranger := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	store := NewMemoryMeetingStore()
	store.AddGroup(gid, admin, member)
	svc := NewStreamTokenService(store, "test-secret")

	if _, _, err := svc.IssueToken(context.Background(), gid, stranger); err != ErrForbidden {
		t.Fatalf("no-miembro debe dar ErrForbidden, got %v", err)
	}
	if _, _, err := svc.IssueToken(context.Background(), uuid.NewString(), admin); err != ErrGroupNotFound {
		t.Fatalf("grupo inexistente debe dar ErrGroupNotFound, got %v", err)
	}
	if _, _, err := svc.IssueToken(context.Background(), "no-uuid", admin); err != ErrInvalidGroupID {
		t.Fatalf("grupo inválido debe dar ErrInvalidGroupID, got %v", err)
	}
	sinSecret := NewStreamTokenService(store, "  ")
	if _, _, err := sinSecret.IssueToken(context.Background(), gid, member); err != ErrStreamNotConfigured {
		t.Fatalf("sin secret debe dar ErrStreamNotConfigured, got %v", err)
	}
}
