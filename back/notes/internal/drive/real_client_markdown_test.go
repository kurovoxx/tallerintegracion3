package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
)

// capturedUpload es lo que la API de Drive recibe en una subida multipart:
// los metadatos JSON y la parte media con su Content-Type.
type capturedUpload struct {
	method       string
	path         string
	metadata     map[string]interface{}
	media        string
	mediaType    string
	hasMediaPart bool
}

// captureDriveRequest lee una petición de la librería de Drive: multipart
// (metadatos + media) o JSON simple (solo metadatos).
func captureDriveRequest(t *testing.T, req *http.Request) capturedUpload {
	t.Helper()
	out := capturedUpload{method: req.Method, path: req.URL.Path}
	body, _ := io.ReadAll(req.Body)
	mediaType, params, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("Content-Type inválido %q: %v", req.Header.Get("Content-Type"), err)
	}
	if !strings.HasPrefix(mediaType, "multipart/") {
		if len(body) > 0 {
			if err := json.Unmarshal(body, &out.metadata); err != nil {
				t.Fatalf("metadatos JSON inválidos: %v", err)
			}
		}
		return out
	}
	mr := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	for i := 0; ; i++ {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("multipart inválido: %v", err)
		}
		data, _ := io.ReadAll(part)
		if i == 0 {
			if err := json.Unmarshal(data, &out.metadata); err != nil {
				t.Fatalf("metadatos JSON inválidos: %v", err)
			}
			continue
		}
		out.hasMediaPart = true
		out.media = string(data)
		out.mediaType = part.Header.Get("Content-Type")
	}
	return out
}

func jsonResponse(v interface{}) *http.Response {
	body, _ := json.Marshal(v)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{"Content-Type": []string{"application/json"}}}
}

// clientCapturing devuelve un RealDriveClient cuyo transporte guarda cada
// petición de escritura (PATCH/POST) y responde con un archivo mínimo.
func clientCapturing(t *testing.T, got *[]capturedUpload) *RealDriveClient {
	t.Helper()
	r := NewRealDriveClient(&perUserTokenProvider{})
	r.newService = func(ctx context.Context, token string) (*drive.Service, error) {
		return driveServiceWithTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/files"):
				// ensureAppFolder: la carpeta de la app ya existe.
				return jsonResponse(map[string]interface{}{"files": []map[string]string{{"id": "folder-1"}}}), nil
			case req.Method == http.MethodGet:
				// Descarga del original en CopyFile.
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("# Original")), Header: http.Header{}}, nil
			default:
				*got = append(*got, captureDriveRequest(t, req))
				return jsonResponse(map[string]string{"id": "file-1"}), nil
			}
		})), nil
	}
	return r
}

// Editar contenido y título envía el contenido nuevo como text/markdown y
// conserva el nombre .md y el MIME del archivo (antes el nombre perdía .md y
// Drive cambiaba el archivo a text/plain).
func TestRealUpdateFileSendsMarkdownContentAndKeepsExtension(t *testing.T) {
	var got []capturedUpload
	r := clientCapturing(t, &got)
	content := "# Apunte (editado)\n\nTexto nuevo"
	title := "Apunte (editado)"
	if err := r.UpdateFile(context.Background(), "user-b", "file-1", &content, &title); err != nil {
		t.Fatalf("UpdateFile: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("se esperaba 1 petición de escritura, got %d", len(got))
	}
	u := got[0]
	if u.method != http.MethodPatch || !strings.Contains(u.path, "/upload/drive/v3/files/file-1") {
		t.Fatalf("debe ser una subida PATCH al archivo existente, got %s %s", u.method, u.path)
	}
	if !u.hasMediaPart || u.media != content {
		t.Fatalf("el contenido nuevo no viaja en la subida: %q", u.media)
	}
	if !strings.HasPrefix(u.mediaType, MimeMarkdown) {
		t.Fatalf("la parte media debe ser %s, got %q", MimeMarkdown, u.mediaType)
	}
	if u.metadata["name"] != "Apunte (editado).md" || u.metadata["mimeType"] != MimeMarkdown {
		t.Fatalf("metadatos incorrectos: %v", u.metadata)
	}
}

// Editar solo el contenido no cambia el nombre del archivo.
func TestRealUpdateFileContentOnlyDoesNotRename(t *testing.T) {
	var got []capturedUpload
	r := clientCapturing(t, &got)
	content := "solo contenido"
	if err := r.UpdateFile(context.Background(), "user-b", "file-1", &content, nil); err != nil {
		t.Fatalf("UpdateFile: %v", err)
	}
	if len(got) != 1 || got[0].media != content || !strings.HasPrefix(got[0].mediaType, MimeMarkdown) {
		t.Fatalf("subida incorrecta: %+v", got)
	}
	if _, renamed := got[0].metadata["name"]; renamed {
		t.Fatalf("no debe renombrar al editar solo el contenido: %v", got[0].metadata)
	}
}

// Renombrar sin contenido conserva .md y no sube media ni cambia el MIME.
func TestRealUpdateFileRenameOnlyKeepsMarkdownExtension(t *testing.T) {
	var got []capturedUpload
	r := clientCapturing(t, &got)
	for _, title := range []string{"Nuevo título", "Nuevo título.md", "Nuevo título.MD"} {
		got = got[:0]
		tt := title
		if err := r.UpdateFile(context.Background(), "user-b", "file-1", nil, &tt); err != nil {
			t.Fatalf("UpdateFile(%q): %v", title, err)
		}
		u := got[0]
		if u.hasMediaPart || strings.Contains(u.path, "/upload/") {
			t.Fatalf("%q: renombrar no debe subir contenido: %s", title, u.path)
		}
		wantName := "Nuevo título.md"
		if strings.HasSuffix(title, ".MD") {
			wantName = title
		}
		if u.metadata["name"] != wantName {
			t.Fatalf("%q: nombre = %v, want %q", title, u.metadata["name"], wantName)
		}
		if _, ok := u.metadata["mimeType"]; ok {
			t.Fatalf("%q: renombrar no debe tocar el MIME: %v", title, u.metadata)
		}
	}
}

// La copia de un apunte se crea como .md en el Drive de quien copia.
func TestRealCopyFileNamesCloneWithMarkdownExtension(t *testing.T) {
	var got []capturedUpload
	r := clientCapturing(t, &got)
	if _, err := r.CopyFile(context.Background(), "user-a", "src-1", "user-b", "note-clon", "Apunte público"); err != nil {
		t.Fatalf("CopyFile: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("se esperaba 1 creación, got %d", len(got))
	}
	u := got[0]
	if u.metadata["name"] != "Apunte público.md" || u.metadata["mimeType"] != MimeMarkdown || u.media != "# Original" {
		t.Fatalf("copia incorrecta: metadatos %v, media %q", u.metadata, u.media)
	}
}

func TestMarkdownFileName(t *testing.T) {
	cases := map[string]string{
		"Apunte":       "Apunte.md",
		"Apunte.md":    "Apunte.md",
		"  Apunte  ":   "Apunte.md",
		"Apunte.MD":    "Apunte.MD",
		"v1.2 resumen": "v1.2 resumen.md",
		"":             "",
	}
	for in, want := range cases {
		if got := markdownFileName(in); got != want {
			t.Fatalf("markdownFileName(%q) = %q, want %q", in, got, want)
		}
	}
}
