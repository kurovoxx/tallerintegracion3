package mail

import (
	"strings"
	"testing"
	"time"
)

func TestBuildMeetingMessage_Completo(t *testing.T) {
	m := BuildMeetingMessage("Demo", "Daily", time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC), true, "llevar apuntes")
	if m.Subject != "Grupo Demo: Daily" {
		t.Fatalf("asunto: %q", m.Subject)
	}
	for _, want := range []string{"Daily", "Demo", "2026-11-20", "llevar apuntes"} {
		if !strings.Contains(m.Body, want) {
			t.Fatalf("cuerpo sin %q: %q", want, m.Body)
		}
	}
}

func TestBuildMeetingMessage_Minimo(t *testing.T) {
	m := BuildMeetingMessage("", "  ", time.Time{}, false, "")
	if m.Subject != "Nueva reunión: Reunión" {
		t.Fatalf("asunto: %q", m.Subject)
	}
	if !strings.Contains(m.Body, "Sigma Academy") {
		t.Fatalf("cuerpo: %q", m.Body)
	}
}

func TestMeetingMailer_ReadyYValidaciones(t *testing.T) {
	full := &MeetingMailer{Host: "smtp.gmail.com", Port: "587", Username: "noreply@example.com", Password: "x"}
	if !full.Ready() {
		t.Fatal("configurado debe estar Ready")
	}
	if (&MeetingMailer{}).Ready() {
		t.Fatal("vacío no debe estar Ready")
	}
	// Sin red: falla por dial, no por validación (dirección válida pasa el parse).
	err := full.SendMeetingInvite(t.Context(), []string{"a@x.com"}, "G", "T", time.Now(), false, "")
	if err == nil || strings.Contains(err.Error(), "dirección inválida") {
		t.Fatalf("sin red debe fallar por dial, got %v", err)
	}
	if err := full.SendMeetingInvite(t.Context(), []string{"no-email"}, "G", "T", time.Now(), false, ""); err == nil {
		t.Fatal("email inválido debe fallar antes de red")
	}
	if err := full.SendMeetingInvite(t.Context(), nil, "G", "T", time.Now(), false, ""); err == nil {
		t.Fatal("sin destinatarios debe fallar")
	}
}
