package model

import "time"

// OAuthConnection representa identity.oauth_connections.
type OAuthConnection struct {
	ID                   string    `json:"id"`
	UserID               string    `json:"user_id"`
	Provider             string    `json:"provider"`
	AccessToken          string    `json:"-"`
	RefreshToken         *string   `json:"-"`
	ExpiresAt            *time.Time `json:"expires_at"`
	ExternalAccountEmail *string   `json:"external_account_email"`
	RevokedAt            *time.Time `json:"revoked_at"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

const (
	ProviderGoogleDrive    = "google_drive"
	ProviderGoogleCalendar = "google_calendar"
)
