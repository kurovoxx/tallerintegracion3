package service

import (
	"context"
	"errors"
	"testing"
	"time"

	notesv1 "github.com/kurovoxx/tallerintegracion3/back/social/internal/gen/notesv1"
	socialv1 "github.com/kurovoxx/tallerintegracion3/back/social/internal/gen/socialv1"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository"
	"google.golang.org/grpc"
)

// chanChat registra cada RemoveMember en un canal para esperar la goroutine
// best-effort sin sleeps.
type chanChat struct {
	got chan [2]string // {channelID, userID}
	err error
}

func (c *chanChat) RemoveMember(_ context.Context, channelType, channelID, userID string) error {
	if channelType != ChannelTypeStream {
		return errors.New("tipo de canal inesperado: " + channelType)
	}
	c.got <- [2]string{channelID, userID}
	return c.err
}

func newChanChat(err error) *chanChat { return &chanChat{got: make(chan [2]string, 8), err: err} }

func expectRemoved(t *testing.T, c *chanChat, groupID, userID string) {
	t.Helper()
	select {
	case got := <-c.got:
		if got != [2]string{ChannelIDForGroup(groupID), userID} {
			t.Fatalf("remoción inesperada: %v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no se quitó a %s del canal del grupo %s", userID, groupID)
	}
}

func TestChatCleanupOnLeaveKickBan(t *testing.T) {
	svc, store := newTestSvc()
	chat := newChanChat(nil)
	svc.SetChatMembership(chat)
	g := newGroup(t, svc, uAdmin)
	joinAs(t, svc, store, g.ID, uMember)
	joinAs(t, svc, store, g.ID, uOther)
	joinAs(t, svc, store, g.ID, uAdmin2)

	if err := svc.LeaveGroup(ctx, g.ID, uMember, false, ""); err != nil {
		t.Fatal(err)
	}
	expectRemoved(t, chat, g.ID, uMember)

	if err := svc.KickMember(ctx, g.ID, uAdmin, uOther, false, ""); err != nil {
		t.Fatal(err)
	}
	expectRemoved(t, chat, g.ID, uOther)

	if err := svc.BanMember(ctx, g.ID, uAdmin, uAdmin2, false, ""); err != nil {
		t.Fatal(err)
	}
	expectRemoved(t, chat, g.ID, uAdmin2)
}

func TestChatCleanupOnAccountDeletionEveryGroup(t *testing.T) {
	svc, store := newTestSvc()
	chat := newChanChat(nil)
	svc.SetChatMembership(chat)
	solo := newGroup(t, svc, uAdmin) // se elimina: igual se limpia el canal
	shared := newGroup(t, svc, uAdmin2)
	joinAs(t, svc, store, shared.ID, uAdmin)

	if _, err := svc.HandleAccountDeletion(ctx, uAdmin, ""); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{ChannelIDForGroup(solo.ID): true, ChannelIDForGroup(shared.ID): true}
	for range want {
		select {
		case got := <-chat.got:
			if !want[got[0]] || got[1] != uAdmin {
				t.Fatalf("remoción inesperada: %v", got)
			}
			delete(want, got[0])
		case <-time.After(2 * time.Second):
			t.Fatalf("faltan canales por limpiar: %v", want)
		}
	}
}

func TestChatCleanupFailureDoesNotFailLeave(t *testing.T) {
	svc, store := newTestSvc()
	chat := newChanChat(errors.New("stream caído"))
	svc.SetChatMembership(chat)
	g := newGroup(t, svc, uAdmin)
	joinAs(t, svc, store, g.ID, uMember)

	if err := svc.LeaveGroup(ctx, g.ID, uMember, false, ""); err != nil {
		t.Fatalf("un fallo de Stream no debe revertir la salida: %v", err)
	}
	expectRemoved(t, chat, g.ID, uMember)
	if ok, _ := store.IsMember(ctx, g.ID, uMember); ok {
		t.Fatal("la salida debe quedar confirmada aunque Stream falle")
	}
}

func TestChatCleanupSkippedWhenOperationFails(t *testing.T) {
	svc, store := newTestSvc()
	chat := newChanChat(nil)
	svc.SetChatMembership(chat)
	g := newGroup(t, svc, uAdmin)
	joinAs(t, svc, store, g.ID, uMember)

	// Un miembro común no puede expulsar: no debe tocarse el chat.
	if err := svc.KickMember(ctx, g.ID, uMember, uAdmin, false, ""); err == nil {
		t.Fatal("se esperaba error de permisos")
	}
	// removeFromChat solo se lanza después de confirmar la operación: si
	// falló, ni siquiera se crea la goroutine.
	if len(chat.got) != 0 {
		t.Fatalf("no debe limpiarse el chat si la operación falla: %v", <-chat.got)
	}
}

type accountDeletionStoreFake struct {
	group            repository.AccountDeletionGroup
	membershipResult model.SuccessionResult
	complete         bool
}

func (s *accountDeletionStoreFake) Begin(context.Context, string) error { return nil }
func (s *accountDeletionStoreFake) IsComplete(context.Context, string) (bool, error) {
	return s.complete, nil
}
func (s *accountDeletionStoreFake) PendingGroups(context.Context, string) ([]repository.AccountDeletionGroup, error) {
	if s.complete {
		return nil, nil
	}
	return []repository.AccountDeletionGroup{s.group}, nil
}
func (s *accountDeletionStoreFake) RemoveMembership(context.Context, string, string) (model.SuccessionResult, error) {
	return s.membershipResult, nil
}
func (s *accountDeletionStoreFake) MarkMembershipRemoved(context.Context, string, string, bool) error {
	s.group.MembershipRemoved = true
	return nil
}
func (s *accountDeletionStoreFake) CompleteGroup(context.Context, string, string) error {
	s.group = repository.AccountDeletionGroup{}
	return nil
}
func (s *accountDeletionStoreFake) Complete(context.Context, string) error {
	s.complete = true
	return nil
}
func (s *accountDeletionStoreFake) AcquireRequest(context.Context, string) (func(), bool, error) {
	return func() {}, true, nil
}

type completedNotesCleanup struct{}

func (completedNotesCleanup) UnshareAll(context.Context, *notesv1.UnshareAllRequest, ...grpc.CallOption) (*notesv1.UnshareAllResponse, error) {
	return &notesv1.UnshareAllResponse{Status: notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_COMPLETE}, nil
}

func TestDurableAccountDeletionAlsoSchedulesChatRemoval(t *testing.T) {
	const userID, groupID = "user-being-deleted", "group-being-cleaned"
	store := &accountDeletionStoreFake{
		group:            repository.AccountDeletionGroup{GroupID: groupID},
		membershipResult: model.SuccessionResult{GroupID: groupID, GroupDeleted: true},
	}
	chat := newChanChat(nil)
	svc := &AccountDeletionService{store: store, notes: completedNotesCleanup{}}
	svc.SetChatMembership(chat)

	_, status, err := svc.Cleanup(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	if status != socialv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_COMPLETE {
		t.Fatalf("cleanup status=%s, want COMPLETE", status)
	}
	expectRemoved(t, chat, groupID, userID)
}
