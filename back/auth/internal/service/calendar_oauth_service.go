package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/utils"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Errores de dominio para Calendar OAuth (los de intercambio se reutilizan de Drive).
var (
	ErrCalendarNotConnected      = errors.New("not_connected")
	ErrCalendarConnectionInvalid = errors.New("calendar_connection_invalid")
)

// CalendarOAuthResult es lo que devuelve el proveedor tras intercambiar oauth_code.
type CalendarOAuthResult struct {
	AccessToken   string
	RefreshToken  *string
	ExpiresAt     *time.Time
	ExternalEmail *string
}

// CalendarOAuthProvider es la interfaz inyectable para intercambiar oauth_code por tokens.
type CalendarOAuthProvider interface {
	Exchange(ctx context.Context, oauthCode string) (*CalendarOAuthResult, error)
}

// CalendarTokenRefresher es la interfaz para refrescar access_token vía refresh_token.
type CalendarTokenRefresher interface {
	Refresh(ctx context.Context, refreshToken string) (*CalendarOAuthResult, error)
}

// ConfigCalendarOAuthProvider usa GOOGLE_CLIENT_ID/SECRET/REDIRECT_URI del config.
// Si están vacías, no hace request y retorna ErrGoogleUnavailable.
// Para tests, se pueden inyectar Endpoint, UserinfoURL y HTTPClient para usar httptest.Server.
type ConfigCalendarOAuthProvider struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	// Opcionales para testing
	Endpoint    oauth2.Endpoint
	UserinfoURL string
	HTTPClient  *NetHTTPClient
}

// CalendarRedirectExchanger extiende el provider para flujos desktop con
// puerto efímero (RFC 8252), igual que Drive: el redirect_uri viaja en el
// request y debe validarse con ValidateRedirectURI antes de usarse.
type CalendarRedirectExchanger interface {
	ExchangeWithRedirect(ctx context.Context, oauthCode, redirectURI string) (*CalendarOAuthResult, error)
}

func (p *ConfigCalendarOAuthProvider) ConfiguredRedirectURI() string {
	return strings.TrimSpace(p.RedirectURI)
}

func (p *ConfigCalendarOAuthProvider) Exchange(ctx context.Context, oauthCode string) (*CalendarOAuthResult, error) {
	return p.exchangeWithRedirect(ctx, oauthCode, strings.TrimSpace(p.RedirectURI))
}

// ExchangeWithRedirect intercambia usando un redirect_uri validado por el
// llamador (flujo desktop con puerto efímero). Sin validación previa no usar.
func (p *ConfigCalendarOAuthProvider) ExchangeWithRedirect(ctx context.Context, oauthCode, redirectURI string) (*CalendarOAuthResult, error) {
	return p.exchangeWithRedirect(ctx, oauthCode, strings.TrimSpace(redirectURI))
}

func (p *ConfigCalendarOAuthProvider) exchangeWithRedirect(ctx context.Context, oauthCode, redirectURI string) (*CalendarOAuthResult, error) {
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
	cfg := &oauth2.Config{
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
		RedirectURL:  redirectURI,
		Endpoint:     endpoint,
		Scopes:       []string{"https://www.googleapis.com/auth/calendar.events", "openid", "email", "profile"},
	}
	ctx = withHTTPClient(ctx, p.HTTPClient)
	tok, err := cfg.Exchange(ctx, oauthCode)
	if err != nil {
		return nil, mapExchangeError(err)
	}
	if tok == nil || strings.TrimSpace(tok.AccessToken) == "" {
		return nil, errors.New("access_token vacío")
	}
	email, err := fetchUserEmail(ctx, toStdClient(p.HTTPClient), userinfoURL, tok.AccessToken)
	if err != nil {
		if errors.Is(err, ErrGoogleUnavailable) {
			return nil, ErrGoogleUnavailable
		}
		return nil, err
	}
	return &CalendarOAuthResult{
		AccessToken:   tok.AccessToken,
		RefreshToken:  strPtrOrNil(tok.RefreshToken),
		ExpiresAt:     timePtrOrNil(tok.Expiry),
		ExternalEmail: strPtrOrNil(email),
	}, nil
}

func (p *ConfigCalendarOAuthProvider) Refresh(ctx context.Context, refreshToken string) (*CalendarOAuthResult, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if strings.TrimSpace(p.ClientID) == "" || strings.TrimSpace(p.ClientSecret) == "" {
		return nil, ErrGoogleUnavailable
	}
	if refreshToken == "" {
		return nil, ErrCalendarConnectionInvalid
	}
	endpoint := p.Endpoint
	if endpoint.AuthURL == "" && endpoint.TokenURL == "" {
		endpoint = google.Endpoint
	}
	cfg := &oauth2.Config{
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
		Endpoint:     endpoint,
	}
	ctx = withHTTPClient(ctx, p.HTTPClient)
	tokSource := cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken})
	tok, err := tokSource.Token()
	if err != nil {
		return nil, mapRefreshError(err)
	}
	if tok == nil || strings.TrimSpace(tok.AccessToken) == "" {
		return nil, errors.New("access_token vacío")
	}
	return &CalendarOAuthResult{
		AccessToken:  tok.AccessToken,
		RefreshToken: strPtrOrNil(tok.RefreshToken),
		ExpiresAt:    timePtrOrNil(tok.Expiry),
	}, nil
}

// CalendarOAuthService orquesta la conexión de Google Calendar.
type CalendarOAuthService struct {
	oauthRepo CalendarOAuthRepository
	provider  CalendarOAuthProvider
}

type CalendarOAuthRepository interface {
	UpsertGoogleCalendarConnection(ctx context.Context, userID, accessToken string, refreshToken *string, expiresAt *time.Time, externalEmail *string) error
	GetByUserIDAndProvider(ctx context.Context, userID, provider string) (*model.OAuthConnection, error)
	UpdateGoogleCalendarAccessToken(ctx context.Context, userID, accessToken string, refreshToken *string, expiresAt time.Time) error
	MarkGoogleCalendarConnectionRevoked(ctx context.Context, userID string) error
}

func NewCalendarOAuthService(repo CalendarOAuthRepository, provider CalendarOAuthProvider) *CalendarOAuthService {
	return &CalendarOAuthService{oauthRepo: repo, provider: provider}
}

// Connect intercambia oauth_code y guarda la conexión. No devuelve tokens.
func (s *CalendarOAuthService) Connect(ctx context.Context, userID, oauthCode string) error {
	return s.ConnectWithRedirect(ctx, userID, oauthCode, "")
}

// ConnectWithRedirect acepta el redirect_uri real del flujo desktop con
// puerto efímero (RFC 8252). Vacío = redirect configurado. Se valida con la
// misma allowlist que Drive antes de usarse.
func (s *CalendarOAuthService) ConnectWithRedirect(ctx context.Context, userID, oauthCode, redirectURI string) error {
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
		ex, ok := s.provider.(CalendarRedirectExchanger)
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
		exchanger = func(ctx context.Context, code string) (*CalendarOAuthResult, error) {
			return ex.ExchangeWithRedirect(ctx, code, redirectURI)
		}
	}
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
	if err := s.oauthRepo.UpsertGoogleCalendarConnection(ctx, userID, result.AccessToken, result.RefreshToken, result.ExpiresAt, result.ExternalEmail); err != nil {
		return NewServiceError("internal_error")
	}
	return nil
}

// GetValidAccessToken devuelve un access_token vigente para Google Calendar por user_id.
// Reutilizable por Social vía endpoint interno. No crea endpoint HTTP por sí solo.
func (s *CalendarOAuthService) GetValidAccessToken(ctx context.Context, userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", NewServiceError("bad_request")
	}
	conn, err := s.oauthRepo.GetByUserIDAndProvider(ctx, userID, model.ProviderGoogleCalendar)
	if err != nil {
		return "", NewServiceError("internal_error")
	}
	if conn == nil {
		return "", NewServiceError("not_connected")
	}
	if conn.RevokedAt != nil {
		return "", NewServiceError("calendar_connection_invalid")
	}
	if strings.TrimSpace(conn.AccessToken) == "" {
		return "", NewServiceError("calendar_connection_invalid")
	}
	// Si expires_at es nil o tiene más de 5 minutos restantes, está vigente
	if conn.ExpiresAt == nil || time.Now().Add(5*time.Minute).Before(*conn.ExpiresAt) {
		return conn.AccessToken, nil
	}
	// Vencido o dentro del margen: necesita refresh
	if conn.RefreshToken == nil || strings.TrimSpace(*conn.RefreshToken) == "" {
		return "", NewServiceError("calendar_connection_invalid")
	}
	refresher, ok := s.provider.(CalendarTokenRefresher)
	if !ok {
		return "", NewServiceError("google_unavailable")
	}
	result, err := refresher.Refresh(ctx, strings.TrimSpace(*conn.RefreshToken))
	if err != nil {
		if errors.Is(err, ErrCalendarConnectionInvalid) {
			if markErr := s.oauthRepo.MarkGoogleCalendarConnectionRevoked(ctx, userID); markErr != nil {
				return "", NewServiceError("internal_error")
			}
			return "", NewServiceError("calendar_connection_invalid")
		}
		if errors.Is(err, ErrGoogleUnavailable) {
			return "", NewServiceError("google_unavailable")
		}
		return "", NewServiceError("internal_error")
	}
	if result == nil || strings.TrimSpace(result.AccessToken) == "" {
		return "", NewServiceError("internal_error")
	}
	expiresAt := time.Now().Add(1 * time.Hour)
	if result.ExpiresAt != nil && !result.ExpiresAt.IsZero() {
		expiresAt = *result.ExpiresAt
	}
	if err := s.oauthRepo.UpdateGoogleCalendarAccessToken(ctx, userID, result.AccessToken, result.RefreshToken, expiresAt); err != nil {
		return "", NewServiceError("internal_error")
	}
	return result.AccessToken, nil
}

// CalendarConnectionStatus es el estado para la UI (vincular/reconectar).
// Nunca incluye tokens ni secretos: solo bandera + email externo opcional.
type CalendarConnectionStatus struct {
	Connected     bool
	ExternalEmail *string
}

// GetCalendarConnectionStatus devuelve el estado sin HTTP ni escritura.
// disconnected si no hay fila o está revocada; nunca expone tokens.
func (s *CalendarOAuthService) GetCalendarConnectionStatus(ctx context.Context, userID string) (CalendarConnectionStatus, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return CalendarConnectionStatus{}, NewServiceError("bad_request")
	}
	conn, err := s.oauthRepo.GetByUserIDAndProvider(ctx, userID, model.ProviderGoogleCalendar)
	if err != nil {
		return CalendarConnectionStatus{}, NewServiceError("internal_error")
	}
	if conn == nil || conn.RevokedAt != nil {
		return CalendarConnectionStatus{Connected: false}, nil
	}
	return CalendarConnectionStatus{Connected: true, ExternalEmail: conn.ExternalAccountEmail}, nil
}

// ReportCalendarPermissionDenied marca revoked_at para que el cliente muestre "reconecta Calendar".
// Será llamado por Social cuando reciba 401/403 de Google Calendar. Idempotente.
func (s *CalendarOAuthService) ReportCalendarPermissionDenied(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return NewServiceError("bad_request")
	}
	if err := s.oauthRepo.MarkGoogleCalendarConnectionRevoked(ctx, userID); err != nil {
		return NewServiceError("internal_error")
	}
	return nil
}
