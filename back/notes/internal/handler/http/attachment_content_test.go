package http

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"net/http"
	"strings"
	"testing"
)

func TestAttachmentContentAuthorizationAndMIME(t *testing.T) {
	for _, tc := range []struct {
		name, mime string
		data       []byte
	}{
		{"foto.png", "image/png", testPNGBytes}, {"foto.jpg", "image/jpeg", testJPEGBytes}, {"guia.pdf", "application/pdf", testPDFBytes},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, svc, storage, _ := setupRouter()
			ctx := context.Background()
			owner, reader := uuid.NewString(), uuid.NewString()
			note, err := svc.Create(ctx, owner, "Adjuntos", nil, "private", nil, "")
			if err != nil {
				t.Fatal(err)
			}
			att, err := svc.AddAttachment(ctx, owner, note.ID, tc.name, tc.mime, tc.data, strings.HasPrefix(tc.mime, "image/"))
			if err != nil {
				t.Fatal(err)
			}
			path := "/notes/" + note.ID + "/attachments/" + att.ID + "/content"
			if got := doReq(r, "GET", path, "", ""); got.Code != 401 {
				t.Fatalf("auth: %d", got.Code)
			}
			if got := doReq(r, "GET", path, genToken(reader, "student"), ""); got.Code != 404 {
				t.Fatalf("private: %d", got.Code)
			}
			got := doReq(r, "GET", path, genToken(owner, "student"), "")
			if got.Code != 200 || !bytes.Equal(got.Body.Bytes(), tc.data) || got.Header().Get("Content-Type") != tc.mime {
				t.Fatalf("content: %d %s", got.Code, got.Body.String())
			}
			if !strings.HasPrefix(got.Header().Get("Content-Disposition"), "inline;") || got.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatal("unsafe content headers")
			}
			other, err := svc.Create(ctx, owner, "Otra", nil, "private", nil, "")
			if err != nil {
				t.Fatal(err)
			}
			if got := doReq(r, "GET", "/notes/"+other.ID+"/attachments/"+att.ID+"/content", genToken(owner, "student"), ""); got.Code != 404 {
				t.Fatalf("cross-note: %d", got.Code)
			}
			visibility := "public"
			if _, err := svc.Update(ctx, owner, note.ID, nil, &visibility, nil, ""); err != nil {
				t.Fatal(err)
			}
			storage.Disconnect(reader) // Reader has no OAuth; owner's connection must be used.
			if got := doReq(r, "GET", path, genToken(reader, "student"), ""); got.Code != 200 {
				t.Fatalf("public reader: %d", got.Code)
			}
			detail := doReq(r, http.MethodGet, "/notes/"+note.ID, genToken(reader, "student"), "")
			var body struct {
				Attachments []struct {
					ID string `json:"id"`
				} `json:"attachments"`
			}
			if err := json.Unmarshal(detail.Body.Bytes(), &body); err != nil || len(body.Attachments) != 1 || body.Attachments[0].ID != att.ID {
				t.Fatalf("metadata: %s", detail.Body.String())
			}
			storage.Disconnect(owner)
			if got := doReq(r, "GET", path, genToken(reader, "student"), ""); got.Code == 200 || bytes.Contains(got.Body.Bytes(), tc.data) {
				t.Fatal("owner disconnected served bytes")
			}
		})
	}
}
