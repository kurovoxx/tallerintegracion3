package service

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/stream"
)

var ErrStreamNotConfigured = errors.New("stream no configurado: falta STREAM_API_SECRET")

// ErrStreamSyncFailed indica que el usuario pertenece al grupo pero Stream no
// pudo dejarlo como miembro del canal (el handler lo traduce a 503 para que
// el front muestre estado humano en vez de un 403 críptico del SDK).
var ErrStreamSyncFailed = errors.New("stream no pudo sincronizar la membresía del canal")

// StreamTokenRepo es la porción de persistencia que usa StreamTokenService.
// *repository.StreamRepository es la implementación real.
type StreamTokenRepo interface {
	GetGroupByID(ctx context.Context, id pgtype.UUID) (sqlc.SocialGroup, error)
	GetMemberRole(ctx context.Context, groupID, userID pgtype.UUID) (string, error)
	GetChannelByGroup(ctx context.Context, groupID pgtype.UUID) (sqlc.SocialStreamChannel, error)
}

// StreamTokenService emite tokens de usuario Stream para que el front se
// conecte directo al chat. Solo miembros; firma local HS256 y sincroniza la
// membresía del canal en Stream (server-side) antes de entregar el token.
type StreamTokenService struct {
	repo      StreamTokenRepo
	apiSecret string
	apiKey    string
	// client sincroniza canal/miembro en Stream. Nil = solo firma (tests
	// unitarios y compatibilidad); en producción main.go siempre lo inyecta.
	client stream.Client
}

func NewStreamTokenService(repo StreamTokenRepo, apiSecret string) *StreamTokenService {
	return &StreamTokenService{repo: repo, apiSecret: apiSecret}
}

// NewStreamTokenServiceWithKey igual, más la API key pública que el cliente
// necesita para conectarse (las keys de Stream son públicas por diseño;
// el secret jamás sale del backend).
func NewStreamTokenServiceWithKey(repo StreamTokenRepo, apiSecret, apiKey string) *StreamTokenService {
	return &StreamTokenService{repo: repo, apiSecret: apiSecret, apiKey: strings.TrimSpace(apiKey)}
}

// SetClient inyecta el cliente Stream para la sincronización de membresía
// (MockClient en modo mock/test, RESTClient en modo real).
func (s *StreamTokenService) SetClient(c stream.Client) {
	s.client = c
}

// IssueToken verifica grupo + membresía y firma el token para channel_id.
// Si el grupo no tiene fila en stream_channels (grupos viejos), usa el id
// determinístico para no dejar el chat inalcanzable.
func (s *StreamTokenService) IssueToken(ctx context.Context, groupID, userID string) (token, channelID string, err error) {
	token, channelID, _, err = s.IssueTokenWithKey(ctx, groupID, userID)
	return token, channelID, err
}

// IssueTokenWithKey además devuelve la API key pública para el cliente.
func (s *StreamTokenService) IssueTokenWithKey(ctx context.Context, groupID, userID string) (token, channelID, apiKey string, err error) {
	gid, err := parseGroupUUID(groupID)
	if err != nil {
		return "", "", "", err
	}
	uid, err := parseMeetingUserUUID(userID)
	if err != nil {
		return "", "", "", err
	}
	if strings.TrimSpace(s.apiSecret) == "" {
		return "", "", "", ErrStreamNotConfigured
	}

	group, err := s.repo.GetGroupByID(ctx, gid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", "", ErrGroupNotFound
		}
		return "", "", "", err
	}
	// Fuente de verdad Sigma: solo miembros del grupo siguen adelante.
	// Un ajeno recibe 403 y nunca se toca Stream.
	if _, err := s.repo.GetMemberRole(ctx, gid, uid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", "", ErrForbidden
		}
		return "", "", "", err
	}

	channelID = ChannelIDForGroup(groupID)
	if ch, err := s.repo.GetChannelByGroup(ctx, gid); err == nil && strings.TrimSpace(ch.ChannelID) != "" {
		channelID = ch.ChannelID
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", err
	}

	// El usuario pertenece al grupo: garantizar (server-side, idempotente)
	// que también es miembro del canal Stream antes de darle el token.
	// Sin esto, el SDK falla con 403 code 17 al hacer query del canal.
	if s.client != nil {
		if err := s.ensureChannelMember(ctx, channelID, group.Name, userID); err != nil {
			return "", "", "", err
		}
	}

	token, err = stream.SignUserToken(s.apiSecret, userID)
	if err != nil {
		return "", "", "", err
	}
	return token, channelID, s.apiKey, nil
}

// ensureChannelMember deja el canal listo y al usuario como miembro.
// CreateChannel es get-or-create (no toca canales existentes) y agregar un
// miembro existente también es idempotente: pedir el token N veces no
// duplica, no borra mensajes ni rompe nada.
func (s *StreamTokenService) ensureChannelMember(ctx context.Context, channelID, groupName, userID string) error {
	if err := s.client.CreateChannel(ctx, ChannelTypeStream, channelID, groupName, userID); err != nil {
		logChannelSync("create", channelID, err)
		return errors.Join(ErrStreamSyncFailed, err)
	}
	if err := s.client.EnsureMember(ctx, ChannelTypeStream, channelID, userID); err != nil {
		logChannelSync("ensure-member", channelID, err)
		return errors.Join(ErrStreamSyncFailed, err)
	}
	return nil
}

// logChannelSync deja rastro server-side sanitizado: operación, canal y
// status de Stream. Jamás tokens, secrets ni Authorization.
func logChannelSync(op, channelID string, err error) {
	code := 0
	var se *stream.StreamError
	if errors.As(err, &se) {
		code = se.Code
	}
	log.Printf("stream sync: %s canal %s falló (stream status %d)", op, channelID, code)
}
