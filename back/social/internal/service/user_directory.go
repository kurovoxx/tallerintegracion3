package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// UserPublic son los datos mínimos no sensibles de un usuario resueltos
// vía Identity (ver back/auth GET /internal/users/lookup).
type UserPublic struct {
	UserID      string  `json:"user_id"`
	Email       string  `json:"email"`
	DisplayName *string `json:"display_name,omitempty"`
}

// UserDirectory resuelve datos públicos de usuarios para enriquecer
// respuestas (p. ej. nombres en GET /groups/{id}/members).
// La implementación HTTP es best-effort: ante cualquier fallo devuelve
// mapa vacío sin error para no romper la operación principal.
type UserDirectory interface {
	LookupUsers(ctx context.Context, userIDs []string) (map[string]UserPublic, error)
	// LookupEmailsStrict es la variante fail-closed para los endpoints
	// internos (Notes → Social): cualquier fallo es error, para que Notes
	// no calcule permisos sobre una lista incompleta.
	LookupEmailsStrict(ctx context.Context, userIDs []string) ([]string, error)
}

// HTTPUserDirectory consulta Auth servicio-a-servicio con X-Internal-Key.
type HTTPUserDirectory struct {
	baseURL string
	key     string
	client  *http.Client
}

func NewHTTPUserDirectory(baseURL, internalKey string) *HTTPUserDirectory {
	return &HTTPUserDirectory{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		key:     internalKey,
		client:  &http.Client{Timeout: 3 * time.Second},
	}
}

func (d *HTTPUserDirectory) LookupUsers(ctx context.Context, userIDs []string) (map[string]UserPublic, error) {
	out, err := d.lookup(ctx, userIDs)
	if err != nil {
		log.Printf("social: user directory no disponible: %v", err)
		return map[string]UserPublic{}, nil
	}
	if out == nil {
		out = map[string]UserPublic{}
	}
	return out, nil
}

// LookupEmailsStrict resuelve emails sin enmascarar fallos: error de red,
// status != 200 o JSON inválido es error (el llamante interno decide).
func (d *HTTPUserDirectory) LookupEmailsStrict(ctx context.Context, userIDs []string) ([]string, error) {
	got, err := d.lookup(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	emails := []string{}
	for _, id := range userIDs {
		u, ok := got[strings.TrimSpace(id)]
		if !ok {
			continue
		}
		email := strings.ToLower(strings.TrimSpace(u.Email))
		if email == "" {
			continue
		}
		if _, dup := seen[email]; dup {
			continue
		}
		seen[email] = struct{}{}
		emails = append(emails, email)
	}
	if emails == nil {
		emails = []string{}
	}
	return emails, nil
}

func (d *HTTPUserDirectory) lookup(ctx context.Context, userIDs []string) (map[string]UserPublic, error) {
	out := map[string]UserPublic{}
	if d == nil || strings.TrimSpace(d.baseURL) == "" || len(userIDs) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(userIDs))
	for _, id := range userIDs {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	u := d.baseURL + "/internal/users/lookup?ids=" + url.QueryEscape(strings.Join(ids, ","))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(d.key) != "" {
		req.Header.Set("X-Internal-Key", strings.TrimSpace(d.key))
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("social: user directory status=%d", resp.StatusCode)
	}
	var list []UserPublic
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}
	for _, p := range list {
		if strings.TrimSpace(p.UserID) != "" {
			out[p.UserID] = p
		}
	}
	return out, nil
}
