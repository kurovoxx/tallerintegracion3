package drive

import (
	"mime"
	"net/http"
	"path/filepath"
	"strings"
)

// Tipos MIME soportados explícitamente por el sprint (adjuntos de apuntes).
const (
	MimeJPEG     = "image/jpeg"
	MimePNG      = "image/png"
	MimePDF      = "application/pdf"
	MimeMarkdown = "text/markdown"
	MimeOctet    = "application/octet-stream"
)

// extensionMimeTypes mapea extensiones conocidas a su MIME canónico. Se usa
// antes del sniffing porque http.DetectContentType no reconoce text/markdown
// (lo degrada a text/plain) y porque el nombre del archivo es la señal más
// estable que envía el cliente.
var extensionMimeTypes = map[string]string{
	".jpg":      MimeJPEG,
	".jpeg":     MimeJPEG,
	".jpe":      MimeJPEG,
	".png":      MimePNG,
	".pdf":      MimePDF,
	".md":       MimeMarkdown,
	".markdown": MimeMarkdown,
}

// normalizeMimeType limpia parámetros (charset, boundary, etc.), espacios y
// mayúsculas: "Application/PDF; charset=binary" -> "application/pdf".
func normalizeMimeType(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if mt, _, err := mime.ParseMediaType(raw); err == nil && mt != "" {
		return strings.ToLower(mt)
	}
	if i := strings.IndexByte(raw, ';'); i >= 0 {
		raw = raw[:i]
	}
	return strings.ToLower(strings.TrimSpace(raw))
}

// isGenericMime reporta si el tipo declarado no aporta información real y por
// tanto conviene resolverlo por extensión/sniffing.
func isGenericMime(mt string) bool {
	switch mt {
	case "", MimeOctet, "binary/octet-stream", "application/unknown":
		return true
	default:
		return false
	}
}

// MimeTypeFromExtension resuelve el MIME por la extensión del nombre de
// archivo. Devuelve "" si la extensión es desconocida.
func MimeTypeFromExtension(fileName string) string {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(fileName)))
	if ext == "" {
		return ""
	}
	if mt, ok := extensionMimeTypes[ext]; ok {
		return mt
	}
	return normalizeMimeType(mime.TypeByExtension(ext))
}

// DetectMimeType resuelve el MIME a persistir en Drive/Postgres preservando lo
// que declare el cliente cuando es específico. Solo cuando el valor viene vacío
// o genérico (application/octet-stream) se resuelve por extensión y, como
// último recurso, por sniffing del contenido (http.DetectContentType).
func DetectMimeType(fileName string, declared string, data []byte) string {
	declared = normalizeMimeType(declared)
	if !isGenericMime(declared) {
		return declared
	}
	if mt := MimeTypeFromExtension(fileName); mt != "" {
		return mt
	}
	if len(data) > 0 {
		if mt := normalizeMimeType(http.DetectContentType(data)); !isGenericMime(mt) {
			return mt
		}
	}
	if declared != "" {
		return declared
	}
	return MimeOctet
}
