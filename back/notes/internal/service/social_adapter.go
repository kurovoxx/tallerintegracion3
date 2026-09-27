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

// Adaptador HTTP real de Social: implementa SocialResolver (membership, admin y
// seguidores) y GroupMemberDirectory (correos de miembros) consumiendo
// SOCIAL_SERVICE_URL.
//
// Contrato interno consumido (servicio-a-servicio, header X-Internal-Key):
//
//	GET /internal/groups/{groupID}/members        -> [{"user_id":"...","role":"admin|member"}]
//	GET /internal/groups/{groupID}/member-emails  -> ["a@x.com"] | {"emails":["a@x.com"]}
//	GET /internal/users/{userID}/followers-count  -> {"followers_count":N} | {"count":N}
//
// Degradación controlada: cada llamada lleva timeout propio (http.Client y
// context) y nunca bloquea el request más allá de ese presupuesto. Un 404 se
// interpreta como ausencia (no miembro / sin correos); 401/403/5xx y fallos de
// red se propagan como error para que la capa de servicio decida (fail-closed
// en membresía, best-effort en seguidores).
type SocialHTTPAdapter struct {
	baseURL string
	apiKey  string
	timeout time.Duration
	client  *http.Client
}

var (
	_ SocialResolver       = (*SocialHTTPAdapter)(nil)
	_ GroupMemberDirectory = (*SocialHTTPAdapter)(nil)
)

// errSocialNotFound marca un 404 del servicio Social sin filtrar detalles.
var errSocialNotFound = errors.New("social: recurso no encontrado")

// NewSocialHTTPAdapter crea el adaptador con timeout explícito (<=0 usa 5s) y
// la clave interna opcional enviada como X-Internal-Key.
func NewSocialHTTPAdapter(baseURL string, timeout time.Duration, internalAPIKey string) *SocialHTTPAdapter {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &SocialHTTPAdapter{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:  strings.TrimSpace(internalAPIKey),
		timeout: timeout,
		client:  &http.Client{Timeout: timeout},
	}
}

// NewHTTPSocialResolver construye el SocialResolver HTTP (sin clave interna).
func NewHTTPSocialResolver(baseURL string, timeout time.Duration) SocialResolver {
	return NewSocialHTTPAdapter(baseURL, timeout, "")
}

// NewHTTPSocialMemberDirectory construye el GroupMemberDirectory HTTP.
func NewHTTPSocialMemberDirectory(baseURL string, timeout time.Duration) GroupMemberDirectory {
	return NewSocialHTTPAdapter(baseURL, timeout, "")
}

type socialMember struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

func (a *SocialHTTPAdapter) IsMember(ctx context.Context, userID, groupID string) (bool, error) {
	members, err := a.listMembers(ctx, groupID)
	if err != nil {
		return false, err
	}
	for _, member := range members {
		if member.UserID == userID {
			return true, nil
		}
	}
	return false, nil
}

func (a *SocialHTTPAdapter) IsAdmin(ctx context.Context, userID, groupID string) (bool, error) {
	members, err := a.listMembers(ctx, groupID)
	if err != nil {
		return false, err
	}
	for _, member := range members {
		if member.UserID == userID {
			return strings.EqualFold(member.Role, "admin"), nil
		}
	}
	return false, nil
}

func (a *SocialHTTPAdapter) GetFollowersCount(ctx context.Context, userID string) (int, error) {
	var payload struct {
		FollowersCount *int `json:"followers_count"`
		Count          *int `json:"count"`
	}
	if err := a.getJSON(ctx, "/internal/users/"+url.PathEscape(userID)+"/followers-count", &payload); err != nil {
		return 0, err
	}
	if payload.FollowersCount != nil {
		return *payload.FollowersCount, nil
	}
	if payload.Count != nil {
		return *payload.Count, nil
	}
	return 0, nil
}

func (a *SocialHTTPAdapter) ListMemberEmails(ctx context.Context, groupID string) ([]string, error) {
	body, err := a.get(ctx, "/internal/groups/"+url.PathEscape(groupID)+"/member-emails")
	if err != nil {
		if errors.Is(err, errSocialNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var emails []string
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(body, &emails); err == nil {
		return emails, nil
	}
	var wrapper struct {
		Emails []string `json:"emails"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("social: respuesta de member-emails inválida")
	}
	return wrapper.Emails, nil
}

func (a *SocialHTTPAdapter) listMembers(ctx context.Context, groupID string) ([]socialMember, error) {
	body, err := a.get(ctx, "/internal/groups/"+url.PathEscape(groupID)+"/members")
	if err != nil {
		if errors.Is(err, errSocialNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, nil
	}
	var members []socialMember
	if err := json.Unmarshal(body, &members); err != nil {
		return nil, fmt.Errorf("social: respuesta de members inválida")
	}
	return members, nil
}

func (a *SocialHTTPAdapter) getJSON(ctx context.Context, path string, out any) error {
	body, err := a.get(ctx, path)
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("social: respuesta JSON inválida")
	}
	return nil
}

// get ejecuta la llamada con timeout propio y clasifica el resultado: 2xx
// devuelve el body, 404 devuelve errSocialNotFound y el resto error.
func (a *SocialHTTPAdapter) get(ctx context.Context, path string) ([]byte, error) {
	if a.baseURL == "" {
		return nil, fmt.Errorf("social: SOCIAL_SERVICE_URL vacía")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodGet, a.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("social: request inválido")
	}
	req.Header.Set("Accept", "application/json")
	if a.apiKey != "" {
		req.Header.Set("X-Internal-Key", a.apiKey)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("social: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("social: lectura de respuesta falló")
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, errSocialNotFound
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return body, nil
	default:
		return nil, fmt.Errorf("social: status %d", resp.StatusCode)
	}
}
