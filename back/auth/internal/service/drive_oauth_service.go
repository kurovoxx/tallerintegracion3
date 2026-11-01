package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/utils"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Errores de dominio para Drive OAuth
var (
	ErrInvalidOAuthCode       = errors.New("invalid_oauth_code")
	ErrGoogleUnavailable      = errors.New("google_unavailable")
	ErrDriveNotConnected      = errors.New("not_connected")
	ErrDriveConnectionInvalid = errors.New("drive_connection_invalid")
)

// DriveOAuthResult es lo que devuelve el proveedor tras intercambiar oauth_code.
type DriveOAuthResult struct {
	AccessToken   string
	RefreshToken  *string
	ExpiresAt     *time.Time
	ExternalEmail *string
}

// DriveOAuthProvider es la interfaz inyectable para intercambiar oauth_code por tokens.
// Permite mock en tests y aislar la integración real con Google.
type DriveOAuthProvider interface {
	Exchange(ctx context.Context, oauthCode string) (*DriveOAuthResult, error)
}

// DriveRedirectExchanger extiende el provider para flujos desktop con puerto
// efímero (RFC 8252): el redirect_uri viaja en el request y debe validarse
// con ValidateRedirectURI antes de usarse. ConfigDriveOAuthProvider la implementa.
type DriveRedirectExchanger interface {
	ExchangeWithRedirect(ctx context.Context, oauthCode, redirectURI string) (*DriveOAuthResult, error)
}

// ErrInvalidRedirectURI se responde 400: el redirect no está en la allowlist.
var ErrInvalidRedirectURI = errors.New("invalid_redirect_uri")

// ValidateRedirectURI allowlist para redirect_uri provisto por el cliente:
//   - loopback 127.0.0.1 por http en cualquier puerto y path /callback
//     (flujo desktop RFC 8252, válido sin pre-registro para clientes Desktop);
//   - legacy exacto http://localhost:8081/auth/google/callback;
//   - la redirect configurada (fallback web / prod https).
//
// Todo lo demás (incluido cualquier host externo) se rechaza: aceptar un
// redirect arbitrario filtraría el code a un atacante.
func ValidateRedirectURI(raw, configured, legacy string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ErrInvalidRedirectURI
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ErrInvalidRedirectURI
	}
	if u.Scheme == "http" && u.Hostname() == "127.0.0.1" && u.Path == "/callback" {
		return nil
	}
	if raw == strings.TrimSpace(legacy) || raw == strings.TrimSpace(configured) {
		return nil
	}
	return ErrInvalidRedirectURI
}

// legacyDesktopRedirect es el redirect fijo histórico del flujo desktop.
const legacyDesktopRedirect = "http://localhost:8081/auth/google/callback"

// redirectConfigured lo implementa el provider real para exponer la redirect
// configurada (fallback web/prod) en la validación de la allowlist.
type redirectConfigured interface {
	ConfiguredRedirectURI() string
}

// ConfiguredRedirectURI expone el RedirectURL configurado.
func (p *ConfigDriveOAuthProvider) ConfiguredRedirectURI() string {
	return strings.TrimSpace(p.RedirectURI)
}

// DriveTokenRefresher es la interfaz para refrescar access_token vía refresh_token.
// Separada de Exchange para no mezclar authorization_code con refresh_token grant.
type DriveTokenRefresher interface {
	Refresh(ctx context.Context, refreshToken string) (*DriveOAuthResult, error)
}

// ConfigDriveOAuthProvider usa GOOGLE_CLIENT_ID/SECRET/REDIRECT_URI del config.
// Si están vacías, no hace request y retorna ErrGoogleUnavailable.
// Para tests, se pueden inyectar Endpoint, UserinfoURL y HTTPClient para usar httptest.Server.
type ConfigDriveOAuthProvider struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	// Opcionales para testing
	Endpoint    oauth2.Endpoint
	UserinfoURL string
	HTTPClient  *http.Client
}

func (p *ConfigDriveOAuthProvider) Exchange(ctx context.Context, oauthCode string) (*DriveOAuthResult, error) {
	return p.exchangeWithRedirect(ctx, oauthCode, strings.TrimSpace(p.RedirectURI))
}

// ExchangeWithRedirect intercambia usando un redirect_uri validado por el
// llamador (flujo desktop con puerto efímero). Sin validación previa no usar.
func (p *ConfigDriveOAuthProvider) ExchangeWithRedirect(ctx context.Context, oauthCode, redirectURI string) (*DriveOAuthResult, error) {
	return p.exchangeWithRedirect(ctx, oauthCode, strings.TrimSpace(redirectURI))
}

func (p *ConfigDriveOAuthProvider) exchangeWithRedirect(ctx context.Context, oauthCode, redirectURI string) (*DriveOAuthResult, error) {
	oauthCode = strings.TrimSpace(oauthCode)
	if strings.TrimSpace(p.ClientID) == "" || strings.TrimSpace(p.ClientSecret) == "" || redirectURI == "" {
		return nil, ErrGoogleUnavailable
	}
	if oauthCode == "" {
		return nil, ErrInvalidOAuthCode
	}
	endpoint := p.Endpoint
	if endpoint.AuthURL == "" && endpoint.TokenURL == "" {
		endpoint = google.Endpoint
	}
	userinfoURL := p.UserinfoURL
	if strings.TrimSpace(userinfoURL) == "" {
		userinfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"
	}
	// Scopes mínimos (principio de menor privilegio):
	// - drive.file: solo archivos creados o abiertos explícitamente por la app
	//   (dentro de la carpeta "Apuntes TI3", ver notes drive.ensureAppFolder).
	//   NUNCA scope drive completo (vería todo el Drive) ni drive.readonly
	//   (impediría crear/subir).
	// - openid/email/profile: solo para capturar external_account_email.
	cfg := &oauth2.Config{
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
		RedirectURL:  redirectURI,
		Endpoint:     endpoint,
		Scopes:       []string{"https://www.googleapis.com/auth/drive.file", "openid", "email", "profile"},
	}
	// Inyectar HTTP client si se proveyó (para httptest)
	if p.HTTPClient != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, p.HTTPClient)
	}
	tok, err := cfg.Exchange(ctx, oauthCode)
	if err != nil {
		// Mapear invalid_grant / 400 -> ErrInvalidOAuthCode, otros 429/5xx/timeout -> ErrGoogleUnavailable
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "invalid_grant") || strings.Contains(msg, "invalid_request") || strings.Contains(msg, "400") {
			return nil, ErrInvalidOAuthCode
		}
		// oauth2.RetrieveError contiene Response con StatusCode
		var retrieveErr *oauth2.RetrieveError
		if errors.As(err, &retrieveErr) {
			if retrieveErr.Response != nil {
				code := retrieveErr.Response.StatusCode
				if code == 400 {
					return nil, ErrInvalidOAuthCode
				}
				if code == 429 || code >= 500 {
					return nil, ErrGoogleUnavailable
				}
			}
		}
		if strings.Contains(msg, "timeout") || strings.Contains(msg, "connection") || strings.Contains(msg, "429") || strings.Contains(msg, "500") || strings.Contains(msg, "502") || strings.Contains(msg, "503") {
			return nil, ErrGoogleUnavailable
		}
		// Por defecto, si no es invalid_grant, considerar como código inválido si es 400
		return nil, ErrInvalidOAuthCode
	}
	if tok == nil || strings.TrimSpace(tok.AccessToken) == "" {
		return nil, errors.New("access_token vacío")
	}
	// Obtener email via userinfo
	email, err := fetchUserEmail(ctx, p.HTTPClient, userinfoURL, tok.AccessToken)
	if err != nil {
		if errors.Is(err, ErrGoogleUnavailable) {
			return nil, ErrGoogleUnavailable
		}
		return nil, err
	}
	var refreshPtr *string
	if strings.TrimSpace(tok.RefreshToken) != "" {
		rt := tok.RefreshToken
		refreshPtr = &rt
	}
	var expPtr *time.Time
	if !tok.Expiry.IsZero() {
		exp := tok.Expiry
		expPtr = &exp
	}
	var emailPtr *string
	if strings.TrimSpace(email) != "" {
		em := email
		emailPtr = &em
	}
	return &DriveOAuthResult{
		AccessToken:   tok.AccessToken,
		RefreshToken:  refreshPtr,
		ExpiresAt:     expPtr,
		ExternalEmail: emailPtr,
	}, nil
}

func (p *ConfigDriveOAuthProvider) Refresh(ctx context.Context, refreshToken string) (*DriveOAuthResult, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if strings.TrimSpace(p.ClientID) == "" || strings.TrimSpace(p.ClientSecret) == "" {
		return nil, ErrGoogleUnavailable
	}
	if refreshToken == "" {
		return nil, ErrDriveConnectionInvalid
	}
	endpoint := p.Endpoint
	if endpoint.AuthURL == "" && endpoint.TokenURL == "" {
		endpoint = google.Endpoint
	}
	// Para refresh, no se necesita RedirectURI ni scopes, solo client_id, client_secret, refresh_token
	cfg := &oauth2.Config{
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
		Endpoint:     endpoint,
	}
	if p.HTTPClient != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, p.HTTPClient)
	}
	tokSource := cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken})
	tok, err := tokSource.Token()
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "invalid_grant") {
			return nil, ErrDriveConnectionInvalid
		}
		var retrieveErr *oauth2.RetrieveError
		if errors.As(err, &retrieveErr) {
			if retrieveErr.Response != nil {
				code := retrieveErr.Response.StatusCode
				if code == 400 {
					return nil, ErrDriveConnectionInvalid
				}
				if code == 429 || code >= 500 {
					return nil, ErrGoogleUnavailable
				}
			}
		}
		if strings.Contains(msg, "timeout") || strings.Contains(msg, "connection") || strings.Contains(msg, "429") || strings.Contains(msg, "500") {
			return nil, ErrGoogleUnavailable
		}
		return nil, ErrDriveConnectionInvalid
	}
	if tok == nil || strings.TrimSpace(tok.AccessToken) == "" {
		return nil, errors.New("access_token vacío")
	}
	var refreshPtr *string
	if strings.TrimSpace(tok.RefreshToken) != "" {
		rt := tok.RefreshToken
		refreshPtr = &rt
	}
	var expPtr *time.Time
	if !tok.Expiry.IsZero() {
		exp := tok.Expiry
		expPtr = &exp
	}
	// Para refresh, no necesitamos userinfo (ya tenemos email), pero lo mantenemos por si Google lo requiere
	return &DriveOAuthResult{
		AccessToken:  tok.AccessToken,
		RefreshToken: refreshPtr,
		ExpiresAt:    expPtr,
	}, nil
}

func fetchUserEmail(ctx context.Context, client *http.Client, userinfoURL, accessToken string) (string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userinfoURL, nil)
	if err != nil {
		return "", ErrGoogleUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := client.Do(req)
	if err != nil {
		return "", ErrGoogleUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 || resp.StatusCode == 429 || resp.StatusCode >= 500 {
		return "", ErrGoogleUnavailable
	}
	if resp.StatusCode != 200 {
		return "", errors.New("userinfo status no 200")
	}
	var data struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", errors.New("userinfo json inválido")
	}
	if strings.TrimSpace(data.Email) == "" {
		return "", errors.New("email vacío en userinfo")
	}
	return strings.TrimSpace(data.Email), nil
}

// DriveOAuthService orquesta la conexión de Google Drive.
type DriveOAuthService struct {
	oauthRepo OAuthRepository
	provider  DriveOAuthProvider
}

type OAuthRepository interface {
	UpsertGoogleDriveConnection(ctx context.Context, userID, accessToken string, refreshToken *string, expiresAt *time.Time, externalEmail *string) error
	GetByUserIDAndProvider(ctx context.Context, userID, provider string) (*model.OAuthConnection, error)
	UpdateGoogleDriveAccessToken(ctx context.Context, userID, accessToken string, refreshToken *string, expiresAt time.Time) error
	MarkGoogleDriveConnectionRevoked(ctx context.Context, userID string) error
	DeleteGoogleDriveConnection(ctx context.Context, userID string) error
}

func NewDriveOAuthService(repo OAuthRepository, provider DriveOAuthProvider) *DriveOAuthService {
	return &DriveOAuthService{oauthRepo: repo, provider: provider}
}

// Connect intercambia oauth_code y guarda la conexión. No devuelve tokens.
// expectedEmail (opcional, el correo declarado en la app): si viene y el email
// real de userinfo difiere, se rechaza con email_mismatch sin guardar nada
// (evita vincular la cuenta de Google equivocada).
// redirectURI (opcional, flujo desktop RFC 8252): si viene se valida con
// ValidateRedirectURI (allowlist) y se usa en el Exchange; vacío = configurado.
func (s *DriveOAuthService) Connect(ctx context.Context, userID, oauthCode string, expectedEmail *string, redirectURI string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return NewServiceError(utils.ErrUnauthorized)
	}
	oauthCode = strings.TrimSpace(oauthCode)
	if oauthCode == "" {
		return NewServiceError("bad_request")
	}
	redirectURI = strings.TrimSpace(redirectURI)
	exchanger := s.provider.Exchange
	if redirectURI != "" {
		ex, ok := s.provider.(DriveRedirectExchanger)
		if !ok {
			return NewServiceError("internal_error")
		}
		configured := ""
		if rc, ok := s.provider.(redirectConfigured); ok {
			configured = rc.ConfiguredRedirectURI()
		}
		if err := ValidateRedirectURI(redirectURI, configured, legacyDesktopRedirect); err != nil {
			return NewServiceError("invalid_redirect_uri")
		}
		exchanger = func(ctx context.Context, code string) (*DriveOAuthResult, error) {
			return ex.ExchangeWithRedirect(ctx, code, redirectURI)
		}
	}
	// Intercambiar con proveedor
	result, err := exchanger(ctx, oauthCode)
	if err != nil {
		if errors.Is(err, ErrInvalidOAuthCode) {
			return NewServiceError("invalid_oauth_code")
		}
		if errors.Is(err, ErrGoogleUnavailable) {
			return NewServiceError("google_unavailable")
		}
		return NewServiceError("internal_error")
	}
	if result == nil || strings.TrimSpace(result.AccessToken) == "" {
		return NewServiceError("internal_error")
	}
	if expectedEmail != nil && strings.TrimSpace(*expectedEmail) != "" &&
		result.ExternalEmail != nil && strings.TrimSpace(*result.ExternalEmail) != "" &&
		!strings.EqualFold(strings.TrimSpace(*expectedEmail), strings.TrimSpace(*result.ExternalEmail)) {
		return NewServiceError("email_mismatch")
	}
	// Upsert en BD
	if err := s.oauthRepo.UpsertGoogleDriveConnection(ctx, userID, result.AccessToken, result.RefreshToken, result.ExpiresAt, result.ExternalEmail); err != nil {
		return NewServiceError("internal_error")
	}
	return nil
}

// GetValidAccessToken devuelve un access_token vigente para Google Drive por user_id.
// Reutilizable por Notes via gRPC. No crea endpoint HTTP.
func (s *DriveOAuthService) GetValidAccessToken(ctx context.Context, userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", NewServiceError("bad_request")
	}
	conn, err := s.oauthRepo.GetByUserIDAndProvider(ctx, userID, model.ProviderGoogleDrive)
	if err != nil {
		return "", NewServiceError("internal_error")
	}
	if conn == nil {
		return "", NewServiceError("not_connected")
	}
	if conn.RevokedAt != nil {
		return "", NewServiceError("drive_connection_invalid")
	}
	if strings.TrimSpace(conn.AccessToken) == "" {
		return "", NewServiceError("drive_connection_invalid")
	}
	// Si expires_at es nil o tiene más de 5 minutos restantes, está vigente
	if conn.ExpiresAt == nil || time.Now().Add(5*time.Minute).Before(*conn.ExpiresAt) {
		return conn.AccessToken, nil
	}
	// Vencido o dentro del margen: necesita refresh
	if conn.RefreshToken == nil || strings.TrimSpace(*conn.RefreshToken) == "" {
		return "", NewServiceError("drive_connection_invalid")
	}
	// Verificar configuración
	if refresher, ok := s.provider.(DriveTokenRefresher); ok {
		// Usar refresher si implementa Refresh
		result, err := refresher.Refresh(ctx, strings.TrimSpace(*conn.RefreshToken))
		if err != nil {
			if errors.Is(err, ErrDriveConnectionInvalid) {
				// Marcar revocada para banner, si falla el mark, internal_error
				if markErr := s.oauthRepo.MarkGoogleDriveConnectionRevoked(ctx, userID); markErr != nil {
					return "", NewServiceError("internal_error")
				}
				log.Printf("drive oauth refresh failed user_id=%s code=drive_connection_invalid", userID)
				return "", NewServiceError("drive_connection_invalid")
			}
			if errors.Is(err, ErrGoogleUnavailable) {
				log.Printf("drive oauth refresh failed user_id=%s code=google_unavailable", userID)
				return "", NewServiceError("google_unavailable")
			}
			return "", NewServiceError("internal_error")
		}
		if result == nil || strings.TrimSpace(result.AccessToken) == "" {
			return "", NewServiceError("internal_error")
		}
		// Actualizar solo access_token, refresh_token (COALESCE), expires_at, updated_at
		expiresAt := time.Now().Add(1 * time.Hour)
		if result.ExpiresAt != nil && !result.ExpiresAt.IsZero() {
			expiresAt = *result.ExpiresAt
		}
		if err := s.oauthRepo.UpdateGoogleDriveAccessToken(ctx, userID, result.AccessToken, result.RefreshToken, expiresAt); err != nil {
			return "", NewServiceError("internal_error")
		}
		log.Printf("drive oauth refresh ok user_id=%s", userID)
		return result.AccessToken, nil
	}
	// Si el provider no implementa Refresh, intentar con Exchange no es correcto; retornar google_unavailable
	return "", NewServiceError("google_unavailable")
}

// DriveConnectionStatus es el estado interno para banner "reconecta Drive".
type DriveConnectionStatus struct {
	Connected         bool
	ReconnectRequired bool
}

// GetGoogleDriveConnectionStatus devuelve el estado de la conexión sin hacer HTTP ni actualizar BD.
// No retorna tokens, email, expiración, secret ni detalles OAuth.
func (s *DriveOAuthService) GetGoogleDriveConnectionStatus(ctx context.Context, userID string) (DriveConnectionStatus, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return DriveConnectionStatus{}, NewServiceError("bad_request")
	}
	conn, err := s.oauthRepo.GetByUserIDAndProvider(ctx, userID, model.ProviderGoogleDrive)
	if err != nil {
		return DriveConnectionStatus{}, NewServiceError("internal_error")
	}
	if conn == nil {
		return DriveConnectionStatus{Connected: false, ReconnectRequired: false}, nil
	}
	if conn.RevokedAt != nil {
		return DriveConnectionStatus{Connected: false, ReconnectRequired: true}, nil
	}
	return DriveConnectionStatus{Connected: true, ReconnectRequired: false}, nil
}

// ReportGoogleDrivePermissionDenied marca revoked_at para que el banner se muestre.
// Será llamado por Notes cuando reciba 401/403 de Drive. No requiere tokens, no llama Google, idempotente.
func (s *DriveOAuthService) ReportGoogleDrivePermissionDenied(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return NewServiceError("bad_request")
	}
	if err := s.oauthRepo.MarkGoogleDriveConnectionRevoked(ctx, userID); err != nil {
		// Si no existe fila, considerarlo internal_error (no debe ocurrir si Notes llama con user válido)
		return NewServiceError("internal_error")
	}
	return nil
}

// revokeURL de Google: revocar cualquier token del par invalida todo el grant.
const googleRevokeURL = "https://oauth2.googleapis.com/revoke"

// Disconnect desvincula Drive por petición explícita del usuario:
// revoca el grant en Google (best-effort, con timeout) y borra la fila local.
// Sin fila previa es éxito silencioso (idempotente). Nunca expone tokens.
func (s *DriveOAuthService) Disconnect(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return NewServiceError(utils.ErrUnauthorized)
	}
	conn, err := s.oauthRepo.GetByUserIDAndProvider(ctx, userID, model.ProviderGoogleDrive)
	if err != nil {
		return NewServiceError("internal_error")
	}
	if conn != nil {
		revokeGoogleGrant(ctx, conn.AccessToken, conn.RefreshToken)
	}
	if err := s.oauthRepo.DeleteGoogleDriveConnection(ctx, userID); err != nil {
		return NewServiceError("internal_error")
	}
	return nil
}

// revokeGoogleGrant intenta invalidar el grant en Google. Best-effort puro:
// cualquier fallo se loguea y se ignora (la fila local se borra igual,
// y el token huérfano expira solo).
func revokeGoogleGrant(ctx context.Context, accessToken string, refreshToken *string) {
	token := strings.TrimSpace(accessToken)
	if token == "" && refreshToken != nil {
		token = strings.TrimSpace(*refreshToken)
	}
	if token == "" {
		return
	}
	client := &http.Client{Timeout: 8 * time.Second}
	form := url.Values{"token": {token}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleRevokeURL, strings.NewReader(form))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("drive: revoke en Google falló (best-effort): %v", err)
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
}
