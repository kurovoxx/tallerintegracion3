package service

import (
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Los fragmentos de layout deben ser idénticos a los del aviso de reuniones
// de Social (los servicios no se importan entre sí; ver AGENTS.md). Si alguien
// cambia el diseño de un lado, este test lo detecta. Se omite si el archivo de
// Social no está disponible (p. ej. dentro del contexto Docker de Auth).
func TestEmailLayout_CoincideConCorreoDeReuniones(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "social", "internal", "mail", "mailer.go"))
	if err != nil {
		t.Skipf("correo de reuniones no disponible: %v", err)
	}
	social := strings.ReplaceAll(string(src), "\r\n", "\n")
	for name, frag := range map[string]string{
		"body":       emailBodyStyle,
		"exterior":   emailOuterTable,
		"tarjeta":    emailCardTable,
		"encabezado": emailHeaderCell,
		"marca":      emailHeaderBrand,
		"título":     emailHeaderTitle,
		"contenido":  emailContentCell,
		"pie":        emailFooter,
	} {
		if !strings.Contains(social, frag) {
			t.Errorf("el fragmento %q ya no coincide con social/internal/mail/mailer.go:\n%s", name, frag)
		}
	}
}

func TestVerificationEmail_CodigoDestacadoYTextoPlano(t *testing.T) {
	token, _, _ := generateVerificationToken()
	msg := buildVerificationEmail(VerificationEmail{Recipient: "a@example.com", Token: token, ExpiresIn: 24 * time.Hour})

	if msg.Subject != "Confirma tu correo en Sigma Academy" {
		t.Fatalf("asunto: %q", msg.Subject)
	}
	// Estructura común.
	for _, want := range []string{
		"<!DOCTYPE html>", `<html lang="es">`, `<meta charset="UTF-8">`, emailHeaderBrand,
		">Confirma tu cuenta</div>", emailFooter, "Código de confirmación", "Válido por", "24 horas",
		"«Confirmar correo»", "Una sola vez",
	} {
		if !strings.Contains(msg.HTML, want) {
			t.Errorf("falta %q en el HTML", want)
		}
	}
	// El código aparece una sola vez, dentro del bloque destacado.
	if strings.Count(msg.HTML, token) != 1 {
		t.Fatalf("el código debe aparecer una vez en el HTML: %d", strings.Count(msg.HTML, token))
	}
	block := emailCodeBlock("Código de confirmación", token, 18, 1)
	if !strings.Contains(msg.HTML, block) || !strings.Contains(block, "border:2px dashed #1A1A1A") ||
		!strings.Contains(block, "monospace") || !strings.Contains(block, ">"+token+"</div>") {
		t.Fatal("el código debe ir solo, en el bloque monoespaciado con borde discontinuo")
	}
	// El preheader (vista previa de la bandeja) nunca lleva el código.
	pre := msg.HTML[strings.Index(msg.HTML, "<div style=\"display:none"):]
	pre = pre[:strings.Index(pre, "</div>")]
	if strings.Contains(pre, token) {
		t.Fatal("el preheader no debe incluir el código")
	}
	// Sin EMAIL_VERIFICATION_URL no hay botón ni enlaces salvo el del pie.
	if strings.Contains(msg.HTML, "Confirmar mi cuenta") || strings.Count(msg.HTML, "href=") != 1 {
		t.Fatal("sin enlace configurado no debe haber botón")
	}
	// Texto plano: código en su propia línea bajo una etiqueta clara.
	for _, want := range []string{"CÓDIGO DE CONFIRMACIÓN:\r\n" + token + "\r\n", "24 horas", "solo puede usarse una vez", "Confirmar correo"} {
		if !strings.Contains(msg.Text, want) {
			t.Errorf("falta %q en el texto plano:\n%s", want, msg.Text)
		}
	}
}

func TestVerificationEmail_ConEnlaceSoloHTTP(t *testing.T) {
	token, _, _ := generateVerificationToken()
	link := "https://ti3.example.cl/verificar?a=1&token=" + token
	msg := buildVerificationEmail(VerificationEmail{Token: token, Link: link, ExpiresIn: 2 * time.Hour})
	if !strings.Contains(msg.HTML, `href="https://ti3.example.cl/verificar?a=1&amp;token=`+token+`"`) ||
		!strings.Contains(msg.HTML, "Confirmar mi cuenta") || !strings.Contains(msg.HTML, "2 horas") {
		t.Fatalf("debe incluir el botón con el enlace escapado:\n%s", msg.HTML)
	}
	if !strings.Contains(msg.Text, link) || !strings.Contains(msg.Text, "Si el enlace no funciona") {
		t.Fatal("el texto plano debe incluir el enlace")
	}
	// Un esquema no http(s) nunca se convierte en enlace.
	bad := buildVerificationEmail(VerificationEmail{Token: token, Link: "javascript:alert(1)", ExpiresIn: time.Hour})
	if strings.Contains(bad.HTML, "javascript:") || strings.Contains(bad.Text, "javascript:") || strings.Contains(bad.HTML, "Confirmar mi cuenta") {
		t.Fatal("un enlace no http(s) debe descartarse")
	}
}

func TestResetEmail_CodigoDestacadoYTextoPlano(t *testing.T) {
	msg := buildResetEmail("01234567", passwordResetCodeValidity)
	if msg.Subject != "Recupera tu acceso a Sigma Academy" {
		t.Fatalf("asunto: %q", msg.Subject)
	}
	for _, want := range []string{
		">Recupera tu contraseña</div>", emailHeaderBrand, emailFooter, "Código de recuperación",
		"15 minutos", "«Recuperar contraseña»", "Nadie de Sigma Academy te pedirá este código",
	} {
		if !strings.Contains(msg.HTML, want) {
			t.Errorf("falta %q en el HTML", want)
		}
	}
	// 8 dígitos sin espacios reales (el espaciado es letter-spacing).
	if strings.Count(msg.HTML, "01234567") != 1 || !strings.Contains(msg.HTML, emailCodeBlock("Código de recuperación", "01234567", 34, 8)) {
		t.Fatal("el código debe aparecer una vez, completo y en el bloque destacado")
	}
	if !strings.Contains(msg.Text, "Tu código para restablecer la contraseña es: 01234567\r\n") || !strings.Contains(msg.Text, "15 minutos") {
		t.Fatalf("texto plano incompleto:\n%s", msg.Text)
	}
	if strings.Contains(msg.HTML[:strings.Index(msg.HTML, "<table")], "01234567") {
		t.Fatal("el código no debe ir en el título ni en el preheader")
	}
}

func TestEmailTemplates_EscapanHTML(t *testing.T) {
	msg := buildResetEmail(`<b>"x"&`, time.Minute)
	if strings.Contains(msg.HTML, "<b>") || !strings.Contains(msg.HTML, "&lt;b&gt;&#34;x&#34;&amp;") {
		t.Fatal("el código debe escaparse en el HTML")
	}
}

// TestEmailPreview_WriteFiles genera vistas previas con datos ficticios para
// revisarlas en un navegador. Solo se ejecuta con EMAIL_PREVIEW_DIR definido.
func TestEmailPreview_WriteFiles(t *testing.T) {
	dir := os.Getenv("EMAIL_PREVIEW_DIR")
	if dir == "" {
		t.Skip("EMAIL_PREVIEW_DIR no definida")
	}
	files := map[string]emailContent{
		"confirmacion.html":            buildVerificationEmail(VerificationEmail{Token: "Xq3vN8kL2pR7sT1wY5zA9bC4dE6fG0hJ-mK_nPqRsTu", ExpiresIn: 24 * time.Hour}),
		"confirmacion_con_enlace.html": buildVerificationEmail(VerificationEmail{Token: "Xq3vN8kL2pR7sT1wY5zA9bC4dE6fG0hJ-mK_nPqRsTu", Link: "https://ti3-brojas.dev.censei.cl/verificar?token=Xq3vN8kL2pR7sT1wY5zA9bC4dE6fG0hJ-mK_nPqRsTu", ExpiresIn: 24 * time.Hour}),
		"recuperacion.html":            buildResetEmail("48201736", passwordResetCodeValidity),
	}
	for name, msg := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(msg.HTML), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, strings.TrimSuffix(name, ".html")+".txt"), []byte(msg.Text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// Regresión: las instrucciones se partían en <strong>primeras palabras</strong>
// + resto normal. Ahora cada oración tiene un único formato.
func TestEmailTemplates_InstruccionesConFormatoUnico(t *testing.T) {
	token, _, _ := generateVerificationToken()
	cases := map[string]struct {
		html      string
		sentences []string
	}{
		"confirmación": {buildVerificationEmail(VerificationEmail{Token: token, ExpiresIn: 24 * time.Hour}).HTML,
			[]string{"Copia este código y pégalo en la pantalla «Confirmar correo» de la aplicación."}},
		"confirmación con enlace": {buildVerificationEmail(VerificationEmail{Token: token, Link: "https://x.example/v?token=" + token, ExpiresIn: time.Hour}).HTML,
			[]string{"Opción 1: pulsa el botón para confirmar tu cuenta.", "Opción 2: copia este código y pégalo en la pantalla «Confirmar correo» de la aplicación."}},
		"recuperación": {buildResetEmail("12345678", passwordResetCodeValidity).HTML,
			[]string{"Ingresa este código en la pantalla «Recuperar contraseña» de la aplicación, junto con tu nueva contraseña."}},
	}
	for name, c := range cases {
		low := strings.ToLower(c.html)
		if strings.Contains(low, "<strong") || strings.Contains(low, "<b>") || strings.Contains(low, "<b ") {
			t.Errorf("%s: no debe haber negrita parcial con <strong>/<b>", name)
		}
		for _, s := range c.sentences {
			want := `font-weight:bold;line-height:1.6;color:#1A1A1A;">` + html.EscapeString(s) + `</p>`
			if !strings.Contains(c.html, want) {
				t.Errorf("%s: la instrucción debe ir completa en un párrafo en negrita: %q", name, s)
			}
		}
	}
}
