package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
)

// Credenciales de Discord OAuth2 para conexión automática (tarea 3_5_18).
var (
	discordClientID     string
	discordClientSecret string
	discordRedirectURI  string
)

// SetDiscordOAuthConfig configura las credenciales de Discord OAuth2 para el servicio.
// Se llama desde main.go al iniciar el servidor.
func SetDiscordOAuthConfig(clientID, clientSecret, redirectURI string) {
	discordClientID = clientID
	discordClientSecret = clientSecret
	discordRedirectURI = redirectURI
	log.Printf("Discord OAuth (service): configurado con client_id=%s redirect_uri=%s", clientID, redirectURI)
}

var (
	ErrInvalidWebhookURL   = errors.New("invalid webhook_url: must be a https://discord.com/api/webhooks/... URL")
	ErrServerNameEmpty     = errors.New("server_name cannot be empty")
	ErrInviteURLEmpty      = errors.New("invite_url cannot be empty")
	ErrDiscordNotConfigured = errors.New("discord not configured for this group")
)

// Constantes de Discord OAuth2 y API.
const (
	discordAPIBase   = "https://discord.com/api/v10"
	discordOAuthURL  = "https://discord.com/api/oauth2/token"
)

// OAuthTokenResponse es la respuesta de intercambio de código por token.
type discordTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

// discordGuild es un servidor de Discord.
type discordGuild struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// discordChannel es un canal de Discord.
type discordChannel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // "text", "voice", etc.
}

// discordWebhook es un webhook de Discord.
type discordWebhook struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Token string `json:"token"`
	URL   string `json:"url"`
}

// Hosts válidos de webhooks de Discord (prod + clientes de prueba).
var validWebhookHosts = map[string]struct{}{
	"discord.com":        {},
	"ptb.discord.com":    {},
	"canary.discord.com": {},
}

type DiscordService struct {
	repo DiscordRepo
}

// DiscordRepo es la porción de persistencia que usa DiscordService.
// *repository.DiscordRepository es la implementación real (Postgres);
// MemoryMeetingStore, la de tests.
type DiscordRepo interface {
	GetGroupByID(ctx context.Context, id pgtype.UUID) (sqlc.SocialGroup, error)
	GetMemberRole(ctx context.Context, groupID, userID pgtype.UUID) (string, error)
	UpsertConfig(ctx context.Context, arg sqlc.UpsertDiscordConfigParams) (sqlc.SocialDiscordIntegration, error)
	GetConfigByGroup(ctx context.Context, groupID pgtype.UUID) (sqlc.SocialDiscordIntegration, error)
}

func NewDiscordService(repo DiscordRepo) *DiscordService {
	return &DiscordService{repo: repo}
}

func validateServerName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrServerNameEmpty
	}
	if len([]rune(name)) > 200 {
		return "", ErrServerNameEmpty
	}
	return name, nil
}

func validateInviteURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrInviteURLEmpty
	}
	if len(raw) > 500 {
		return "", ErrInviteURLEmpty
	}
	return raw, nil
}

// validateWebhookURL acepta vacío (NULL = sin notificaciones) o un webhook https de Discord.
func validateWebhookURL(raw string) (pgtype.Text, error) {
	var out pgtype.Text
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out, nil
	}
	if len(raw) > 500 {
		return out, ErrInvalidWebhookURL
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return out, ErrInvalidWebhookURL
	}
	if _, ok := validWebhookHosts[strings.ToLower(u.Hostname())]; !ok {
		return out, ErrInvalidWebhookURL
	}
	if !strings.HasPrefix(u.Path, "/api/webhooks/") {
		return out, ErrInvalidWebhookURL
	}
	if err := out.Scan(raw); err != nil {
		return out, ErrInvalidWebhookURL
	}
	return out, nil
}

func textOrNull(s string) pgtype.Text {
	var t pgtype.Text
	s = strings.TrimSpace(s)
	if s == "" {
		return t
	}
	_ = t.Scan(s)
	return t
}

// PutConfig crea o reemplaza la config de Discord del grupo. Solo admin.
func (s *DiscordService) PutConfig(ctx context.Context, groupID, userID, serverName, inviteURL, webhookURL string) (sqlc.SocialDiscordIntegration, error) {
	gid, err := parseGroupUUID(groupID)
	if err != nil {
		return sqlc.SocialDiscordIntegration{}, err
	}
	uid, err := parseMeetingUserUUID(userID)
	if err != nil {
		return sqlc.SocialDiscordIntegration{}, err
	}
	name, err := validateServerName(serverName)
	if err != nil {
		return sqlc.SocialDiscordIntegration{}, err
	}
	invite, err := validateInviteURL(inviteURL)
	if err != nil {
		return sqlc.SocialDiscordIntegration{}, err
	}
	webhook, err := validateWebhookURL(webhookURL)
	if err != nil {
		return sqlc.SocialDiscordIntegration{}, err
	}

	if _, err := s.repo.GetGroupByID(ctx, gid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlc.SocialDiscordIntegration{}, ErrGroupNotFound
		}
		return sqlc.SocialDiscordIntegration{}, err
	}
	role, err := s.repo.GetMemberRole(ctx, gid, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlc.SocialDiscordIntegration{}, ErrForbidden
		}
		return sqlc.SocialDiscordIntegration{}, err
	}
	if role != "admin" {
		return sqlc.SocialDiscordIntegration{}, ErrForbidden
	}

	return s.repo.UpsertConfig(ctx, sqlc.UpsertDiscordConfigParams{
		GroupID:    gid,
		ServerName: textOrNull(name),
		InviteUrl:  textOrNull(invite),
		WebhookUrl: webhook,
	})
}

// GetConfig devuelve la integración de Discord del grupo para cualquier
// miembro (lectura). 404 discord_not_configured si nunca se configuró.
func (s *DiscordService) GetConfig(ctx context.Context, groupID, userID string) (sqlc.SocialDiscordIntegration, error) {
	gid, err := parseGroupUUID(groupID)
	if err != nil {
		return sqlc.SocialDiscordIntegration{}, err
	}
	uid, err := parseMeetingUserUUID(userID)
	if err != nil {
		return sqlc.SocialDiscordIntegration{}, err
	}
	if _, err := s.repo.GetGroupByID(ctx, gid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlc.SocialDiscordIntegration{}, ErrGroupNotFound
		}
		return sqlc.SocialDiscordIntegration{}, err
	}
	if _, err := s.repo.GetMemberRole(ctx, gid, uid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlc.SocialDiscordIntegration{}, ErrForbidden
		}
		return sqlc.SocialDiscordIntegration{}, err
	}
	cfg, err := s.repo.GetConfigByGroup(ctx, gid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlc.SocialDiscordIntegration{}, ErrDiscordNotConfigured
		}
		return sqlc.SocialDiscordIntegration{}, err
	}
	return cfg, nil
}

// ConnectDiscord completa el flujo OAuth2 de Discord:
// 1. Intercambia el código de autorización por un access token
// 2. Obtiene los servidores del usuario
// 3. Obtiene los canales del servidor
// 4. Crea un webhook en el primer canal de texto
// 5. Guarda la configuración en la base de datos
func (s *DiscordService) ConnectDiscord(ctx context.Context, groupID, userID, code string) (sqlc.SocialDiscordIntegration, error) {
	// 1. Intercambiar código por access token
	token, err := s.exchangeCodeForToken(ctx, code)
	if err != nil {
		return sqlc.SocialDiscordIntegration{}, fmt.Errorf("exchange code for token: %w", err)
	}

	// 2. Obtener servidores del usuario
	guilds, err := s.getUserGuilds(ctx, token.AccessToken)
	if err != nil {
		return sqlc.SocialDiscordIntegration{}, fmt.Errorf("fetch guilds: %w", err)
	}
	if len(guilds) == 0 {
		return sqlc.SocialDiscordIntegration{}, fmt.Errorf("no Discord servers found for user")
	}

	// Seleccionar el primer servidor
	guild := guilds[0]

	// 3. Obtener canales del servidor
	channels, err := s.getGuildChannels(ctx, token.AccessToken, guild.ID)
	if err != nil {
		return sqlc.SocialDiscordIntegration{}, fmt.Errorf("fetch channels: %w", err)
	}

	// Buscar el primer canal de texto
	var textChannelID string
	for _, ch := range channels {
		if ch.Type == "text" {
			textChannelID = ch.ID
			break
		}
	}
	if textChannelID == "" {
		return sqlc.SocialDiscordIntegration{}, fmt.Errorf("no text channel found in guild %s", guild.ID)
	}

	// 4. Crear webhook en el canal de texto
	webhook, err := s.createWebhook(ctx, token.AccessToken, textChannelID, "Taller Integración")
	if err != nil {
		return sqlc.SocialDiscordIntegration{}, fmt.Errorf("create webhook: %w", err)
	}

	// 5. Construir invite_url (placeholder hasta que el usuario lo configure)
	inviteURL := "https://discord.gg/" + guild.ID

	// 6. Guardar configuración
	gid, err := parseGroupUUID(groupID)
	if err != nil {
		return sqlc.SocialDiscordIntegration{}, err
	}
	if _, err := parseMeetingUserUUID(userID); err != nil {
		return sqlc.SocialDiscordIntegration{}, err
	}

	return s.repo.UpsertConfig(ctx, sqlc.UpsertDiscordConfigParams{
		GroupID:    gid,
		ServerName: textOrNull(guild.Name),
		InviteUrl:  textOrNull(inviteURL),
		WebhookUrl: textOrNull(webhook.URL),
	})
}

// exchangeCodeForToken intercambia el código de autorización por un access token.
func (s *DiscordService) exchangeCodeForToken(ctx context.Context, code string) (*discordTokenResponse, error) {
	data := url.Values{}
	data.Set("client_id", discordClientID)
	data.Set("client_secret", discordClientSecret)
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", discordRedirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, discordOAuthURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("discord token exchange failed: %s (status %d)", string(body), resp.StatusCode)
	}

	var tokenResp discordTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}
	return &tokenResp, nil
}

// getUserGuilds obtiene los servidores de Discord del usuario autenticado.
func (s *DiscordService) getUserGuilds(ctx context.Context, accessToken string) ([]discordGuild, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discordAPIBase+"/users/@me/guilds", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch guilds failed: %s (status %d)", string(body), resp.StatusCode)
	}

	var guilds []discordGuild
	if err := json.NewDecoder(resp.Body).Decode(&guilds); err != nil {
		return nil, err
	}
	return guilds, nil
}

// getGuildChannels obtiene los canales de un servidor de Discord.
func (s *DiscordService) getGuildChannels(ctx context.Context, accessToken, guildID string) ([]discordChannel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discordAPIBase+"/guilds/"+guildID+"/channels", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch channels failed: %s (status %d)", string(body), resp.StatusCode)
	}

	var channels []discordChannel
	if err := json.NewDecoder(resp.Body).Decode(&channels); err != nil {
		return nil, err
	}
	return channels, nil
}

// createWebhook crea un webhook en un canal de Discord.
func (s *DiscordService) createWebhook(ctx context.Context, accessToken, channelID, name string) (*discordWebhook, error) {
	body := map[string]string{"name": name}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, discordAPIBase+"/channels/"+channelID+"/webhooks", strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create webhook failed: %s (status %d)", string(body), resp.StatusCode)
	}

	var webhook discordWebhook
	if err := json.NewDecoder(resp.Body).Decode(&webhook); err != nil {
		return nil, err
	}
	return &webhook, nil
}
