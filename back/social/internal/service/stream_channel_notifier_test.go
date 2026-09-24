package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/stream"
)

type fakeStreamStore struct {
	group     sqlc.SocialGroup
	channel   sqlc.SocialStreamChannel
	getErr    error
	groupErr  error
	createErr error
	created   *sqlc.CreateStreamChannelParams
}

func (f *fakeStreamStore) GetGroupByID(ctx context.Context, id pgtype.UUID) (sqlc.SocialGroup, error) {
	if f.groupErr != nil {
		return sqlc.SocialGroup{}, f.groupErr
	}
	return f.group, nil
}

func (f *fakeStreamStore) GetChannelByGroup(ctx context.Context, groupID pgtype.UUID) (sqlc.SocialStreamChannel, error) {
	if f.getErr != nil {
		return sqlc.SocialStreamChannel{}, f.getErr
	}
	return f.channel, nil
}

func (f *fakeStreamStore) CreateChannel(ctx context.Context, arg sqlc.CreateStreamChannelParams) (sqlc.SocialStreamChannel, error) {
	if f.createErr != nil {
		return sqlc.SocialStreamChannel{}, f.createErr
	}
	f.created = &arg
	return sqlc.SocialStreamChannel{GroupID: arg.GroupID, ChannelID: arg.ChannelID}, nil
}

func streamTestGroup() (string, sqlc.SocialGroup) {
	gid := uuid.NewString()
	var g pgtype.UUID
	_ = g.Scan(gid)
	return gid, sqlc.SocialGroup{ID: g, Name: "Grupo Stream"}
}

func TestChannelIDForGroup(t *testing.T) {
	id := ChannelIDForGroup("ABC-123")
	if id != "group-abc-123" {
		t.Fatalf("channel id inesperado: %q", id)
	}
}

func TestStreamNotifier_CreaYPersiste(t *testing.T) {
	gid, group := streamTestGroup()
	cli := stream.NewMockClient()
	store := &fakeStreamStore{group: group, getErr: pgx.ErrNoRows}
	n := NewStreamChannelNotifier(store, cli)

	n.OnGroupCreated(context.Background(), gid)

	if len(cli.Calls) != 1 {
		t.Fatalf("se esperaba 1 creación de canal, got %d", len(cli.Calls))
	}
	call := cli.Calls[0]
	if call.ChannelType != "messaging" || call.ChannelID != ChannelIDForGroup(gid) || call.Name != "Grupo Stream" {
		t.Fatalf("llamada inesperada: %+v", call)
	}
	if store.created == nil || store.created.ChannelID != ChannelIDForGroup(gid) {
		t.Fatalf("no se persistió el canal: %+v", store.created)
	}
}

func TestStreamNotifier_Skips(t *testing.T) {
	gid, group := streamTestGroup()

	// group_id inválido → skip sin llamar
	cli := stream.NewMockClient()
	n := NewStreamChannelNotifier(&fakeStreamStore{group: group, getErr: pgx.ErrNoRows}, cli)
	n.OnGroupCreated(context.Background(), "no-uuid")
	if len(cli.Calls) != 0 {
		t.Fatalf("group inválido no debe crear canal, got %d", len(cli.Calls))
	}

	// ya tiene canal → skip idempotente
	cli2 := stream.NewMockClient()
	var existing sqlc.SocialStreamChannel
	existing.ChannelID = ChannelIDForGroup(gid)
	n2 := NewStreamChannelNotifier(&fakeStreamStore{group: group, channel: existing}, cli2)
	n2.OnGroupCreated(context.Background(), gid)
	if len(cli2.Calls) != 0 {
		t.Fatalf("con canal existente no debe crear, got %d", len(cli2.Calls))
	}

	// grupo inexistente → skip
	cli3 := stream.NewMockClient()
	n3 := NewStreamChannelNotifier(&fakeStreamStore{getErr: pgx.ErrNoRows, groupErr: pgx.ErrNoRows}, cli3)
	n3.OnGroupCreated(context.Background(), gid)
	if len(cli3.Calls) != 0 {
		t.Fatalf("grupo inexistente no debe crear canal, got %d", len(cli3.Calls))
	}
}

func TestStreamNotifier_ErrorStream_NoPropaga(t *testing.T) {
	gid, group := streamTestGroup()
	cli := stream.NewMockClient()
	cli.CreateErr = &stream.StreamError{Code: 500, Message: "boom"}
	store := &fakeStreamStore{group: group, getErr: pgx.ErrNoRows}
	n := NewStreamChannelNotifier(store, cli)

	// No debe panic ni propagar: solo loguea, y no persiste
	n.OnGroupCreated(context.Background(), gid)
	if store.created != nil {
		t.Fatalf("con error de Stream no debe persistir: %+v", store.created)
	}
}

func TestStreamNotifier_ErrorPersistencia_NoPropaga(t *testing.T) {
	gid, group := streamTestGroup()
	cli := stream.NewMockClient()
	store := &fakeStreamStore{group: group, getErr: pgx.ErrNoRows, createErr: errors.New("db caída")}
	n := NewStreamChannelNotifier(store, cli)

	n.OnGroupCreated(context.Background(), gid)
	if len(cli.Calls) != 1 {
		t.Fatalf("el canal igual debe intentarse crear, got %d", len(cli.Calls))
	}
}
