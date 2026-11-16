package service

import "context"

// SendResetCode envía el código de recuperación de contraseña
// (POST /auth/forgot-password) mediante la configuración SMTP común.
func (m *SMTPMailer) SendResetCode(ctx context.Context, recipient, code string) error {
	return m.send(ctx, recipient, "Recupera tu acceso a Sigma Academy",
		"Tu código para restablecer la contraseña es: "+code+"\r\n\r\n"+
			"Ingresa este código en Sigma Academy. Es válido durante 15 minutos y solo puede usarse una vez.\r\n"+
			"Si no solicitaste este cambio, ignora este correo. Tu contraseña seguirá siendo la misma.\r\n")
}
