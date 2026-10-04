package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/utils"
)

type telemetryFailingDrive struct {
	drive.Client
	err error
}

func (d telemetryFailingDrive) GetFileContent(context.Context, string, string) (string, error) {
	return "", d.err
}

func TestNotesTelemetryRetainsDriveStatus(t *testing.T) {
	for _, status := range []int{403, 404} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			svc, mock, _, _, _, _, _, _ := newTestService()
			owner := uuid.NewString()
			note, err := svc.Create(context.Background(), owner, "Telemetry", nil, "private", nil, "")
			if err != nil {
				t.Fatal(err)
			}
			svc.drive = telemetryFailingDrive{Client: mock, err: &drive.DriveError{Code: status, Message: "unavailable"}}
			var logs bytes.Buffer
			ctx := utils.WithNotesLogger(context.Background(), slog.New(slog.NewJSONHandler(&logs, nil)).With("user_id", owner, "note_id", note.ID))
			_, err = svc.Get(ctx, owner, note.ID)
			var domain *ServiceError
			if !errors.As(err, &domain) || domain.Code != utils.ErrNoteUnavailable {
				t.Fatalf("domain contract changed: %v", err)
			}
			found := false
			decoder := json.NewDecoder(&logs)
			for decoder.More() {
				var record map[string]any
				if err := decoder.Decode(&record); err != nil {
					t.Fatal(err)
				}
				if record["event"] == "DRIVE DOWNLOAD" {
					found = true
					if record["level"] != "ERROR" || record["drive_status"] != float64(status) || record["drive_file_id"] != *note.ExternalFileID || record["note_id"] != note.ID {
						t.Fatalf("lost original Drive error or identifiers: %+v", record)
					}
				}
			}
			if !found {
				t.Fatal("Drive failure was not logged")
			}
		})
	}
}

type telemetryFailingOutbox struct {
	NoteStore
	err error
}

func (s telemetryFailingOutbox) EnqueueDriveOperation(context.Context, string, string, string, string, string, map[string]any) error {
	return s.err
}

func TestNotesTelemetryLogsIgnoredOutboxFailure(t *testing.T) {
	want := errors.New("outbox unavailable")
	svc := &NoteService{notes: telemetryFailingOutbox{err: want}}
	var logs bytes.Buffer
	ctx := utils.WithNotesLogger(context.Background(), slog.New(slog.NewJSONHandler(&logs, nil)))
	err := svc.observedNotesEnqueueDriveOperation(ctx, "delete_file", "note", "attachment", "file", "owner", nil)
	if err != want {
		t.Fatalf("store error changed: %v", err)
	}
	var record map[string]any
	if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record["level"] != "ERROR" || record["error"] != want.Error() || record["note_id"] != "note" || record["attachment_id"] != "attachment" || record["drive_file_id"] != "file" {
		t.Fatalf("outbox failure lacks detail: %+v", record)
	}
}
