package service

import (
	"context"
	"time"
)

// passwordResetCodeValidity es la vigencia del código de recuperación. Debe
// coincidir con el intervalo de PasswordResetRepository.Issue
// (now() + interval '15 minutes'); solo se usa para el texto del correo.
const passwordResetCodeValidity = 15 * time.Minute

// SendResetCode envía el código de recuperación de contraseña
// (POST /auth/forgot-password) mediante la configuración SMTP común, con
// versión HTML (buildResetEmail) y de texto plano.
func (m *SMTPMailer) SendResetCode(ctx context.Context, recipient, code string) error {
	msg := buildResetEmail(code, passwordResetCodeValidity)
	return m.sendMessage(ctx, recipient, msg)
}
