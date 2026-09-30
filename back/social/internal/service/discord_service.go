package service

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
)

var (
	ErrInvalidWebhookURL   = errors.New("invalid webhook_url: must be a https://discord.com/api/webhooks/... URL")
	ErrServerNameEmpty     = errors.New("server_name cannot be empty")
	ErrInviteURLEmpty      = errors.New("invite_url cannot be empty")
	ErrDiscordNotConfigured = errors.New("discord not configured for this group")
)

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
