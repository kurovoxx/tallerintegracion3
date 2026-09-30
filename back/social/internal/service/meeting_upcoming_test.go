package service

import (
	"context"
	"testing"
	"time"
)

// Visibilidad de reuniones: mismo grupo ve lo mismo, otro grupo no,
// y el pasado se excluye de upcoming.
func TestMeetingUpcoming_VisibilidadYExpiracion(t *testing.T) {
	gid, admin, member, _ := meetingSeamSetup()
	other := "dddddddd-0000-4000-8000-000000000001"
	store := NewMemoryMeetingStore()
	store.AddGroup(gid, admin, member)
	store.AddGroup(other, admin)
	svc := NewMeetingService(store, NoopMeetingCreatedNotifier{})
	ctx := context.Background()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	future := now.Add(48 * time.Hour).UTC().Format(time.RFC3339)
	past := now.Add(-48 * time.Hour).UTC().Format(time.RFC3339)
	if _, err := svc.CreateMeeting(ctx, gid, admin, "Futura", nil, future, nil); err != nil {
		t.Fatalf("crear futura: %v", err)
	}
	if _, err := svc.CreateMeeting(ctx, gid, admin, "Pasada", nil, past, nil); err != nil {
		t.Fatalf("crear pasada (permitida, se filtra al listar): %v", err)
	}
	if _, err := svc.CreateMeeting(ctx, other, admin, "Otra", nil, future, nil); err != nil {
		t.Fatalf("crear en otro grupo: %v", err)
	}

	// A crea, B del mismo grupo la ve; solo la futura.
	got, err := svc.ListUpcomingMeetings(ctx, gid, member, now)
	if err != nil {
		t.Fatalf("listar miembro: %v", err)
	}
	if len(got) != 1 || got[0].Title != "Futura" {
		t.Fatalf("esperada solo [Futura], got %+v", got)
	}
	// Otro grupo no ve las de A.
	otherList, err := svc.ListUpcomingMeetings(ctx, other, admin, now)
	if err != nil {
		t.Fatalf("listar otro grupo: %v", err)
	}
	if len(otherList) != 1 || otherList[0].Title != "Otra" {
		t.Fatalf("aislamiento entre grupos: %+v", otherList)
	}
}
