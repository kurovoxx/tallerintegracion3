package service

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

const (
	SMTPTLSModeStartTLS = "starttls" // puerto 587: STARTTLS obligatorio
	SMTPTLSModeImplicit = "tls"      // puerto 465: TLS desde el primer byte

	defaultSMTPFromName = "Sigma Academy"
	defaultSMTPTimeout  = 10 * time.Second
)

// SMTPMailer envía correos transaccionales de texto plano (recuperación de
// contraseña y confirmación de correo). Siempre cifra la conexión (STARTTLS o
// TLS implícito, TLS 1.2+) y autentica con PLAIN; las credenciales solo viven
// en el servidor (variables de entorno) y nunca aparecen en errores ni logs.
type SMTPMailer struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string        // remitente; vacío = Username
	FromName string        // nombre visible; vacío = "Sigma Academy"
	TLSMode  string        // "starttls" (defecto) o "tls"
	Timeout  time.Duration // conexión + conversación completa; 0 = 10 s

	tlsConfig *tls.Config // solo pruebas: CA del servidor SMTP falso
}

// SMTPError describe en qué etapa falló un envío sin arrastrar el texto de
// la respuesta del servidor, que podría repetir direcciones o datos del
// mensaje. Code es el código SMTP (p. ej. 535) cuando el servidor respondió.
type SMTPError struct {
	Stage string
	Code  int
	err   error
}

func (e *SMTPError) Error() string {
	if e.Code > 0 {
		return fmt.Sprintf("smtp %s failed (code %d)", e.Stage, e.Code)
	}
	return "smtp " + e.Stage + " failed"
}

func (e *SMTPError) Unwrap() error { return e.err }

func smtpFailure(stage string, err error) error {
	se := &SMTPError{Stage: stage, err: err}
	var tp *textproto.Error
	if errors.As(err, &tp) {
		se.Code = tp.Code
	}
	return se
}

// Ready indica si hay configuración completa y válida para enviar.
func (m *SMTPMailer) Ready() bool {
	return m != nil && m.Host != "" && m.Port != "" && m.Username != "" && m.Password != "" && m.Validate() == nil
}

// Validate comprueba el formato de la configuración. Los mensajes nombran el
// campo, nunca su valor, para poder registrarlos sin exponer nada.
func (m *SMTPMailer) Validate() error {
	if strings.TrimSpace(m.Host) == "" || strings.ContainsAny(m.Host, " \r\n/") {
		return errors.New("SMTP_HOST inválido")
	}
	port, err := strconv.Atoi(m.Port)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("SMTP_PORT inválido")
	}
	switch m.tlsMode() {
	case SMTPTLSModeStartTLS, SMTPTLSModeImplicit:
	default:
		return errors.New("SMTP_TLS_MODE debe ser starttls o tls")
	}
	if strings.ContainsAny(m.Username, "\r\n") {
		return errors.New("SMTP_USERNAME inválido")
	}
	if _, err := parseBareAddress(m.from()); err != nil {
		return errors.New("SMTP_FROM inválido")
	}
	if strings.ContainsAny(m.FromName, "\r\n") {
		return errors.New("SMTP_FROM_NAME inválido")
	}
	return nil
}

func (m *SMTPMailer) tlsMode() string {
	mode := strings.ToLower(strings.TrimSpace(m.TLSMode))
	if mode == "" {
		return SMTPTLSModeStartTLS
	}
	return mode
}

func (m *SMTPMailer) from() string {
	if m.From != "" {
		return m.From
	}
	return m.Username
}

func (m *SMTPMailer) fromName() string {
	if m.FromName != "" {
		return m.FromName
	}
	return defaultSMTPFromName
}

// parseBareAddress acepta solo una dirección simple (sin nombre visible ni
// saltos de línea) para usarla en MAIL FROM/RCPT TO y cabeceras.
func parseBareAddress(address string) (string, error) {
	if address == "" || strings.ContainsAny(address, "\r\n<>") {
		return "", errors.New("invalid address")
	}
	parsed, err := mail.ParseAddress(address)
	if err != nil || parsed.Name != "" || parsed.Address != address {
		return "", errors.New("invalid address")
	}
	return parsed.Address, nil
}

// send entrega un correo de texto plano UTF-8 a un único destinatario.
func (m *SMTPMailer) send(ctx context.Context, recipient, subject, body string) error {
	if !m.Ready() {
		return &SMTPError{Stage: "config"}
	}
	to, err := parseBareAddress(recipient)
	if err != nil {
		return &SMTPError{Stage: "address", err: err}
	}
	from := m.from()
	timeout := m.Timeout
	if timeout <= 0 {
		timeout = defaultSMTPTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(m.Host, m.Port))
	if err != nil {
		return smtpFailure("dial", err)
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return smtpFailure("dial", err)
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()

	tlsCfg := &tls.Config{ServerName: m.Host, MinVersion: tls.VersionTLS12}
	if m.tlsConfig != nil {
		tlsCfg = m.tlsConfig.Clone()
	}
	if m.tlsMode() == SMTPTLSModeImplicit {
		tlsConn := tls.Client(conn, tlsCfg)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return smtpFailure("tls", err)
		}
		conn = tlsConn
	}
	client, err := smtp.NewClient(conn, m.Host)
	if err != nil {
		return smtpFailure("greeting", err)
	}
	defer client.Close()
	if m.tlsMode() == SMTPTLSModeStartTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			// Nunca degradar a texto plano.
			return &SMTPError{Stage: "starttls"}
		}
		if err := client.StartTLS(tlsCfg); err != nil {
			return smtpFailure("starttls", err)
		}
	}
	if err := client.Auth(smtp.PlainAuth("", m.Username, m.Password, m.Host)); err != nil {
		return smtpFailure("auth", err)
	}
	if err := client.Mail(from); err != nil {
		return smtpFailure("mail_from", err)
	}
	if err := client.Rcpt(to); err != nil {
		return smtpFailure("rcpt", err)
	}
	writer, err := client.Data()
	if err != nil {
		return smtpFailure("data", err)
	}
	message := "From: " + (&mail.Address{Name: m.fromName(), Address: from}).String() + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n" +
		"Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n" +
		"Content-Transfer-Encoding: 8bit\r\n\r\n" + body
	if _, err := fmt.Fprint(writer, message); err != nil {
		return smtpFailure("data", err)
	}
	if err := writer.Close(); err != nil {
		return smtpFailure("data", err)
	}
	// El servidor ya aceptó el mensaje (250 tras DATA): un fallo de QUIT no
	// significa que el correo se haya perdido, así que no se informa como error.
	_ = client.Quit()
	return nil
}
