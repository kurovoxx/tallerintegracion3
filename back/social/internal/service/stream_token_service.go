package service

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/stream"
)

var ErrStreamNotConfigured = errors.New("stream no configurado: falta STREAM_API_SECRET")

// StreamTokenRepo es la porción de persistencia que usa StreamTokenService.
// *repository.StreamRepository es la implementación real.
type StreamTokenRepo interface {
	GetGroupByID(ctx context.Context, id pgtype.UUID) (sqlc.SocialGroup, error)
	GetMemberRole(ctx context.Context, groupID, userID pgtype.UUID) (string, error)
	GetChannelByGroup(ctx context.Context, groupID pgtype.UUID) (sqlc.SocialStreamChannel, error)
}

// StreamTokenService emite tokens de usuario Stream para que el front se
// conecte directo al chat. Solo miembros; firma local HS256, sin llamar a Stream.
type StreamTokenService struct {
	repo      StreamTokenRepo
	apiSecret string
}

func NewStreamTokenService(repo StreamTokenRepo, apiSecret string) *StreamTokenService {
	return &StreamTokenService{repo: repo, apiSecret: apiSecret}
}

// IssueToken verifica grupo + membresía y firma el token para channel_id.
// Si el grupo no tiene fila en stream_channels (grupos viejos), usa el id
// determinístico para no dejar el chat inalcanzable.
func (s *StreamTokenService) IssueToken(ctx context.Context, groupID, userID string) (token, channelID string, err error) {
	gid, err := parseGroupUUID(groupID)
	if err != nil {
		return "", "", err
	}
	uid, err := parseMeetingUserUUID(userID)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(s.apiSecret) == "" {
		return "", "", ErrStreamNotConfigured
	}

	if _, err := s.repo.GetGroupByID(ctx, gid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", ErrGroupNotFound
		}
		return "", "", err
	}
	if _, err := s.repo.GetMemberRole(ctx, gid, uid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", ErrForbidden
		}
		return "", "", err
	}

	channelID = ChannelIDForGroup(groupID)
	if ch, err := s.repo.GetChannelByGroup(ctx, gid); err == nil && strings.TrimSpace(ch.ChannelID) != "" {
		channelID = ch.ChannelID
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", "", err
	}

	token, err = stream.SignUserToken(s.apiSecret, userID)
	if err != nil {
		return "", "", err
	}
	return token, channelID, nil
}
