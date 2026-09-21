package drive

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
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
	// refreshLocks evita refresh concurrente duplicado: dos requests simultáneas
	// con el mismo refresh_token expirado comparten un mutex por usuario, de modo
	// que solo una llama a Google y la otra reutiliza el token ya refrescado.
	refreshLocks sync.Map // userID -> *sync.Mutex
}

func (s *PGOAuthTokenStore) lockFor(userID string) *sync.Mutex {
	v, _ := s.refreshLocks.LoadOrStore(userID, &sync.Mutex{})
	m, _ := v.(*sync.Mutex)
	return m
}

// NewPGOAuthTokenStore crea el store. clientID/clientSecret pueden venir vacíos:
// en ese caso el refresh queda deshabilitado y se retorna error claro al expirar.
func NewPGOAuthTokenStore(pool *pgxpool.Pool, clientID, clientSecret string) *PGOAuthTokenStore {
	return &PGOAuthTokenStore{pool: pool, clientID: clientID, clientSecret: clientSecret}
}

// GetValidAccessToken devuelve el access_token vigente o un error tipado:
//   - *OAuthError (403) si no hay conexión, fue revocada, expiró sin refresh
//     posible o el refresh fue rechazado (invalid_grant). El Message es
//     genérico ("Conecte o renueve su Google Drive") para no filtrar texto
//     interno al frontend; el detalle se registra vía log.
//   - *DriveError 500 si Google no está disponible o la BD falla.
//   - error de reconexión si expiró y no se puede refrescar.
func (s *PGOAuthTokenStore) GetValidAccessToken(ctx context.Context, userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", &OAuthError{Message: "Conecte o renueve su Google Drive"}
	}
	if s.pool == nil {
		return "", &OAuthError{Message: "Conecte o renueve su Google Drive"}
	}
	conn, err := s.getConnection(ctx, userID)
	if err != nil {
		return "", err
	}
	if conn == nil {
		return "", &OAuthError{Message: "Conecte o renueve su Google Drive"}
	}
	if conn.RevokedAt != nil {
		log.Printf("drive: conexión revocada (revoked_at=%v)", conn.RevokedAt)
		return "", &DriveError{Code: 403, Message: "Nota no disponible en almacenamiento remoto"}
	}
	if strings.TrimSpace(conn.AccessToken) == "" {
		return "", &DriveError{Code: 403, Message: "Nota no disponible en almacenamiento remoto"}
	}
	// Vigente si no tiene expiración o le quedan más de 5 minutos.
	if conn.ExpiresAt == nil || time.Now().Add(5*time.Minute).Before(*conn.ExpiresAt) {
		return conn.AccessToken, nil
	}
	// Expirado: serializar refresh por usuario para no desperdiciar cuota de
	// Google ni generar UPDATEs concurrentes sobre la misma fila.
	mu := s.lockFor(userID)
	mu.Lock()
	defer func() {
		mu.Unlock()
		// Evitar leak de memoria: eliminar la entrada una vez liberado el lock.
		// Solo borrar si el mapa aún apunta a este mismo mutex (evita borrar
		// el mutex nuevo creado por un lockFor concurrente posterior al Unlock).
		if v, ok := s.refreshLocks.Load(userID); ok && v == mu {
			s.refreshLocks.Delete(userID)
		}
	}()
	// Releer tras adquirir el lock: otra goroutine pudo haber refrescado ya.
	conn2, err := s.getConnection(ctx, userID)
	if err != nil {
		return "", err
	}
	if conn2 == nil {
		return "", &OAuthError{Message: "Conecte o renueve su Google Drive"}
	}
	if conn2.RevokedAt != nil {
		return "", &DriveError{Code: 403, Message: "Nota no disponible en almacenamiento remoto"}
	}
	if conn2.ExpiresAt == nil || time.Now().Add(5*time.Minute).Before(*conn2.ExpiresAt) {
		if strings.TrimSpace(conn2.AccessToken) == "" {
			return "", &DriveError{Code: 403, Message: "Nota no disponible en almacenamiento remoto"}
		}
		return conn2.AccessToken, nil
	}
	if conn2.RefreshToken == nil || strings.TrimSpace(*conn2.RefreshToken) == "" {
		return "", &OAuthError{Message: "Conecte o renueve su Google Drive"}
	}
	if strings.TrimSpace(s.clientID) == "" || strings.TrimSpace(s.clientSecret) == "" {
		log.Printf("drive: refresh deshabilitado (GOOGLE_CLIENT_ID/SECRET ausentes) user=%s", userID)
		return "", &OAuthError{Message: "Conecte o renueve su Google Drive"}
	}
	refreshed, expiresAt, err := s.refresh(ctx, strings.TrimSpace(*conn2.RefreshToken))
	if err != nil {
		return "", err
	}
	if err := s.updateAccessToken(ctx, userID, refreshed, expiresAt); err != nil {
		log.Printf("drive: no se pudo persistir token refrescado: %v", err)
		return "", &DriveError{Code: 500, Message: "Servicio de almacenamiento no disponible"}
	}
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
		log.Printf("drive: error consultando oauth_connections user=%s: %v", userID, err)
		return nil, &DriveError{Code: 500, Message: "Servicio de almacenamiento no disponible"}
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
		return &OAuthError{Message: "Conecte o renueve su Google Drive"}
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
			log.Printf("drive: refresh rechazado por Google (invalid_grant)")
			return "", time.Time{}, &OAuthError{Message: "invalid_grant: refresh rechazado por Google, reconecte Drive"}
		}
		log.Printf("drive: google_unavailable refrescando token: %v", err)
		return "", time.Time{}, &DriveError{Code: 500, Message: "google_unavailable refrescando token"}
	}
	if tok == nil || strings.TrimSpace(tok.AccessToken) == "" {
		return "", time.Time{}, &DriveError{Code: 500, Message: "Servicio de almacenamiento no disponible"}
	}
	exp := tok.Expiry
	if exp.IsZero() {
		exp = time.Now().Add(time.Hour)
	}
	return tok.AccessToken, exp, nil
}
