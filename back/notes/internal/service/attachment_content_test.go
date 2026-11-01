package service

import (
	"context"
	"github.com/google/uuid"
	"testing"
)

func TestAttachmentContentSharePolicy(t *testing.T) {
	for _, mode := range []string{"link", "restricted"} {
		t.Run(mode, func(t *testing.T) {
			svc, storage, _, _, _, _, shares, social := newTestService()
			ctx := context.Background()
			owner, reader, group := uuid.NewString(), uuid.NewString(), uuid.NewString()
			note, err := svc.Create(ctx, owner, "Adjuntos", nil, "private", nil, "")
			if err != nil {
				t.Fatal(err)
			}
			att, err := svc.AddAttachment(ctx, owner, note.ID, "foto.png", "image/png", []byte{137, 80, 78, 71}, true)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := shares.Create(ctx, note.ID, group, false, mode, 0); err != nil {
				t.Fatal(err)
			}
			if mode == "restricted" {
				if _, _, _, err := svc.GetAttachmentContent(ctx, reader, note.ID, att.ID); err == nil {
					t.Fatal("nonmember got bytes")
				}
				social.AddMember(reader, group)
			}
			storage.Disconnect(reader)
			if _, _, _, err := svc.GetAttachmentContent(ctx, reader, note.ID, att.ID); err != nil {
				t.Fatal(err)
			}
			for _, ids := range [][2]string{{"invalid", att.ID}, {note.ID, "invalid"}, {note.ID, uuid.NewString()}} {
				if _, _, _, err := svc.GetAttachmentContent(ctx, reader, ids[0], ids[1]); err == nil {
					t.Fatal("invalid/missing attachment served")
				}
			}
		})
	}
}
