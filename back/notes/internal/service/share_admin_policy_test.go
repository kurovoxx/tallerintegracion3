package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// Política: solo los admins del grupo comparten apuntes en él.
func TestShare_SoloAdminsDelGrupo(t *testing.T) {
	svc, _, _, _, _, _, _, social := newTestService()
	ctx := context.Background()
	admin, member, stranger, group := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	social.AddAdmin(admin, group)
	social.AddMember(member, group)

	noteM, err := svc.Create(ctx, member, "De miembro", nil, "private", stringPtr("x"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Share(ctx, member, noteM.ID, group, "link"); err == nil {
		t.Fatal("un miembro común no debe poder compartir")
	} else if se, ok := err.(*ServiceError); !ok || se.Code != "forbidden" {
		t.Fatalf("esperaba forbidden, got %v", err)
	}

	noteS, _ := svc.Create(ctx, stranger, "De extraño", nil, "private", stringPtr("x"), "")
	if _, err := svc.Share(ctx, stranger, noteS.ID, group, "link"); err == nil {
		t.Fatal("un no miembro no debe poder compartir")
	}

	noteA, _ := svc.Create(ctx, admin, "De admin", nil, "private", stringPtr("x"), "")
	shared, err := svc.Share(ctx, admin, noteA.ID, group, "link")
	if err != nil {
		t.Fatalf("el admin sí puede compartir: %v", err)
	}
	if !shared.IsAdminNote {
		t.Fatal("el share de un admin debe marcarse is_admin_note")
	}
}
