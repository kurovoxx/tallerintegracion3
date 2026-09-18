package drive

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// ProviderGoogleDrive es el valor de identity.oauth_connections.provider
// que usa el microservicio auth al conectar Drive (ver back/auth).
const ProviderGoogleDrive = "google_drive"

// TokenProvider entrega un access_token vigente de Google Drive para un usuario.
type TokenProvider interface {
	GetValidAccessToken(ctx context.Context, userID string) (string, error)
}

// oauthConnection refleja las columnas que notes necesita de identity.oauth_connections.
type oauthConnection struct {
	AccessToken  string
	RefreshToken *string
	ExpiresAt    *time.Time
	RevokedAt    *time.Time
}

// PGOAuthTokenStore lee los tokens OAuth del usuario desde Postgres
// (tabla identity.oauth_connections, provider='google_drive') y renueva el
// access_token con GOOGLE_CLIENT_ID/SECRET cuando expiró.
type PGOAuthTokenStore struct {
	pool         *pgxpool.Pool
	clientID     string
	clientSecret string
	// tokenURL y httpClient solo se usan en tests para inyectar un servidor falso.
	tokenURL   string
	httpClient *http.Client
}

// NewPGOAuthTokenStore crea el store. clientID/clientSecret pueden venir vacíos:
// en ese caso el refresh queda deshabilitado y se retorna error claro al expirar.
func NewPGOAuthTokenStore(pool *pgxpool.Pool, clientID, clientSecret string) *PGOAuthTokenStore {
	return &PGOAuthTokenStore{pool: pool, clientID: clientID, clientSecret: clientSecret}
}

// GetValidAccessToken devuelve el access_token vigente o un error claro:
//   - "no oauth connection found for user <id>" si no hay fila en BD.
//   - DriveError 403 si la conexión fue revocada.
//   - error de reconexión si expiró y no se puede refrescar.
func (s *PGOAuthTokenStore) GetValidAccessToken(ctx context.Context, userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", fmt.Errorf("no oauth connection found for user %q: user_id vacío", userID)
	}
	if s.pool == nil {
		return "", fmt.Errorf("no oauth connection found for user %s: sin pool de BD (STORAGE_MODE=drive requiere DATABASE_URL)", userID)
	}
	conn, err := s.getConnection(ctx, userID)
	if err != nil {
		return "", err
	}
	if conn == nil {
		log.Printf("[DRIVE DEBUG] No se encontró token OAuth para userID: %s", userID)
		return "", fmt.Errorf("no oauth connection found for user %s: conecte Google Drive primero (POST /auth/google-drive/connect)", userID)
	}
	log.Printf("[DRIVE DEBUG] Token OAuth encontrado para userID: %s. Subiendo archivo a Drive...", userID)
	if conn.RevokedAt != nil {
		log.Printf("drive: conexión revocada para user %s (revoked_at=%v)", userID, conn.RevokedAt)
		return "", &DriveError{Code: 403, Message: "Nota no disponible en almacenamiento remoto"}
	}
	if strings.TrimSpace(conn.AccessToken) == "" {
		return "", &DriveError{Code: 403, Message: "Nota no disponible en almacenamiento remoto"}
	}
	// Vigente si no tiene expiración o le quedan más de 5 minutos.
	if conn.ExpiresAt == nil || time.Now().Add(5*time.Minute).Before(*conn.ExpiresAt) {
		return conn.AccessToken, nil
	}
	// Expirado: intentar refresh.
	if conn.RefreshToken == nil || strings.TrimSpace(*conn.RefreshToken) == "" {
		log.Printf("drive: token expirado sin refresh_token para user %s", userID)
		return "", fmt.Errorf("drive connection expired for user %s: sin refresh_token, reconecte Drive", userID)
	}
	if strings.TrimSpace(s.clientID) == "" || strings.TrimSpace(s.clientSecret) == "" {
		log.Printf("drive: token expirado para user %s y GOOGLE_CLIENT_ID/SECRET vacíos (refresh deshabilitado)", userID)
		return "", fmt.Errorf("drive connection expired for user %s: GOOGLE_CLIENT_ID/SECRET no configurados, reconecte Drive", userID)
	}
	refreshed, expiresAt, err := s.refresh(ctx, strings.TrimSpace(*conn.RefreshToken))
	if err != nil {
		return "", err
	}
	if err := s.updateAccessToken(ctx, userID, refreshed, expiresAt); err != nil {
		log.Printf("drive: no se pudo persistir token refrescado para user %s: %v", userID, err)
		return "", fmt.Errorf("drive: error persistiendo token refrescado para user %s: %w", userID, err)
	}
	log.Printf("drive: token refrescado OK para user %s", userID)
	return refreshed, nil
}

func (s *PGOAuthTokenStore) getConnection(ctx context.Context, userID string) (*oauthConnection, error) {
	var c oauthConnection
	err := s.pool.QueryRow(ctx,
		`SELECT access_token, refresh_token, expires_at, revoked_at
		 FROM identity.oauth_connections WHERE user_id=$1 AND provider=$2`,
		userID, ProviderGoogleDrive,
	).Scan(&c.AccessToken, &c.RefreshToken, &c.ExpiresAt, &c.RevokedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("drive: error consultando oauth_connections para user %s: %w", userID, err)
	}
	return &c, nil
}

func (s *PGOAuthTokenStore) updateAccessToken(ctx context.Context, userID, accessToken string, expiresAt time.Time) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE identity.oauth_connections
		 SET access_token=$1, expires_at=$2, updated_at=now()
		 WHERE user_id=$3 AND provider=$4`,
		accessToken, expiresAt, userID, ProviderGoogleDrive,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("no oauth connection found for user %s", userID)
	}
	return nil
}

// refresh intercambia el refresh_token por un access_token nuevo vía Google OAuth2.
func (s *PGOAuthTokenStore) refresh(ctx context.Context, refreshToken string) (string, time.Time, error) {
	endpoint := google.Endpoint
	if s.tokenURL != "" {
		endpoint.TokenURL = s.tokenURL
	}
	cfg := &oauth2.Config{
		ClientID:     s.clientID,
		ClientSecret: s.clientSecret,
		Endpoint:     endpoint,
	}
	if s.httpClient != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, s.httpClient)
	}
	tok, err := cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken}).Token()
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "invalid_grant") || strings.Contains(msg, "400") {
			return "", time.Time{}, fmt.Errorf("drive connection invalid: refresh rechazado por Google (invalid_grant), reconecte Drive: %w", err)
		}
		return "", time.Time{}, fmt.Errorf("drive: google_unavailable refrescando token: %w", err)
	}
	if tok == nil || strings.TrimSpace(tok.AccessToken) == "" {
		return "", time.Time{}, fmt.Errorf("drive: google devolvió access_token vacío al refrescar")
	}
	exp := tok.Expiry
	if exp.IsZero() {
		exp = time.Now().Add(time.Hour)
	}
	return tok.AccessToken, exp, nil
}
