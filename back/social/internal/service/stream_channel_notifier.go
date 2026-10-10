package service

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/stream"
)

// StreamChannelStore es lo mínimo que el notifier necesita: leer el grupo y su canal.
type StreamChannelStore interface {
	GetGroupByID(ctx context.Context, id pgtype.UUID) (sqlc.SocialGroup, error)
	GetChannelByGroup(ctx context.Context, groupID pgtype.UUID) (sqlc.SocialStreamChannel, error)
	CreateChannel(ctx context.Context, arg sqlc.CreateStreamChannelParams) (sqlc.SocialStreamChannel, error)
}

// ChannelTypeStream es el tipo de canal grupal en Stream.
const ChannelTypeStream = "messaging"

// ChannelIDForGroup deriva un channel_id determinístico y válido para Stream.
// Exportada para tests y para el futuro endpoint de stream-token.
func ChannelIDForGroup(groupID string) string {
	return "group-" + strings.ToLower(strings.TrimSpace(groupID))
}

// StreamChannelNotifier implementa GroupCreatedNotifier creando el canal de
// chat del grupo en Stream. Todo best-effort, en background:
//  1. Si el grupo ya tiene canal registrado → skip (idempotente).
//  2. Si el grupo no existe → skip.
//  3. Si Stream falla o la persistencia falla → solo log.
//
// Un fallo aquí jamás revierte el grupo ya creado.
// Nota: no publica mensajes (eso es otra tarea: anuncio de reuniones por Stream).
type StreamChannelNotifier struct {
	store  StreamChannelStore
	client stream.Client
}

func NewStreamChannelNotifier(store StreamChannelStore, client stream.Client) *StreamChannelNotifier {
	return &StreamChannelNotifier{store: store, client: client}
}

func (n *StreamChannelNotifier) OnGroupCreated(ctx context.Context, groupID string) {
	groupID = strings.TrimSpace(groupID)
	if _, err := uuid.Parse(groupID); err != nil {
		log.Printf("stream sync: group_id inválido, skip")
		return
	}
	var gid pgtype.UUID
	if err := gid.Scan(groupID); err != nil {
		log.Printf("stream sync: group_id inválido, skip")
		return
	}

	if _, err := n.store.GetChannelByGroup(ctx, gid); err == nil {
		return // ya tiene canal: idempotente, sin log ruidoso
	} else if !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("stream sync: no se pudo leer canal del grupo: %v", err)
		return
	}

	group, err := n.store.GetGroupByID(ctx, gid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Printf("stream sync: grupo inexistente, skip")
			return
		}
		log.Printf("stream sync: no se pudo leer el grupo: %v", err)
		return
	}

	channelID := ChannelIDForGroup(groupID)
	ownerID := ""
	if group.OwnerUserID.Valid {
		if parsed, err := uuid.FromBytes(group.OwnerUserID.Bytes[:]); err == nil {
			ownerID = parsed.String()
		}
	}
	if err := n.client.CreateChannel(ctx, ChannelTypeStream, channelID, group.Name, ownerID); err != nil {
		log.Printf("stream sync: no se pudo crear canal: %v", err)
		return
	}
	if _, err := n.store.CreateChannel(ctx, sqlc.CreateStreamChannelParams{
		GroupID:   gid,
		ChannelID: channelID,
	}); err != nil {
		log.Printf("stream sync: canal %s creado pero no se pudo persistir: %v", channelID, err)
		return
	}
	log.Printf("stream sync: canal %s creado para el grupo", channelID)
}
