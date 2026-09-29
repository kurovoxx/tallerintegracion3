package drive

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
)

func jsonResp(status int, v any) *http.Response {
	body, _ := json.Marshal(v)
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(body))),
	}
}

func TestRealCreateFile_CreaCarpetaSiFalta(t *testing.T) {
	r := NewRealDriveClient(&stubTokenProvider{token: "tok"})
	var createdFolder bool
	var fileParents []any
	r.newService = func(ctx context.Context, token string) (*drive.Service, error) {
		return driveServiceWithTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/files"):
				return jsonResp(200, map[string]any{"files": []any{}}), nil
			case req.Method == http.MethodPost && req.URL.Path == "/drive/v3/files":
				var body map[string]any
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body["mimeType"] == "application/vnd.google-apps.folder" {
					createdFolder = true
					if body["name"] != AppFolderName {
						t.Fatalf("nombre carpeta inesperado: %v", body["name"])
					}
					return jsonResp(200, map[string]any{"id": "folder-1"}), nil
				}
				if p, ok := body["parents"].([]any); ok {
					fileParents = p
				}
				return jsonResp(200, map[string]any{"id": "file-1"}), nil
			}
			if strings.HasPrefix(req.URL.Path, "/upload/") {
				// Multipart: la metadata viaja como parte; basta verificar parents.
				raw, _ := io.ReadAll(req.Body)
				if strings.Contains(string(raw), "folder-1") {
					fileParents = []any{"folder-1"}
				}
				return jsonResp(200, map[string]any{"id": "file-1"}), nil
			}
			t.Fatalf("request inesperado: %s %s", req.Method, req.URL.Path)
			return nil, nil
		})), nil
	}
	id, err := r.CreateFile(context.Background(), "u", "n1", "a.md", "x")
	if err != nil || id != "file-1" {
		t.Fatalf("create = %q, %v", id, err)
	}
	if !createdFolder {
		t.Fatal("debió crear la carpeta Apuntes TI3")
	}
	if len(fileParents) != 1 || fileParents[0] != "folder-1" {
		t.Fatalf("archivo debe ir en la carpeta: %v", fileParents)
	}
}

func TestRealCreateFile_ReusaCarpetaExistente(t *testing.T) {
	r := NewRealDriveClient(&stubTokenProvider{token: "tok"})
	var folderCreates int
	r.newService = func(ctx context.Context, token string) (*drive.Service, error) {
		return driveServiceWithTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/files") {
				return jsonResp(200, map[string]any{"files": []any{map[string]any{"id": "folder-9"}}}), nil
			}
			if req.Method == http.MethodPost && req.URL.Path == "/drive/v3/files" {
				var body map[string]any
				_ = json.NewDecoder(req.Body).Decode(&body)
				if body["mimeType"] == "application/vnd.google-apps.folder" {
					folderCreates++
				}
				return jsonResp(200, map[string]any{"id": "file-2"}), nil
			}
			if strings.HasPrefix(req.URL.Path, "/upload/") {
				return jsonResp(200, map[string]any{"id": "file-2"}), nil
			}
			t.Fatalf("request inesperado: %s %s", req.Method, req.URL.Path)
			return nil, nil
		})), nil
	}
	if _, err := r.CreateFile(context.Background(), "u", "n1", "a.md", "x"); err != nil {
		t.Fatal(err)
	}
	if folderCreates != 0 {
		t.Fatalf("no debió crear carpeta, creó %d", folderCreates)
	}
}
