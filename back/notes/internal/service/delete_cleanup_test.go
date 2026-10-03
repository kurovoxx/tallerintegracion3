package service

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
)

type failingDriveCleanup struct {
	drive.Client
	err   error
	calls int
}

func (d *failingDriveCleanup) DeleteFile(context.Context, string, string) error {
	d.calls++
	return d.err
}

func (d *failingDriveCleanup) DeleteAttachment(context.Context, string, string) error {
	d.calls++
	return d.err
}

func TestDeleteCleanupToleratesMissingDriveFilesAndNetworkFailures(t *testing.T) {
	for _, attachment := range []bool{false, true} {
		for name, upstreamErr := range map[string]error{
			"not_found": &drive.DriveError{Code: 404, Message: "not found"},
			"network":   &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")},
		} {
			resource := "note"
			if attachment {
				resource = "attachment"
			}
			t.Run(resource+"/"+name, func(t *testing.T) {
				ctx := context.Background()
				svc, client, notes, attachments, _, _, _, _ := newTestService()
				owner := uuid.NewString()
				note, err := svc.Create(ctx, owner, "Delete cleanup", nil, "private", stringPtr("text"), "")
				if err != nil {
					t.Fatal(err)
				}
				cleanup := &failingDriveCleanup{Client: client, err: upstreamErr}
				if attachment {
					att, err := svc.AddAttachment(ctx, owner, note.ID, "photo.png", "image/png", []byte("image"), false)
					if err != nil {
						t.Fatal(err)
					}
					svc.drive = cleanup
					if err := svc.RemoveAttachment(ctx, owner, note.ID, att.ID); err != nil {
						t.Fatalf("Drive failure blocked attachment deletion: %v", err)
					}
					if stored, err := attachments.GetByID(ctx, att.ID); err != nil || stored != nil {
						t.Fatalf("attachment metadata remains: %v %v", stored, err)
					}
					if stored, err := notes.GetByID(ctx, note.ID); err != nil || stored == nil {
						t.Fatalf("attachment deletion affected parent: %v %v", stored, err)
					}
				} else {
					svc.drive = cleanup
					if err := svc.Delete(ctx, owner, note.ID); err != nil {
						t.Fatalf("Drive failure blocked note deletion: %v", err)
					}
					if stored, err := notes.GetByID(ctx, note.ID); err != nil || stored != nil {
						t.Fatalf("note metadata remains: %v %v", stored, err)
					}
				}
				if cleanup.calls == 0 {
					t.Fatal("Drive cleanup was not attempted")
				}
				jobs, err := notes.ClaimDriveOperations(ctx, 10, time.Minute)
				if err != nil || len(jobs) != 1 {
					t.Fatalf("cleanup retry must remain durable: %v %v", jobs, err)
				}
			})
		}
	}
}
