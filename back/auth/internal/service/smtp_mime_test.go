package service

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
	"time"
)

// sentMail es un correo recibido por el servidor SMTP falso, ya decodificado.
type sentMail struct {
	header mail.Header
	text   string
	html   string
}

// decodeSentMail interpreta el mensaje tal como llegó por SMTP: cabeceras,
// multipart/alternative y cada parte decodificada según su
// Content-Transfer-Encoding (multipart.Reader decodifica quoted-printable).
func decodeSentMail(t *testing.T, data string) sentMail {
	t.Helper()
	msg, err := mail.ReadMessage(strings.NewReader(data))
	if err != nil {
		t.Fatalf("mensaje no parseable: %v", err)
	}
	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/alternative" {
		t.Fatalf("se esperaba multipart/alternative: %q %v", mediaType, err)
	}
	out := sentMail{header: msg.Header}
	mr := multipart.NewReader(msg.Body, params["boundary"])
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("parte MIME inválida: %v", err)
		}
		body, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("no se pudo decodificar la parte: %v", err)
		}
		decoded := strings.ReplaceAll(string(body), "\r\n", "\n")
		switch ct, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type")); ct {
		case "text/plain":
			out.text = decoded
		case "text/html":
			out.html = decoded
		}
	}
	if out.text == "" || out.html == "" {
		t.Fatal("deben llegar la parte de texto plano y la HTML")
	}
	return out
}

// assertMIMELines exige líneas cortas en el mensaje realmente enviado:
// ninguna supera 998 caracteres (RFC 5322 §2.1.1) y las del cuerpo
// quoted-printable no superan 76 (RFC 2045 §6.7). Todo debe ser ASCII.
func assertMIMELines(t *testing.T, data string) {
	t.Helper()
	// El servidor falso lee DATA con textproto.DotReader, que entrega LF:
	// se normaliza y se mide cada línea sin su fin de línea.
	data = strings.ReplaceAll(data, "\r\n", "\n")
	headerEnd := strings.Index(data, "\n\n")
	if headerEnd < 0 {
		t.Fatal("mensaje sin separación de cabeceras")
	}
	for i, line := range strings.Split(data, "\n") {
		if len(line) > 998 {
			t.Fatalf("línea %d de %d caracteres supera el límite de 998", i+1, len(line))
		}
	}
	for i, line := range strings.Split(data[headerEnd+2:], "\n") {
		if len(line) > 76 {
			t.Fatalf("línea %d del cuerpo de %d caracteres supera 76 (quoted-printable)", i+1, len(line))
		}
	}
	for i := 0; i < len(data); i++ {
		if data[i] > 127 {
			t.Fatalf("byte no ASCII en la posición %d: el cuerpo debe ir codificado", i)
		}
	}
	if n := strings.Count(data, "Content-Transfer-Encoding: quoted-printable"); n != 2 {
		t.Fatalf("texto y HTML deben declarar quoted-printable: %d", n)
	}
}

func TestSMTPMailer_VerificationMIME_LineasCortasYContenidoIntacto(t *testing.T) {
	for _, withLink := range []bool{false, true} {
		srv, m := startFakeSMTP(t, nil)
		token, _, _ := generateVerificationToken()
		email := VerificationEmail{Recipient: "student@example.com", Token: token, ExpiresIn: 24 * time.Hour}
		if withLink {
			email.Link = "https://ti3.example.cl/verificar?origen=correo&token=" + token
		}
		if err := m.SendVerificationEmail(context.Background(), email); err != nil {
			t.Fatalf("send: %v", err)
		}
		_, _, _, _, data := srv.snapshot()
		assertMIMELines(t, data)

		got := decodeSentMail(t, data)
		want := buildVerificationEmail(email)
		if got.html != want.HTML {
			t.Fatal("el HTML decodificado debe ser idéntico al de la plantilla")
		}
		if got.text != strings.ReplaceAll(want.Text, "\r\n", "\n") {
			t.Fatalf("el texto plano decodificado debe ser idéntico:\n%s", got.text)
		}
		// El código se puede copiar completo: una aparición, intacto y sin
		// saltos suaves en medio una vez decodificado.
		if strings.Count(got.html, ">"+token+"</div>") != 1 {
			t.Fatal("el código debe aparecer una vez, completo, en el bloque destacado")
		}
		if !strings.Contains(got.text, "\n"+token+"\n") {
			t.Fatal("el texto plano debe traer el código en su propia línea")
		}
		for _, es := range []string{"CÓDIGO DE CONFIRMACIÓN", "«Confirmar correo»", "¡Hola!"} {
			if !strings.Contains(got.text, es) {
				t.Fatalf("caracteres en español alterados: falta %q", es)
			}
		}
		if withLink && !strings.Contains(got.html, `href="https://ti3.example.cl/verificar?origen=correo&amp;token=`+token+`"`) {
			t.Fatal("el enlace del botón debe llegar intacto")
		}
	}
}

func TestSMTPMailer_ResetMIME_LineasCortasYContenidoIntacto(t *testing.T) {
	srv, m := startFakeSMTP(t, nil)
	if err := m.SendResetCode(context.Background(), "student@example.com", "04812736"); err != nil {
		t.Fatalf("send: %v", err)
	}
	_, _, _, _, data := srv.snapshot()
	assertMIMELines(t, data)

	got := decodeSentMail(t, data)
	want := buildResetEmail("04812736", passwordResetCodeValidity)
	if got.html != want.HTML || got.text != strings.ReplaceAll(want.Text, "\r\n", "\n") {
		t.Fatal("HTML y texto decodificados deben ser idénticos a la plantilla")
	}
	if strings.Count(got.html, ">04812736</div>") != 1 || !strings.Contains(got.text, "es: 04812736\n") {
		t.Fatal("el código de 8 dígitos debe llegar completo y sin espacios")
	}
	if !strings.Contains(got.html, "Recupera tu contraseña") || !strings.Contains(got.text, "contraseña") {
		t.Fatal("caracteres en español alterados")
	}
}
