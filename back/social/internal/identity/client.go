package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

// ErrInvalidToken indica que Identity rechazó el token (401).
var ErrInvalidToken = errors.New("invalid token")

// ErrUserNotFound indica que el usuario no existe en Identity (404).
var ErrUserNotFound = errors.New("user not found")

// Client consume los endpoints internos de Identity (tarea 3_5_6:
// POST /internal/auth/verify, GET /internal/users/:id) con X-Internal-Key.
// Copia local a propósito: los microservicios no dependen entre sí.
// Fail-closed: cualquier fallo de red o status inesperado es error,
// nunca éxito silencioso.
type Client struct {
	baseURL string
	key     string
	http    *http.Client
}

func NewClient(baseURL, internalKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		key:     strings.TrimSpace(internalKey),
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *Client) req(ctx context.Context, method, path string, body any) (*http.Response, error) {
	if strings.TrimSpace(c.baseURL) == "" {
		return nil, fmt.Errorf("identity: baseURL vacía")
	}
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, &buf)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.key != "" {
		req.Header.Set("X-Internal-Key", c.key)
	}
	return c.http.Do(req)
}

// VerifyToken valida un JWT de usuario contra Identity.
// Devuelve el user_id si es válido; ErrInvalidToken si Identity lo rechaza.
func (c *Client) VerifyToken(ctx context.Context, token string) (string, error) {
	if strings.TrimSpace(token) == "" {
		return "", ErrInvalidToken
	}
	resp, err := c.req(ctx, http.MethodPost, "/internal/auth/verify", map[string]string{"token": token})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return "", ErrInvalidToken
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("identity: verify status=%d", resp.StatusCode)
	}
	var out struct {
		Valid  bool   `json:"valid"`
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if !out.Valid || strings.TrimSpace(out.UserID) == "" {
		return "", ErrInvalidToken
	}
	return out.UserID, nil
}

// GetUser resuelve un usuario por id. Devuelve ErrUserNotFound si no existe.
func (c *Client) GetUser(ctx context.Context, id string) (*service.UserPublic, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrUserNotFound
	}
	resp, err := c.req(ctx, http.MethodGet, "/internal/users/"+url.PathEscape(strings.TrimSpace(id)), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrUserNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("identity: get user status=%d", resp.StatusCode)
	}
	var out service.UserPublic
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}
