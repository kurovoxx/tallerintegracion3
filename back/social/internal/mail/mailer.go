package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// MeetingMailer envía avisos de reunión por SMTP (tarea 3_5_15, solo Gmail).
// Mismo patrón que ResetMailer de auth: STARTTLS obligatorio, credenciales
// solo en el servidor. Remitente noreply.
type MeetingMailer struct{ Host, Port, Username, Password string }

func (m *MeetingMailer) Ready() bool {
	return m.Host != "" && m.Port != "" && m.Username != "" && m.Password != ""
}

// MeetingMessage es el contenido ya compuesto (función pura, testeable).
type MeetingMessage struct {
	Subject string
	Body    string
}

// BuildMeetingMessage compone asunto y cuerpo del aviso.
// groupName vacío = asunto sin grupo.
func BuildMeetingMessage(groupName, title string, scheduled time.Time, hasSchedule bool, description string) MeetingMessage {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Reunión"
	}
	subject := "Nueva reunión: " + title
	if g := strings.TrimSpace(groupName); g != "" {
		subject = "Grupo " + g + ": " + title
	}
	var b strings.Builder
	b.WriteString("Se agendó una reunión en Sigma Academy.\n\n")
	b.WriteString("Título: " + title + "\n")
	if g := strings.TrimSpace(groupName); g != "" {
		b.WriteString("Grupo: " + g + "\n")
	}
	if hasSchedule {
		b.WriteString("Fecha: " + scheduled.Format("2006-01-02 15:04 MST") + "\n")
	}
	if d := strings.TrimSpace(description); d != "" {
		b.WriteString("\n" + d + "\n")
	}
	return MeetingMessage{Subject: subject, Body: b.String()}
}

// SendMeetingInvite envía el aviso a todos los destinatarios en una sesión.
func (m *MeetingMailer) SendMeetingInvite(ctx context.Context, to []string, groupName, title string, scheduled time.Time, hasSchedule bool, description string) error {
	addrs := make([]string, 0, len(to))
	for _, r := range to {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		if _, err := mail.ParseAddress(r); err != nil {
			return fmt.Errorf("dirección inválida %q", r)
		}
		addrs = append(addrs, r)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("sin destinatarios")
	}
	if !m.Ready() {
		return fmt.Errorf("SMTP not configured")
	}
	if strings.ContainsAny(m.Username, "\r\n") {
		return fmt.Errorf("invalid address")
	}
	msg := BuildMeetingMessage(groupName, title, scheduled, hasSchedule, description)

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(m.Host, m.Port))
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	client, err := smtp.NewClient(conn, m.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if err := client.StartTLS(&tls.Config{ServerName: m.Host, MinVersion: tls.VersionTLS12}); err != nil {
		return err
	}
	if err := client.Auth(smtp.PlainAuth("", m.Username, m.Password, m.Host)); err != nil {
		return err
	}
	if err := client.Mail(m.Username); err != nil {
		return err
	}
	for _, r := range addrs {
		if err := client.Rcpt(r); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	raw := "From: " + (&mail.Address{Name: "Sigma Academy", Address: m.Username}).String() + "\r\n" +
		"To: " + strings.Join(addrs, ", ") + "\r\nSubject: " + msg.Subject + "\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" +
		msg.Body
	if _, err := fmt.Fprint(writer, raw); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
