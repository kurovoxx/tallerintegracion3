package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/utils"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Errores de dominio para Drive OAuth
var (
	ErrInvalidOAuthCode     = errors.New("invalid_oauth_code")
	ErrGoogleUnavailable    = errors.New("google_unavailable")
	ErrDriveNotConnected    = errors.New("not_connected")
	ErrDriveConnectionInvalid = errors.New("drive_connection_invalid")
)

// DriveOAuthResult es lo que devuelve el proveedor tras intercambiar oauth_code.
type DriveOAuthResult struct {
	AccessToken  string
	RefreshToken *string
	ExpiresAt    *time.Time
	ExternalEmail *string
}

// DriveOAuthProvider es la interfaz inyectable para intercambiar oauth_code por tokens.
// Permite mock en tests y aislar la integración real con Google.
type DriveOAuthProvider interface {
	Exchange(ctx context.Context, oauthCode string) (*DriveOAuthResult, error)
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
	oauthCode = strings.TrimSpace(oauthCode)
	if strings.TrimSpace(p.ClientID) == "" || strings.TrimSpace(p.ClientSecret) == "" || strings.TrimSpace(p.RedirectURI) == "" {
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
	// Scopes requeridos: drive.file + openid email profile
	cfg := &oauth2.Config{
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
		RedirectURL:  p.RedirectURI,
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
}

func NewDriveOAuthService(repo OAuthRepository, provider DriveOAuthProvider) *DriveOAuthService {
	return &DriveOAuthService{oauthRepo: repo, provider: provider}
}

// Connect intercambia oauth_code y guarda la conexión. No devuelve tokens.
func (s *DriveOAuthService) Connect(ctx context.Context, userID, oauthCode string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return NewServiceError(utils.ErrUnauthorized)
	}
	oauthCode = strings.TrimSpace(oauthCode)
	if oauthCode == "" {
		return NewServiceError("bad_request")
	}
	// Intercambiar con proveedor
	result, err := s.provider.Exchange(ctx, oauthCode)
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
				return "", NewServiceError("drive_connection_invalid")
			}
			if errors.Is(err, ErrGoogleUnavailable) {
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
