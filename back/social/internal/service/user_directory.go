package service

import (
	"context"
	"encoding/json"
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
		return out, nil
	}
	if strings.TrimSpace(d.key) != "" {
		req.Header.Set("X-Internal-Key", strings.TrimSpace(d.key))
	}
	resp, err := d.client.Do(req)
	if err != nil {
		log.Printf("social: user directory no disponible: %v", err)
		return out, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("social: user directory status=%d", resp.StatusCode)
		return out, nil
	}
	var list []UserPublic
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		log.Printf("social: user directory decode: %v", err)
		return out, nil
	}
	for _, p := range list {
		if strings.TrimSpace(p.UserID) != "" {
			out[p.UserID] = p
		}
	}
	return out, nil
}
