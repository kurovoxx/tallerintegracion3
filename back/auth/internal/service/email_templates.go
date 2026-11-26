package service

import (
	"html"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Plantillas HTML de los correos transaccionales de Auth (confirmación de
// cuenta y recuperación de contraseña).
//
// La identidad visual es la del aviso de reuniones de Social
// (back/social/internal/mail/mailer.go, BuildMeetingMessage): fondo beige,
// tarjeta blanca de 600 px con borde negro de 2 px, encabezado amarillo con
// «Sigma Academy», etiquetas en mayúsculas, separadores #E5E0D8 y pie negro.
// Auth y Social son módulos Go independientes (cada uno se construye con su
// propio contexto Docker), por lo que no pueden importar el mismo paquete; los
// fragmentos comunes se copian tal cual y TestEmailLayout_CoincideConCorreoDeReuniones
// comprueba que sigan siendo idénticos a los de Social.
//
// Reglas de compatibilidad (Gmail, Outlook, móviles): solo tablas y estilos
// inline, sin CSS externo ni <style>, sin JavaScript, sin imágenes y con
// fuentes del sistema. El HTML va siempre acompañado de una versión de texto
// plano (multipart/alternative).

const (
	// Fragmentos idénticos a los del correo de reuniones (ver test).
	emailBodyStyle   = `margin:0;padding:0;background:#F5F0E8;font-family:Arial,Helvetica,sans-serif;color:#1A1A1A;`
	emailOuterTable  = `<table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="background:#F5F0E8;padding:24px 12px;">`
	emailCardTable   = `<table role="presentation" width="600" cellspacing="0" cellpadding="0" border="0" style="width:100%;max-width:600px;background:#FFFFFF;border:2px solid #1A1A1A;">`
	emailHeaderCell  = `<tr><td style="background:#FFCC00;border-bottom:2px solid #1A1A1A;padding:18px 24px;">`
	emailHeaderBrand = `<div style="font-size:11px;font-weight:bold;letter-spacing:2px;text-transform:uppercase;color:#1A1A1A;">Sigma Academy</div>`
	emailHeaderTitle = `<div style="font-size:24px;font-weight:900;line-height:1.2;margin-top:4px;color:#1A1A1A;">`
	emailContentCell = `<tr><td style="padding:24px;">`
	emailFooter      = `<tr><td style="background:#1A1A1A;border-top:4px solid #FFCC00;padding:20px 24px;">
<div style="font-size:14px;font-weight:900;letter-spacing:2px;text-transform:uppercase;color:#FFCC00;">Sigma Academy</div>
<div style="font-size:12px;line-height:1.6;color:#F5F0E8;margin-top:6px;">Organiza tus grupos, tareas y reuniones en un solo lugar.</div>
<a href="https://ti3-brojas.dev.censei.cl" style="display:inline-block;margin-top:10px;font-size:12px;font-weight:bold;color:#FFCC00;text-decoration:none;">ti3-brojas.dev.censei.cl</a>
<div style="font-size:10px;line-height:1.5;color:#A6A6A6;margin-top:12px;">Este correo se generó automáticamente. No respondas a este mensaje.</div>
</td></tr>`

	emailSiteURL = "https://ti3-brojas.dev.censei.cl"
)

// emailContent es un correo ya compuesto: asunto, texto plano y HTML.
type emailContent struct {
	Subject string
	Text    string
	HTML    string
}

// emailLayout envuelve el contenido en la estructura común. headerTitle y
// preheader son texto plano (se escapan aquí); contentHTML ya viene escapado.
// El preheader es el resumen que muestran las bandejas de entrada; nunca
// lleva el código.
func emailLayout(headerTitle, preheader, contentHTML string) string {
	esc := html.EscapeString(headerTitle)
	return `<!DOCTYPE html>
<html lang="es">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light only">
<title>` + esc + `</title>
</head>
<body style="` + emailBodyStyle + `">
<div style="display:none;max-height:0;overflow:hidden;opacity:0;color:#F5F0E8;font-size:1px;line-height:1px;">` + html.EscapeString(preheader) + `</div>
` + emailOuterTable + `
<tr><td align="center">
` + emailCardTable + `
` + emailHeaderCell + `
` + emailHeaderBrand + `
` + emailHeaderTitle + esc + `</div>
</td></tr>
` + emailContentCell + `
` + contentHTML + `
</td></tr>
` + emailFooter + `
</table>
</td></tr>
</table>
</body>
</html>`
}

func emailHeading(text string) string {
	return `<div style="font-size:20px;font-weight:900;line-height:1.3;color:#1A1A1A;">` + html.EscapeString(text) + `</div>`
}

func emailParagraph(text string) string {
	return `<p style="margin:10px 0 0;font-size:14px;line-height:1.6;color:#555555;">` + html.EscapeString(text) + `</p>`
}

// emailInstruction es el paso que debe seguir el usuario. La oración completa
// va en negrita con un único formato (font-weight en el párrafo, sin <strong>
// parcial): partirla dejaba las primeras palabras en negrita y el resto no.
func emailInstruction(sentence string) string {
	return `<p style="margin:18px 0 0;font-size:15px;font-weight:bold;line-height:1.6;color:#1A1A1A;">` +
		html.EscapeString(sentence) + `</p>`
}

// emailCodeBlock destaca el código que hay que copiar. Va en su propia celda,
// con borde discontinuo, fondo beige, tipografía monoespaciada y gran tamaño,
// para que no se confunda con ningún otro dato. El espaciado entre caracteres
// es solo visual (letter-spacing): al copiar no se agregan espacios. Si el
// cliente ignora estos estilos, el código sigue en una línea aparte, sin
// adornos, justo debajo de su etiqueta.
func emailCodeBlock(label, code string, fontSize, letterSpacing int) string {
	return `<table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="margin-top:14px;">` +
		`<tr><td style="font-size:11px;font-weight:bold;letter-spacing:1px;text-transform:uppercase;color:#555555;padding-bottom:6px;">` + html.EscapeString(label) + `</td></tr>` +
		`<tr><td align="center" style="background:#F5F0E8;border:2px dashed #1A1A1A;padding:18px 12px;">` +
		`<div style="font-family:'Courier New',Courier,monospace;font-size:` + strconv.Itoa(fontSize) + `px;font-weight:bold;line-height:1.35;letter-spacing:` + strconv.Itoa(letterSpacing) + `px;color:#1A1A1A;word-break:break-all;-webkit-user-select:all;user-select:all;">` +
		html.EscapeString(code) + `</div>` +
		`</td></tr>` +
		`</table>`
}

// emailDetails es la tabla de etiquetas/valores (mismo estilo que detailRow
// del correo de reuniones).
func emailDetails(rows [][2]string) string {
	var b strings.Builder
	b.WriteString(`<table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="margin-top:22px;border-top:1px solid #E5E0D8;">`)
	for _, r := range rows {
		b.WriteString(`<tr>` +
			`<td style="padding:10px 12px 10px 0;border-bottom:1px solid #E5E0D8;font-size:11px;font-weight:bold;letter-spacing:1px;text-transform:uppercase;color:#555555;vertical-align:top;width:90px;">` + html.EscapeString(r[0]) + `</td>` +
			`<td style="padding:10px 0;border-bottom:1px solid #E5E0D8;font-size:14px;line-height:1.5;color:#1A1A1A;">` + html.EscapeString(r[1]) + `</td>` +
			`</tr>`)
	}
	b.WriteString(`</table>`)
	return b.String()
}

// emailNotice es una nota lateral (mismo bloque que la descripción de la
// reunión, con borde amarillo).
func emailNotice(text string) string {
	return `<div style="margin-top:20px;padding:14px 16px;background:#F5F0E8;border-left:4px solid #FFCC00;font-size:13px;line-height:1.6;color:#1A1A1A;">` +
		html.EscapeString(text) + `</div>`
}

// emailButton es el botón azul del correo de reuniones. Solo acepta http(s).
func emailButton(label, link string) string {
	href := safeEmailHref(link)
	if href == "" {
		return ""
	}
	return `<div style="margin-top:20px;">` +
		`<a href="` + html.EscapeString(href) + `" style="display:inline-block;background:#0055FF;color:#FFFFFF;border:2px solid #1A1A1A;padding:11px 18px;font-size:14px;font-weight:bold;text-decoration:none;">` + html.EscapeString(label) + `</a>` +
		`<div style="margin-top:10px;font-size:12px;line-height:1.5;color:#555555;word-break:break-all;">` + html.EscapeString(href) + `</div>` +
		`</div>`
}

func safeEmailHref(link string) string {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return u.String()
}

const textFooter = "\r\n--\r\nSigma Academy\r\n" + emailSiteURL + "\r\nEste correo se generó automáticamente. No respondas a este mensaje.\r\n"

// buildVerificationEmail compone el correo de confirmación de cuenta. Siempre
// incluye el código (para pegarlo en «Confirmar correo») y, si
// EMAIL_VERIFICATION_URL está configurada, también el botón con el enlace.
func buildVerificationEmail(e VerificationEmail) emailContent {
	validity := humanDuration(e.ExpiresIn)
	link := safeEmailHref(e.Link)

	var text strings.Builder
	text.WriteString("¡Hola! Gracias por registrarte en Sigma Academy.\r\n\r\n")
	text.WriteString("Recibiste este correo porque se creó una cuenta en Sigma Academy con esta dirección. ")
	text.WriteString("Para activarla, confirma que el correo es tuyo.\r\n\r\n")
	if link != "" {
		text.WriteString("Para confirmar tu correo, abre este enlace:\r\n" + link + "\r\n\r\n")
		text.WriteString("Si el enlace no funciona, usa este código de confirmación:\r\n")
	} else {
		text.WriteString("Para confirmar tu correo, ingresa este código en Sigma Academy:\r\n")
	}
	text.WriteString("\r\nCÓDIGO DE CONFIRMACIÓN:\r\n")
	text.WriteString(e.Token + "\r\n\r\n")
	text.WriteString("Cópialo completo y pégalo en la pantalla «Confirmar correo» de la aplicación.\r\n\r\n")
	text.WriteString("Es válido durante " + validity + " y solo puede usarse una vez. ")
	text.WriteString("Si pides otro correo, este deja de servir.\r\n")
	text.WriteString("Si no creaste una cuenta en Sigma Academy, ignora este mensaje.\r\n")
	text.WriteString(textFooter)

	var c strings.Builder
	c.WriteString(emailHeading("¡Hola! Gracias por registrarte"))
	c.WriteString(emailParagraph("Recibiste este correo porque se creó una cuenta en Sigma Academy con esta dirección. Para activarla, confirma que el correo es tuyo."))
	if link != "" {
		c.WriteString(emailInstruction("Opción 1: pulsa el botón para confirmar tu cuenta."))
		c.WriteString(emailButton("Confirmar mi cuenta", link))
		c.WriteString(emailInstruction("Opción 2: copia este código y pégalo en la pantalla «Confirmar correo» de la aplicación."))
	} else {
		c.WriteString(emailInstruction("Copia este código y pégalo en la pantalla «Confirmar correo» de la aplicación."))
	}
	// 43 caracteres: 18 px monoespaciados caben en la tarjeta de 600 px y en
	// móvil el bloque corta la línea sin agregar caracteres.
	c.WriteString(emailCodeBlock("Código de confirmación", e.Token, 18, 1))
	c.WriteString(`<p style="margin:8px 0 0;font-size:12px;line-height:1.5;color:#555555;">Copia el código completo, sin espacios al principio ni al final.</p>`)
	c.WriteString(emailDetails([][2]string{
		{"Válido por", validity},
		{"Uso", "Una sola vez. Si pides otro correo, este código deja de servir."},
	}))
	c.WriteString(emailNotice("Si no creaste una cuenta en Sigma Academy, ignora este mensaje: no se activará nada."))

	return emailContent{
		Subject: "Confirma tu correo en Sigma Academy",
		Text:    text.String(),
		HTML:    emailLayout("Confirma tu cuenta", "Confirma tu correo para activar tu cuenta de Sigma Academy.", c.String()),
	}
}

// buildResetEmail compone el correo de recuperación de contraseña con el
// código de 8 dígitos que se ingresa en «Recuperar contraseña».
func buildResetEmail(code string, validFor time.Duration) emailContent {
	validity := humanDuration(validFor)

	var text strings.Builder
	text.WriteString("Hola,\r\n\r\n")
	text.WriteString("Recibimos una solicitud para restablecer la contraseña de tu cuenta de Sigma Academy.\r\n\r\n")
	text.WriteString("Tu código para restablecer la contraseña es: " + code + "\r\n\r\n")
	text.WriteString("Ingresa este código en Sigma Academy, en la pantalla «Recuperar contraseña», junto con tu nueva contraseña. ")
	text.WriteString("Es válido durante " + validity + " y solo puede usarse una vez.\r\n\r\n")
	text.WriteString("Nadie de Sigma Academy te pedirá este código. No lo compartas.\r\n")
	text.WriteString("Si no solicitaste este cambio, ignora este correo. Tu contraseña seguirá siendo la misma.\r\n")
	text.WriteString(textFooter)

	var c strings.Builder
	c.WriteString(emailHeading("Restablece tu contraseña"))
	c.WriteString(emailParagraph("Recibimos una solicitud para restablecer la contraseña de tu cuenta de Sigma Academy."))
	c.WriteString(emailInstruction("Ingresa este código en la pantalla «Recuperar contraseña» de la aplicación, junto con tu nueva contraseña."))
	c.WriteString(emailCodeBlock("Código de recuperación", code, 34, 8))
	c.WriteString(emailDetails([][2]string{
		{"Válido por", validity},
		{"Uso", "Una sola vez."},
	}))
	c.WriteString(emailNotice("Nadie de Sigma Academy te pedirá este código. Si no solicitaste este cambio, ignora este correo: tu contraseña seguirá siendo la misma."))

	return emailContent{
		Subject: "Recupera tu acceso a Sigma Academy",
		Text:    text.String(),
		HTML:    emailLayout("Recupera tu contraseña", "Usa el código de este correo para crear una nueva contraseña.", c.String()),
	}
}
