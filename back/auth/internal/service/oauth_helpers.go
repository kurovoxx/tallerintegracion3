package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// NetHTTPClient es un alias para *http.Client que permite inyección en tests.
// Se usa en providers configurables (httptest.Server).
type NetHTTPClient = http.Client

func withHTTPClient(ctx context.Context, client *http.Client) context.Context {
	if client != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, client)
	}
	return ctx
}

func toStdClient(client *http.Client) *http.Client {
	if client == nil {
		return http.DefaultClient
	}
	return client
}

// mapExchangeError mapea errores del grant authorization_code:
// invalid_grant/400 -> ErrInvalidOAuthCode, 429/5xx/timeout -> ErrGoogleUnavailable.
func mapExchangeError(err error) error {
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "invalid_grant") || strings.Contains(msg, "invalid_request") || strings.Contains(msg, "400") {
		return ErrInvalidOAuthCode
	}
	var retrieveErr *oauth2.RetrieveError
	if errors.As(err, &retrieveErr) {
		if retrieveErr.Response != nil {
			code := retrieveErr.Response.StatusCode
			if code == 400 {
				return ErrInvalidOAuthCode
			}
			if code == 429 || code >= 500 {
				return ErrGoogleUnavailable
			}
		}
	}
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "connection") || strings.Contains(msg, "429") || strings.Contains(msg, "500") || strings.Contains(msg, "502") || strings.Contains(msg, "503") {
		return ErrGoogleUnavailable
	}
	return ErrInvalidOAuthCode
}

// mapRefreshError mapea errores del grant refresh_token.
// Mismo criterio que mapExchangeError, pero el defecto es conexión inválida
// (el refresh_token ya no sirve) en vez de código inválido.
func mapRefreshError(err error) error {
	mapped := mapExchangeError(err)
	if mapped == ErrInvalidOAuthCode {
		return ErrCalendarConnectionInvalid
	}
	return mapped
}

func strPtrOrNil(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	v := s
	return &v
}

func timePtrOrNil(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	v := t
	return &v
}
