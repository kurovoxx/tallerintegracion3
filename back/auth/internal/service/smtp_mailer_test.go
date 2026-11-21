package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"math/big"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSMTPServer implementa lo justo de ESMTP para probar el cliente real:
// STARTTLS o TLS implícito, AUTH PLAIN, MAIL, RCPT, DATA y QUIT.
type fakeSMTPServer struct {
	t         *testing.T
	ln        net.Listener
	tlsConfig *tls.Config
	implicit  bool
	noTLS     bool // no anuncia STARTTLS
	authCode  int  // 0 = 235
	rcptCode  int  // 0 = 250
	hang      bool // acepta y no responde nunca

	mu       sync.Mutex
	authSeen []string // credenciales decodificadas recibidas en AUTH
	authTLS  []bool   // si AUTH llegó por un canal cifrado
	mailFrom string
	rcptTo   string
	data     string
}

func newTestTLS(t *testing.T) (server *tls.Config, client *tls.Config) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "fake-smtp"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:         true, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	server = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12}
	client = &tls.Config{RootCAs: pool, ServerName: "127.0.0.1", MinVersion: tls.VersionTLS12}
	return server, client
}

func startFakeSMTP(t *testing.T, configure func(*fakeSMTPServer)) (*fakeSMTPServer, *SMTPMailer) {
	t.Helper()
	serverTLS, clientTLS := newTestTLS(t)
	s := &fakeSMTPServer{t: t, tlsConfig: serverTLS}
	if configure != nil {
		configure(s)
	}
	var err error
	if s.implicit {
		s.ln, err = tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	} else {
		s.ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.ln.Close() })
	go s.serve()
	_, port, _ := net.SplitHostPort(s.ln.Addr().String())
	mode := SMTPTLSModeStartTLS
	if s.implicit {
		mode = SMTPTLSModeImplicit
	}
	m := &SMTPMailer{
		Host: "127.0.0.1", Port: port, Username: "sender@example.com", Password: "app-password-secret",
		TLSMode: mode, Timeout: 2 * time.Second, tlsConfig: clientTLS,
	}
	return s, m
}

func (s *fakeSMTPServer) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *fakeSMTPServer) handle(conn net.Conn) {
	defer conn.Close()
	if s.hang {
		time.Sleep(5 * time.Second)
		return
	}
	_, secure := conn.(*tls.Conn)
	tp := textproto.NewConn(conn)
	reply := func(format string, args ...any) { _ = tp.PrintfLine(format, args...) }
	reply("220 fake ESMTP")
	for {
		line, err := tp.ReadLine()
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.SplitN(line, " ", 2)[0])
		switch cmd {
		case "EHLO", "HELO":
			if !secure && !s.noTLS {
				reply("250-fake")
				reply("250-STARTTLS")
				reply("250 AUTH PLAIN")
			} else {
				reply("250-fake")
				reply("250 AUTH PLAIN")
			}
		case "STARTTLS":
			reply("220 ready")
			tlsConn := tls.Server(conn, s.tlsConfig)
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			conn, secure = tlsConn, true
			tp = textproto.NewConn(conn)
		case "AUTH":
			parts := strings.Fields(line)
			decoded, _ := base64.StdEncoding.DecodeString(parts[len(parts)-1])
			s.mu.Lock()
			s.authSeen = append(s.authSeen, string(decoded))
			s.authTLS = append(s.authTLS, secure)
			s.mu.Unlock()
			if s.authCode != 0 {
				reply("%d 5.7.8 Username and Password not accepted", s.authCode)
				continue
			}
			reply("235 ok")
		case "MAIL":
			s.mu.Lock()
			s.mailFrom = line
			s.mu.Unlock()
			reply("250 ok")
		case "RCPT":
			s.mu.Lock()
			s.rcptTo = line
			s.mu.Unlock()
			if s.rcptCode != 0 {
				reply("%d 5.1.1 mailbox unavailable", s.rcptCode)
				continue
			}
			reply("250 ok")
		case "DATA":
			reply("354 go")
			body, err := tp.ReadDotBytes()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.data = string(body)
			s.mu.Unlock()
			reply("250 queued")
		case "QUIT":
			reply("221 bye")
			return
		default:
			reply("502 unknown")
		}
	}
}

func (s *fakeSMTPServer) snapshot() (auth []string, authTLS []bool, from, rcpt, data string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.authSeen...), append([]bool(nil), s.authTLS...), s.mailFrom, s.rcptTo, s.data
}

func TestSMTPMailer_VerificationEmailOverSTARTTLS(t *testing.T) {
	srv, m := startFakeSMTP(t, nil)
	m.From, m.FromName = "no-reply@example.com", "Sigma Academy Pruebas"
	token, _, _ := generateVerificationToken()
	link := "https://ti3.example.cl/verificar?token=" + token
	err := m.SendVerificationEmail(context.Background(), VerificationEmail{
		Recipient: "student@example.com", Token: token, Link: link, ExpiresIn: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	auth, authTLS, from, rcpt, data := srv.snapshot()
	if len(auth) != 1 || auth[0] != "\x00sender@example.com\x00app-password-secret" || !authTLS[0] {
		t.Fatalf("AUTH PLAIN debe viajar cifrado tras STARTTLS: %q %v", auth, authTLS)
	}
	if from != "MAIL FROM:<no-reply@example.com>" || !strings.HasPrefix(rcpt, "RCPT TO:<student@example.com>") {
		t.Fatalf("sobre SMTP inesperado: %q %q", from, rcpt)
	}
	for _, want := range []string{
		"From: \"Sigma Academy Pruebas\" <no-reply@example.com>", "To: student@example.com",
		"Subject: Confirma tu correo en Sigma Academy", "Content-Type: text/plain; charset=UTF-8",
		link, "\n" + token + "\n", "24 horas", "solo puede usarse una vez",
		"Content-Type: multipart/alternative; boundary=", "Content-Type: text/html; charset=UTF-8",
		"Confirma tu cuenta", "CÓDIGO DE CONFIRMACIÓN:",
	} {
		if !strings.Contains(strings.ReplaceAll(data, "\r\n", "\n"), want) {
			t.Fatalf("falta %q en el mensaje:\n%s", want, data)
		}
	}
}

func TestSMTPMailer_ImplicitTLSAndResetCode(t *testing.T) {
	srv, m := startFakeSMTP(t, func(s *fakeSMTPServer) { s.implicit = true })
	if err := m.SendResetCode(context.Background(), "student@example.com", "12345678"); err != nil {
		t.Fatalf("send: %v", err)
	}
	_, authTLS, from, _, data := srv.snapshot()
	if !authTLS[0] || from != "MAIL FROM:<sender@example.com>" {
		t.Fatal("TLS implícito: AUTH cifrado y remitente por defecto = SMTP_USERNAME")
	}
	if !strings.Contains(data, "From: \"Sigma Academy\" <sender@example.com>") || !strings.Contains(data, "12345678") ||
		!strings.Contains(data, "Subject: Recupera tu acceso a Sigma Academy") {
		t.Fatalf("el correo de recuperación debe conservarse:\n%s", data)
	}
	for _, want := range []string{
		"Content-Type: multipart/alternative; boundary=", "Content-Type: text/plain; charset=UTF-8",
		"Content-Type: text/html; charset=UTF-8", "Recupera tu contraseña", "15 minutos",
	} {
		if !strings.Contains(data, want) {
			t.Fatalf("falta %q en el mensaje:\n%s", want, data)
		}
	}
}

func TestSMTPMailer_NeverSendsCredentialsWithoutTLS(t *testing.T) {
	srv, m := startFakeSMTP(t, func(s *fakeSMTPServer) { s.noTLS = true })
	err := m.SendResetCode(context.Background(), "student@example.com", "12345678")
	var se *SMTPError
	if !errors.As(err, &se) || se.Stage != "starttls" {
		t.Fatalf("sin STARTTLS debe abortar, got %v", err)
	}
	if auth, _, _, _, _ := srv.snapshot(); len(auth) != 0 {
		t.Fatal("las credenciales nunca deben enviarse sin cifrar")
	}
}

func TestSMTPMailer_ErrorsCarryStageAndCodeButNoSecrets(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func(*fakeSMTPServer)
		stage     string
		code      int
	}{
		{"auth rejected", func(s *fakeSMTPServer) { s.authCode = 535 }, "auth", 535},
		{"recipient rejected", func(s *fakeSMTPServer) { s.rcptCode = 550 }, "rcpt", 550},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, m := startFakeSMTP(t, tc.configure)
			token, _, _ := generateVerificationToken()
			err := m.SendVerificationEmail(context.Background(), VerificationEmail{Recipient: "student@example.com", Token: token, ExpiresIn: time.Hour})
			var se *SMTPError
			if !errors.As(err, &se) || se.Stage != tc.stage || se.Code != tc.code {
				t.Fatalf("got %#v", err)
			}
			for _, secret := range []string{"app-password-secret", "sender@example.com", "student@example.com", token, "Username and Password"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("el error expone %q: %s", secret, err)
				}
			}
		})
	}
}

func TestSMTPMailer_DialFailureAndTimeout(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	_, closedPort, _ := net.SplitHostPort(ln.Addr().String())
	ln.Close()
	m := &SMTPMailer{Host: "127.0.0.1", Port: closedPort, Username: "u@example.com", Password: "p", Timeout: time.Second}
	var se *SMTPError
	if err := m.SendResetCode(context.Background(), "a@example.com", "1"); !errors.As(err, &se) || se.Stage != "dial" {
		t.Fatalf("dial: %v", err)
	}

	_, hung := startFakeSMTP(t, func(s *fakeSMTPServer) { s.hang = true })
	hung.Timeout = 200 * time.Millisecond
	start := time.Now()
	err := hung.SendResetCode(context.Background(), "a@example.com", "1")
	if !errors.As(err, &se) || time.Since(start) > 2*time.Second {
		t.Fatalf("un servidor colgado debe cortar por timeout: %v (%s)", err, time.Since(start))
	}
	if errorDetail(err) == "" || !strings.HasPrefix(errorDetail(err), "smtp_stage=") {
		t.Fatalf("detalle de log inesperado: %q", errorDetail(err))
	}
}

func TestSMTPMailer_ValidateAndReady(t *testing.T) {
	base := SMTPMailer{Host: "smtp.gmail.com", Port: "587", Username: "sigma@example.com", Password: "x"}
	if !base.Ready() || base.Validate() != nil {
		t.Fatal("configuración por defecto válida")
	}
	for name, mutate := range map[string]func(*SMTPMailer){
		"port text":     func(m *SMTPMailer) { m.Port = "smtp" },
		"port range":    func(m *SMTPMailer) { m.Port = "70000" },
		"host spaces":   func(m *SMTPMailer) { m.Host = "smtp gmail.com" },
		"tls mode":      func(m *SMTPMailer) { m.TLSMode = "none" },
		"from invalid":  func(m *SMTPMailer) { m.From = "Sigma <a@example.com>" },
		"from crlf":     func(m *SMTPMailer) { m.From = "a@example.com\r\nBcc: b@example.com" },
		"name crlf":     func(m *SMTPMailer) { m.FromName = "Sigma\r\nBcc: x" },
		"user crlf":     func(m *SMTPMailer) { m.Username = "a@example.com\r\n" },
		"user not mail": func(m *SMTPMailer) { m.Username = "apikey" },
	} {
		m := base
		m.Password = "very-secret-password"
		mutate(&m)
		err := m.Validate()
		if err == nil || m.Ready() {
			t.Fatalf("%s: debe ser inválida", name)
		}
		if strings.Contains(err.Error(), m.Password) || strings.Contains(err.Error(), "example.com") {
			t.Fatalf("%s: el error solo nombra el campo: %v", name, err)
		}
	}
	// Con SMTP_FROM explícito, SMTP_USERNAME puede no ser una dirección.
	m := base
	m.Username, m.From, m.TLSMode = "apikey", "no-reply@example.com", "TLS"
	if m.Validate() != nil {
		t.Fatalf("usuario no-correo con SMTP_FROM: %v", m.Validate())
	}
	for _, missing := range []func(*SMTPMailer){
		func(m *SMTPMailer) { m.Password = "" }, func(m *SMTPMailer) { m.Host = "" },
		func(m *SMTPMailer) { m.Port = "" }, func(m *SMTPMailer) { m.Username = "" },
	} {
		m := base
		missing(&m)
		if m.Ready() {
			t.Fatal("faltan datos: no está listo")
		}
	}
	var nilMailer *SMTPMailer
	if nilMailer.Ready() {
		t.Fatal("nil no está listo")
	}
}

func TestSMTPMailer_RejectsUnsafeRecipient(t *testing.T) {
	srv, m := startFakeSMTP(t, nil)
	for _, rcpt := range []string{"a@example.com\r\nBcc: b@example.com", "Nombre <a@example.com>", "no-es-correo", ""} {
		var se *SMTPError
		if err := m.SendResetCode(context.Background(), rcpt, "1"); !errors.As(err, &se) || se.Stage != "address" {
			t.Fatalf("%q: %v", rcpt, err)
		}
	}
	if auth, _, _, _, _ := srv.snapshot(); len(auth) != 0 {
		t.Fatal("un destinatario inválido no debe ni conectar")
	}
	notReady := &SMTPMailer{}
	var se *SMTPError
	if err := notReady.SendResetCode(context.Background(), "a@example.com", "1"); !errors.As(err, &se) || se.Stage != "config" {
		t.Fatalf("sin configuración: %v", err)
	}
}
