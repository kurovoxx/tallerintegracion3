package drive

import (
	"bytes"
	"context"
	api "google.golang.org/api/drive/v3"
	"io"
	"net/http"
	"testing"
)

func TestDownloadAttachmentBytesAndLimit(t *testing.T) {
	for _, size := range []int{8, MaxAttachmentBytes, MaxAttachmentBytes + 1} {
		data := bytes.Repeat([]byte{0xff}, size)
		r := NewRealDriveClient(&stubTokenProvider{token: "owner-token"})
		r.newService = func(ctx context.Context, token string) (*api.Service, error) {
			if token != "owner-token" {
				t.Fatal("wrong token")
			}
			return driveServiceWithTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != "GET" || req.URL.Query().Get("alt") != "media" {
					t.Fatal("not a binary download")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"image/jpeg"}}, Body: io.NopCloser(bytes.NewReader(data)), ContentLength: -1}, nil
			})), nil
		}
		got, mime, err := r.DownloadAttachment(context.Background(), "owner", "file")
		if size > MaxAttachmentBytes {
			if err == nil || len(got) != 0 {
				t.Fatal("unbounded download")
			}
			continue
		}
		if err != nil || mime != "image/jpeg" || !bytes.Equal(got, data) {
			t.Fatalf("binary mismatch: %v", err)
		}
	}
}
