package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Errores tipificados del gateway de tokens Calendar (Auth interno).
var (
	ErrCalendarNotConnected       = errors.New("calendar not connected")
	ErrCalendarConnectionInvalid  = errors.New("calendar connection invalid")
	ErrCalendarGatewayUnavailable = errors.New("calendar gateway unavailable")
)

// CalendarTokenGateway obtiene access tokens vigentes de Google Calendar
// desde el servicio Auth (endpoint interno servicio-a-servicio).
type CalendarTokenGateway interface {
	GetToken(ctx context.Context, userID string) (string, error)
	ReportRevoked(ctx context.Context, userID string) error
}

// AuthCalendarGateway es la implementación HTTP contra Auth.
// BaseURL: ej. http://localhost:8080 (dev) o http://auth:8080 (docker).
type AuthCalendarGateway struct {
	BaseURL     string
	InternalKey string
	HTTPClient  *http.Client
}

func (g *AuthCalendarGateway) http() *http.Client {
	if g.HTTPClient != nil {
		return g.HTTPClient
	}
	return &http.Client{Timeout: 8 * time.Second}
}

func (g *AuthCalendarGateway) GetToken(ctx context.Context, userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", ErrCalendarNotConnected
	}
	base := strings.TrimSuffix(strings.TrimSpace(g.BaseURL), "/")
	if base == "" {
		return "", ErrCalendarGatewayUnavailable
	}
	u := base + "/internal/oauth/calendar-token?user_id=" + url.QueryEscape(userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", ErrCalendarGatewayUnavailable
	}
	if g.InternalKey != "" {
		req.Header.Set("X-Internal-Key", g.InternalKey)
	}
	resp, err := g.http().Do(req)
	if err != nil {
		return "", ErrCalendarGatewayUnavailable
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	switch resp.StatusCode {
	case http.StatusOK:
		var data struct {
			AccessToken string `json:"access_token"`
		}
		if err := json.Unmarshal(body, &data); err != nil || strings.TrimSpace(data.AccessToken) == "" {
			return "", ErrCalendarGatewayUnavailable
		}
		return data.AccessToken, nil
	case http.StatusNotFound:
		return "", ErrCalendarNotConnected
	case http.StatusForbidden:
		return "", ErrCalendarConnectionInvalid
	default:
		return "", fmt.Errorf("%w: status %d", ErrCalendarGatewayUnavailable, resp.StatusCode)
	}
}

// ReportRevoked avisa a Auth que Google rechazó el token (401/403) para que marque revoked_at.
// Best-effort: un fallo aquí solo se propaga como error para log, nunca afecta la reunión.
func (g *AuthCalendarGateway) ReportRevoked(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	base := strings.TrimSuffix(strings.TrimSpace(g.BaseURL), "/")
	if base == "" {
		return ErrCalendarGatewayUnavailable
	}
	payload := fmt.Sprintf(`{"user_id":%q}`, userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/internal/oauth/calendar-revoked", strings.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if g.InternalKey != "" {
		req.Header.Set("X-Internal-Key", g.InternalKey)
	}
	resp, err := g.http().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	return nil
}

// StubCalendarGateway es el gateway de mentira para CALENDAR_MODE=mock y tests:
// siempre entrega un token dummy sin llamar a Auth.
type StubCalendarGateway struct {
	Token          string
	RevokedReports []string
}

func (s *StubCalendarGateway) GetToken(ctx context.Context, userID string) (string, error) {
	if strings.TrimSpace(userID) == "" {
		return "", ErrCalendarNotConnected
	}
	if s.Token != "" {
		return s.Token, nil
	}
	return "mock-token", nil
}

func (s *StubCalendarGateway) ReportRevoked(ctx context.Context, userID string) error {
	s.RevokedReports = append(s.RevokedReports, userID)
	return nil
}

var (
	_ CalendarTokenGateway = (*AuthCalendarGateway)(nil)
	_ CalendarTokenGateway = (*StubCalendarGateway)(nil)
)
