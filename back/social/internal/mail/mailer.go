package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"html"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"net/url"
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
	Subject  string
	TextBody string
	HTMLBody string
}

// BuildMeetingMessage compone asunto, texto plano y HTML del aviso.
// groupName vacío = asunto sin grupo. link puede ser una URL o una sala.
func BuildMeetingMessage(groupName, title string, scheduled time.Time, hasSchedule bool, description, link string) MeetingMessage {
	title = cleanLine(title)
	if title == "" {
		title = "Reunión"
	}
	groupName = cleanLine(groupName)
	description = strings.TrimSpace(description)
	link = strings.TrimSpace(link)

	subject := "Nueva reunión: " + title
	if groupName != "" {
		subject = "Grupo " + groupName + ": " + title
	}

	when := ""
	if hasSchedule {
		when = scheduled.Format("2006-01-02 15:04 MST")
	}

	var text strings.Builder
	text.WriteString("Hola,\n\n")
	text.WriteString("Se agendó una nueva reunión en Sigma Academy.\n\n")
	text.WriteString("Título: " + title + "\n")
	if groupName != "" {
		text.WriteString("Grupo: " + groupName + "\n")
	}
	if when != "" {
		text.WriteString("Fecha: " + when + "\n")
	}
	if description != "" {
		text.WriteString("\nDescripción:\n" + description + "\n")
	}
	if link != "" {
		text.WriteString("\nEnlace de la reunión:\n" + link + "\n")
	}
	text.WriteString("\nSigma Academy\nhttps://ti3-brojas.dev.censei.cl\n")

	escTitle := html.EscapeString(title)
	escGroup := html.EscapeString(groupName)
	escWhen := html.EscapeString(when)
	escDescription := strings.ReplaceAll(html.EscapeString(description), "\n", "<br>")
	escLink := html.EscapeString(link)
	href := safeHref(link)

	var details strings.Builder
	if groupName != "" {
		details.WriteString(detailRow("Grupo", escGroup))
	}
	if when != "" {
		details.WriteString(detailRow("Fecha", escWhen))
	}

	var descriptionBlock string
	if description != "" {
		descriptionBlock = `<div style="margin-top:20px;padding:14px 16px;background:#F5F0E8;border-left:4px solid #0055FF;">` +
			`<div style="font-size:11px;font-weight:bold;letter-spacing:1px;text-transform:uppercase;color:#555555;margin-bottom:6px;">Descripción</div>` +
			`<div style="font-size:14px;line-height:1.6;color:#1A1A1A;">` + escDescription + `</div>` +
			`</div>`
	}

	var linkBlock string
	if link != "" {
		if href != "" {
			linkBlock = `<div style="margin-top:24px;">` +
				`<a href="` + html.EscapeString(href) + `" style="display:inline-block;background:#0055FF;color:#FFFFFF;border:2px solid #1A1A1A;padding:11px 18px;font-size:14px;font-weight:bold;text-decoration:none;">Unirse a la reunión</a>` +
				`<div style="margin-top:10px;font-size:12px;line-height:1.5;color:#555555;word-break:break-all;">` + escLink + `</div>` +
				`</div>`
		} else {
			linkBlock = `<div style="margin-top:20px;padding:14px 16px;background:#F5F0E8;border-left:4px solid #FFCC00;">` +
				`<div style="font-size:11px;font-weight:bold;letter-spacing:1px;text-transform:uppercase;color:#555555;margin-bottom:6px;">Enlace o sala</div>` +
				`<div style="font-size:14px;line-height:1.6;color:#1A1A1A;word-break:break-all;">` + escLink + `</div>` +
				`</div>`
		}
	}

	body := `<!DOCTYPE html>
<html lang="es">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>` + escTitle + `</title>
</head>
<body style="margin:0;padding:0;background:#F5F0E8;font-family:Arial,Helvetica,sans-serif;color:#1A1A1A;">
<table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="background:#F5F0E8;padding:24px 12px;">
<tr><td align="center">
<table role="presentation" width="600" cellspacing="0" cellpadding="0" border="0" style="width:100%;max-width:600px;background:#FFFFFF;border:2px solid #1A1A1A;">
<tr><td style="background:#FFCC00;border-bottom:2px solid #1A1A1A;padding:18px 24px;">
<div style="font-size:11px;font-weight:bold;letter-spacing:2px;text-transform:uppercase;color:#1A1A1A;">Sigma Academy</div>
<div style="font-size:24px;font-weight:900;line-height:1.2;margin-top:4px;color:#1A1A1A;">Nueva reunión</div>
</td></tr>
<tr><td style="padding:24px;">
<div style="font-size:20px;font-weight:900;line-height:1.3;color:#1A1A1A;">` + escTitle + `</div>
<p style="margin:10px 0 0;font-size:14px;line-height:1.6;color:#555555;">Se agendó una reunión en tu grupo. Revisa los detalles y guarda el enlace para conectarte.</p>
<table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="margin-top:18px;border-top:1px solid #E5E0D8;">
` + details.String() + `
</table>
` + descriptionBlock + linkBlock + `
</td></tr>
<tr><td style="background:#1A1A1A;border-top:4px solid #FFCC00;padding:20px 24px;">
<div style="font-size:14px;font-weight:900;letter-spacing:2px;text-transform:uppercase;color:#FFCC00;">Sigma Academy</div>
<div style="font-size:12px;line-height:1.6;color:#F5F0E8;margin-top:6px;">Organiza tus grupos, tareas y reuniones en un solo lugar.</div>
<a href="https://ti3-brojas.dev.censei.cl" style="display:inline-block;margin-top:10px;font-size:12px;font-weight:bold;color:#FFCC00;text-decoration:none;">ti3-brojas.dev.censei.cl</a>
<div style="font-size:10px;line-height:1.5;color:#A6A6A6;margin-top:12px;">Este correo se generó automáticamente. No respondas a este mensaje.</div>
</td></tr>
</table>
</td></tr>
</table>
</body>
</html>`

	return MeetingMessage{Subject: subject, TextBody: text.String(), HTMLBody: body}
}

func cleanLine(s string) string {
	return strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(s))
}

func detailRow(label, value string) string {
	return `<tr>` +
		`<td style="padding:10px 12px 10px 0;border-bottom:1px solid #E5E0D8;font-size:11px;font-weight:bold;letter-spacing:1px;text-transform:uppercase;color:#555555;vertical-align:top;width:90px;">` + label + `</td>` +
		`<td style="padding:10px 0;border-bottom:1px solid #E5E0D8;font-size:14px;line-height:1.5;color:#1A1A1A;">` + value + `</td>` +
		`</tr>`
}

func safeHref(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	if u.Host == "" {
		return ""
	}
	return u.String()
}

// SendMeetingInvite envía el aviso a todos los destinatarios en una sesión.
func (m *MeetingMailer) SendMeetingInvite(ctx context.Context, to []string, groupName, title string, scheduled time.Time, hasSchedule bool, description, link string) error {
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
	msg := BuildMeetingMessage(groupName, title, scheduled, hasSchedule, description, link)

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
	boundary := fmt.Sprintf("sigma-academy-%d", time.Now().UnixNano())
	raw := "From: " + (&mail.Address{Name: "Sigma Academy", Address: m.Username}).String() + "\r\n" +
		"To: " + strings.Join(addrs, ", ") + "\r\n" +
		"Subject: " + mime.QEncoding.Encode("UTF-8", msg.Subject) + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n\r\n" +
		"--" + boundary + "\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"Content-Transfer-Encoding: 8bit\r\n\r\n" +
		msg.TextBody + "\r\n" +
		"--" + boundary + "\r\n" +
		"Content-Type: text/html; charset=UTF-8\r\n" +
		"Content-Transfer-Encoding: 8bit\r\n\r\n" +
		msg.HTMLBody + "\r\n" +
		"--" + boundary + "--\r\n"
	if _, err := fmt.Fprint(writer, raw); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
