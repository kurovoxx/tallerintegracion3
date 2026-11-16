package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/utils"
)

// Política de confirmación de correo.
const (
	DefaultEmailVerificationTTL = 24 * time.Hour
	MinEmailVerificationTTL     = 15 * time.Minute
	MaxEmailVerificationTTL     = 7 * 24 * time.Hour

	// Envíos de confirmación en curso a la vez (registro + reenvío). Cada uno
	// usa una conexión de PostgreSQL durante Issue y el pool de Auth tiene
	// MaxConns = 5: con 4 queda siempre al menos una para login y el resto.
	EmailVerificationMaxConcurrentSends = 4

	// Tope de cada envío en segundo plano (Issue + SMTP + ReleaseCooldown).
	emailVerificationSendTimeout = 30 * time.Second

	// 32 bytes aleatorios en base64url sin relleno = 43 caracteres (256 bits).
	verificationTokenBytes  = 32
	verificationTokenLength = 43
)

// EmailVerificationStore persiste los tokens (solo su hash). Implementación
// real: repository.EmailVerificationRepository.
type EmailVerificationStore interface {
	// Issue guarda el hash para userID si la cuenta existe, no está confirmada
	// y no superó el enfriamiento ni el tope por hora; reemplaza cualquier
	// token anterior. Todo en una transacción.
	Issue(ctx context.Context, userID string, tokenHash []byte, ttl time.Duration) (repository.IssueOutcome, error)
	// ReleaseCooldown permite reintentar de inmediato si el envío falló. Solo
	// actúa si tokenHash sigue siendo el token vigente y no consumido.
	ReleaseCooldown(ctx context.Context, userID string, tokenHash []byte) error
	// Confirm canjea el token de forma atómica y devuelve el user_id afectado
	// ("" si el token no existe).
	Confirm(ctx context.Context, tokenHash []byte) (repository.ConfirmOutcome, string, error)
}

// VerificationEmail es el contenido que necesita el mailer.
type VerificationEmail struct {
	Recipient string
	Token     string
	Link      string // vacío si EMAIL_VERIFICATION_URL no está configurada
	ExpiresIn time.Duration
}

type VerificationMailer interface {
	Ready() bool
	SendVerificationEmail(ctx context.Context, email VerificationEmail) error
}

type verificationUserLookup interface {
	GetByEmail(ctx context.Context, email string) (*model.User, error)
}

// EmailVerificationConfig agrupa la configuración validada al arrancar.
type EmailVerificationConfig struct {
	LinkBaseURL string        // EMAIL_VERIFICATION_URL (opcional)
	TTL         time.Duration // EMAIL_VERIFICATION_TTL; 0 = 24 h
}

type EmailVerificationService struct {
	users    verificationUserLookup
	store    EmailVerificationStore
	mailer   VerificationMailer
	linkBase *url.URL
	ttl      time.Duration
	// slots es el semáforo de envíos en curso (capacidad
	// EmailVerificationMaxConcurrentSends). Se ocupa ANTES de crear la
	// goroutine, así que nunca hay goroutines esperando cupo.
	slots chan struct{}
	// spawn lanza el trabajo ya autorizado por slots. Producción: go f(). Las
	// pruebas lo sustituyen por una llamada síncrona.
	spawn func(func())
}

func NewEmailVerificationService(users verificationUserLookup, store EmailVerificationStore, mailer VerificationMailer, cfg EmailVerificationConfig) (*EmailVerificationService, error) {
	ttl := cfg.TTL
	if ttl == 0 {
		ttl = DefaultEmailVerificationTTL
	}
	if ttl < MinEmailVerificationTTL || ttl > MaxEmailVerificationTTL {
		return nil, fmt.Errorf("EMAIL_VERIFICATION_TTL inválida o fuera de rango (duración Go entre %s y %s, p. ej. 24h)", MinEmailVerificationTTL, MaxEmailVerificationTTL)
	}
	linkBase, err := parseVerificationLinkBase(cfg.LinkBaseURL)
	if err != nil {
		return nil, err
	}
	return &EmailVerificationService{
		users: users, store: store, mailer: mailer, linkBase: linkBase, ttl: ttl,
		slots: make(chan struct{}, EmailVerificationMaxConcurrentSends),
		spawn: func(f func()) { go f() },
	}, nil
}

// parseVerificationLinkBase exige una URL absoluta sin fragmento; https salvo
// en localhost. El token se añade como parámetro ?token=.
func parseVerificationLinkBase(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	invalid := errors.New("EMAIL_VERIFICATION_URL inválida: debe ser una URL https absoluta sin fragmento (http solo en localhost)")
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Fragment != "" || u.User != nil || u.Query().Has("token") {
		return nil, invalid
	}
	host := u.Hostname()
	local := host == "localhost" || net.ParseIP(host).IsLoopback()
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return nil, invalid
	}
	return u, nil
}

func (s *EmailVerificationService) Ready() bool {
	return s != nil && s.mailer != nil && s.mailer.Ready()
}

func (s *EmailVerificationService) TTL() time.Duration { return s.ttl }

func (s *EmailVerificationService) link(token string) string {
	if s.linkBase == nil {
		return ""
	}
	u := *s.linkBase
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return u.String()
}

// generateVerificationToken devuelve el token para el correo y su SHA-256,
// que es lo único que se persiste.
func generateVerificationToken() (string, []byte, error) {
	b := make([]byte, verificationTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	raw := base64.RawURLEncoding.EncodeToString(b)
	return raw, hashVerificationToken(raw), nil
}

func hashVerificationToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

// validVerificationTokenFormat descarta, sin tocar la base, cualquier valor
// que no pueda haber salido de generateVerificationToken.
func validVerificationTokenFormat(raw string) bool {
	if len(raw) != verificationTokenLength {
		return false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	return err == nil && len(decoded) == verificationTokenBytes
}

// logVerification sigue el formato de logAuthOp ([auth] ... op=... k=v). Nunca
// recibe tokens, hashes, correos ni credenciales.
func logVerification(op, userID, step, status, details string) {
	if userID == "" {
		userID = "-"
	}
	if details = strings.TrimSpace(details); details != "" {
		details = " " + details
	}
	log.Printf("[auth] email-verification op=%s user_id=%s step=%s status=%s%s", op, userID, step, status, details)
}

// errorDetail resume un error sin su texto: etapa/código SMTP, SQLSTATE o
// timeout. Los textos de error de SMTP o de PostgreSQL pueden repetir
// direcciones o valores, así que nunca se registran tal cual.
func errorDetail(err error) string {
	var smtpErr *SMTPError
	if errors.As(err, &smtpErr) {
		d := "smtp_stage=" + smtpErr.Stage
		if smtpErr.Code > 0 {
			d += " smtp_code=" + strconv.Itoa(smtpErr.Code)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			d += " timeout=true"
		}
		return d
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return "sqlstate=" + pgErr.Code
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "cause=context"
	}
	return "cause=internal"
}

// SendForNewUser despacha en segundo plano la confirmación tras un registro
// exitoso y vuelve de inmediato: la respuesta 201 no espera al SMTP. El
// usuario ya existe, así que ningún fallo aquí deshace ni repite el registro;
// queda en el log y la persona puede pedir un reenvío.
func (s *EmailVerificationService) SendForNewUser(ctx context.Context, user *model.User) {
	const op = "Register"
	if user == nil {
		return
	}
	if !s.Ready() {
		logVerification(op, user.ID, "mailer", "skipped", "reason=mail_unavailable")
		return
	}
	s.dispatchSend(ctx, op, user.ID, user.Email)
}

// Request atiende POST /auth/send-verification-email (solicitud y reenvío).
// Con correo bien formado y SMTP disponible siempre devuelve nil, exista o no
// la cuenta, para no permitir enumerarlas. El envío ocurre en segundo plano
// (dispatchSend), así que el tiempo de respuesta tampoco delata la cuenta.
func (s *EmailVerificationService) Request(ctx context.Context, email string) error {
	const op = "SendVerificationEmail"
	email = strings.ToLower(strings.TrimSpace(email))
	if !utils.ValidateEmail(email) {
		return NewServiceError(utils.ErrInvalidEmail)
	}
	if !s.Ready() {
		logVerification(op, "", "mailer", "error", "reason=mail_unavailable")
		return &ServiceError{Code: "mail_unavailable", Message: "El envío de correos no está disponible temporalmente. Inténtalo más tarde."}
	}
	user, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		logVerification(op, "", "lookup", "error", errorDetail(err))
		return err
	}
	if user == nil {
		logVerification(op, "", "lookup", "ok", "outcome=unknown_email")
		return nil
	}
	s.dispatchSend(ctx, op, user.ID, user.Email)
	return nil
}

// dispatchSend es el único camino de envío (registro y reenvío). Ocupa un
// cupo sin bloquear; si no hay, descarta el envío al instante, antes de
// generar o guardar token, así que no deja tokens sin correo ni consume el
// enfriamiento o el tope por hora: un reenvío posterior funciona igual. El
// cupo se libera siempre (defer), también ante errores o panic.
func (s *EmailVerificationService) dispatchSend(ctx context.Context, op, userID, recipient string) {
	select {
	case s.slots <- struct{}{}:
	default:
		logVerification(op, userID, "dispatch", "skipped", "reason=busy")
		return
	}
	// El trabajo sobrevive al fin de la petición HTTP con un contexto propio.
	bg := context.WithoutCancel(ctx)
	s.spawn(func() {
		defer func() { <-s.slots }()
		defer func() {
			if recover() != nil {
				// El valor del panic no se registra: podría contener datos.
				logVerification(op, userID, "dispatch", "error", "cause=panic")
			}
		}()
		ctx, cancel := context.WithTimeout(bg, emailVerificationSendTimeout)
		defer cancel()
		s.issueAndSend(ctx, op, userID, recipient)
	})
}

func (s *EmailVerificationService) issueAndSend(ctx context.Context, op, userID, recipient string) {
	token, hash, err := generateVerificationToken()
	if err != nil {
		logVerification(op, userID, "token", "error", "cause=rand")
		return
	}
	outcome, err := s.store.Issue(ctx, userID, hash, s.ttl)
	if err != nil {
		logVerification(op, userID, "persist", "error", errorDetail(err))
		return
	}
	if outcome != repository.IssueIssued {
		logVerification(op, userID, "persist", "skipped", "outcome="+string(outcome))
		return
	}
	err = s.mailer.SendVerificationEmail(ctx, VerificationEmail{
		Recipient: recipient, Token: token, Link: s.link(token), ExpiresIn: s.ttl,
	})
	if err != nil {
		logVerification(op, userID, "send", "error", errorDetail(err))
		// El token queda guardado pero sin enfriamiento: un reenvío inmediato
		// genera otro y lo reemplaza. No se borra porque un fallo tardío del
		// SMTP no garantiza que el correo no haya salido.
		if relErr := s.store.ReleaseCooldown(ctx, userID, hash); relErr != nil {
			logVerification(op, userID, "release_cooldown", "error", errorDetail(relErr))
		}
		return
	}
	logVerification(op, userID, "send", "ok", "outcome=sent")
}

// Confirm atiende POST /auth/verify-email.
func (s *EmailVerificationService) Confirm(ctx context.Context, token string) (repository.ConfirmOutcome, error) {
	const op = "VerifyEmail"
	token = strings.TrimSpace(token)
	if !validVerificationTokenFormat(token) {
		logVerification(op, "", "validate", "rejected", "outcome=malformed")
		return repository.ConfirmInvalid, nil
	}
	outcome, userID, err := s.store.Confirm(ctx, hashVerificationToken(token))
	if err != nil {
		logVerification(op, userID, "confirm", "error", errorDetail(err))
		return "", err
	}
	status := "ok"
	if outcome != repository.ConfirmVerified && outcome != repository.ConfirmAlreadyVerified {
		status = "rejected"
	}
	logVerification(op, userID, "confirm", status, "outcome="+string(outcome))
	return outcome, nil
}

// SendVerificationEmail compone el correo de confirmación. Incluye el enlace
// si EMAIL_VERIFICATION_URL está configurada y siempre el token, para poder
// pegarlo en la app.
func (m *SMTPMailer) SendVerificationEmail(ctx context.Context, email VerificationEmail) error {
	var body strings.Builder
	body.WriteString("¡Hola! Gracias por registrarte en Sigma Academy.\r\n\r\n")
	if email.Link != "" {
		body.WriteString("Para confirmar tu correo, abre este enlace:\r\n" + email.Link + "\r\n\r\n")
		body.WriteString("Si el enlace no funciona, usa este código de confirmación:\r\n")
	} else {
		body.WriteString("Para confirmar tu correo, ingresa este código en Sigma Academy:\r\n")
	}
	body.WriteString(email.Token + "\r\n\r\n")
	body.WriteString("Es válido durante " + humanDuration(email.ExpiresIn) + " y solo puede usarse una vez. ")
	body.WriteString("Si pides otro correo, este deja de servir.\r\n")
	body.WriteString("Si no creaste una cuenta en Sigma Academy, ignora este mensaje.\r\n")
	return m.send(ctx, email.Recipient, "Confirma tu correo en Sigma Academy", body.String())
}

func humanDuration(d time.Duration) string {
	if d >= time.Hour && d%time.Hour == 0 {
		h := int(d / time.Hour)
		if h == 1 {
			return "1 hora"
		}
		return strconv.Itoa(h) + " horas"
	}
	m := int(d / time.Minute)
	if m == 1 {
		return "1 minuto"
	}
	return strconv.Itoa(m) + " minutos"
}
