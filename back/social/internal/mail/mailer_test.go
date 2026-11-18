package mail

import (
	"strings"
	"testing"
	"time"
)

func TestBuildMeetingMessage_Completo(t *testing.T) {
	m := BuildMeetingMessage("Demo", "Daily", time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC), nil, true, "llevar apuntes", "https://meet.google.com/abc-defg-hij")
	if m.Subject != "Grupo Demo: Daily" {
		t.Fatalf("asunto: %q", m.Subject)
	}
	for _, want := range []string{"Daily", "Demo", "2026-11-20", "llevar apuntes", "https://meet.google.com/abc-defg-hij"} {
		if !strings.Contains(m.TextBody, want) {
			t.Fatalf("texto sin %q: %q", want, m.TextBody)
		}
		if !strings.Contains(m.HTMLBody, want) {
			t.Fatalf("html sin %q", want)
		}
	}
	if !strings.Contains(m.HTMLBody, `href="https://meet.google.com/abc-defg-hij"`) {
		t.Fatal("html debe incluir el enlace como href")
	}
	if !strings.Contains(m.HTMLBody, "#FFCC00") || !strings.Contains(m.HTMLBody, "Sigma Academy") {
		t.Fatal("html debe incluir el footer de Sigma Academy")
	}
}

func TestBuildMeetingMessage_ZonaHorariaSantiago(t *testing.T) {
	// 19:00 UTC = 16:00 en Chile verano (UTC-3)
	utc := time.Date(2026, 11, 16, 19, 0, 0, 0, time.UTC)
	clst := time.FixedZone("CLST", -3*3600)
	m := BuildMeetingMessage("Demo", "Reunion", utc, clst, true, "", "")
	if !strings.Contains(m.TextBody, "16:00") {
		t.Fatalf("texto debe mostrar hora local 16:00, got: %q", m.TextBody)
	}
	if !strings.Contains(m.TextBody, "2026-11-16") {
		t.Fatalf("texto debe mostrar fecha, got: %q", m.TextBody)
	}
	if !strings.Contains(m.HTMLBody, "16:00") {
		t.Fatalf("html debe mostrar hora local 16:00, got: %q", m.HTMLBody)
	}
}

func TestBuildMeetingMessage_Minimo(t *testing.T) {
	m := BuildMeetingMessage("", "  ", time.Time{}, nil, false, "", "")
	if m.Subject != "Nueva reunión: Reunión" {
		t.Fatalf("asunto: %q", m.Subject)
	}
	if !strings.Contains(m.TextBody, "Sigma Academy") {
		t.Fatalf("texto: %q", m.TextBody)
	}
}

func TestBuildMeetingMessage_SalaSinURL(t *testing.T) {
	m := BuildMeetingMessage("", "Daily", time.Time{}, nil, false, "", "Sala B-204")
	if !strings.Contains(m.HTMLBody, "Sala B-204") {
		t.Fatal("html debe mostrar la sala")
	}
	if strings.Contains(m.HTMLBody, `href="Sala B-204"`) {
		t.Fatal("una sala no debe convertirse en href")
	}
}

func TestBuildMeetingMessage_EscapaContenido(t *testing.T) {
	m := BuildMeetingMessage("G", `<script>alert("x")</script>`, time.Time{}, nil, false, "a & b", `javascript:alert(1)`)
	if strings.Contains(m.HTMLBody, "<script>") {
		t.Fatal("el título debe ir escapado")
	}
	if strings.Contains(m.HTMLBody, `href="javascript:alert(1)"`) {
		t.Fatal("solo se permiten enlaces http(s)")
	}
	if !strings.Contains(m.HTMLBody, "a &amp; b") {
		t.Fatal("la descripción debe ir escapada")
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
	err := full.SendMeetingInvite(t.Context(), []string{"a@x.com"}, "G", "T", time.Now(), nil, false, "", "")
	if err == nil || strings.Contains(err.Error(), "dirección inválida") {
		t.Fatalf("sin red debe fallar por dial, got %v", err)
	}
	if err := full.SendMeetingInvite(t.Context(), []string{"no-email"}, "G", "T", time.Now(), nil, false, "", ""); err == nil {
		t.Fatal("email inválido debe fallar antes de red")
	}
	if err := full.SendMeetingInvite(t.Context(), nil, "G", "T", time.Now(), nil, false, "", ""); err == nil {
		t.Fatal("sin destinatarios debe fallar")
	}
}
