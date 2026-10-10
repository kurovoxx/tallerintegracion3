package service

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

// Gmail SMTP with mandatory STARTTLS; credentials only live on the server.
type ResetMailer struct{ Host, Port, Username, Password string }

func (m *ResetMailer) Ready() bool {
	return m.Host != "" && m.Port != "" && m.Username != "" && m.Password != ""
}

func (m *ResetMailer) SendResetCode(ctx context.Context, recipient, code string) error {
	if !m.Ready() {
		return fmt.Errorf("SMTP not configured")
	}
	if strings.ContainsAny(recipient+m.Username, "\r\n") {
		return fmt.Errorf("invalid address")
	}
	if _, err := mail.ParseAddress(recipient); err != nil {
		return err
	}
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
	if err := client.Rcpt(recipient); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	message := "From: " + (&mail.Address{Name: "Sigma Academy", Address: m.Username}).String() + "\r\n" +
		"To: " + recipient + "\r\nSubject: Recupera tu acceso a Sigma Academy\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" +
		"Tu código para restablecer la contraseña es: " + code + "\r\n\r\n" +
		"Ingresa este código en Sigma Academy. Es válido durante 15 minutos y solo puede usarse una vez.\r\n" +
		"Si no solicitaste este cambio, ignora este correo. Tu contraseña seguirá siendo la misma.\r\n"
	if _, err := fmt.Fprint(writer, message); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
