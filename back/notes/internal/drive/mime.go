package drive

import (
	"errors"
	"fmt"
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

// mimeSniffLen es el número de bytes que http.DetectContentType inspecciona
// para inferir el tipo real del contenido.
const mimeSniffLen = 512

// allowedAttachmentMimeTypes es la whitelist estricta de adjuntos: solo JPEG,
// PNG y PDF pueden persistirse como adjunto de una nota. Cualquier otro tipo
// detectado en el contenido se rechaza con 415.
var allowedAttachmentMimeTypes = map[string]bool{
	MimeJPEG: true,
	MimePNG:  true,
	MimePDF:  true,
}

// Errores tipados del sniffing estricto: permiten al handler traducir a 415
// (tipo no permitido) o 400 (el contenido no coincide con lo declarado) sin
// inspeccionar mensajes.
var (
	ErrUnsupportedMimeType = errors.New("tipo MIME no permitido")
	ErrMimeTypeMismatch    = errors.New("el MIME declarado no coincide con el contenido")
)

// IsAllowedAttachmentMimeType reporta si el MIME (normalizado) pertenece a la
// whitelist estricta de adjuntos.
func IsAllowedAttachmentMimeType(mt string) bool {
	return allowedAttachmentMimeTypes[normalizeMimeType(mt)]
}

// SniffMimeType detecta el MIME real del contenido inspeccionando los primeros
// 512 bytes con http.DetectContentType y normalizando parámetros (charset).
// Devuelve "" si no hay datos.
func SniffMimeType(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	if len(data) > mimeSniffLen {
		data = data[:mimeSniffLen]
	}
	return normalizeMimeType(http.DetectContentType(data))
}

// ValidateAttachmentMime aplica el contrato de subida estricto de adjuntos:
//  1. sniffa los primeros 512 bytes del contenido y exige que el tipo real
//     pertenezca a la whitelist (image/jpeg, image/png, application/pdf);
//     si no, devuelve ErrUnsupportedMimeType (415).
//  2. si el cliente declaró un MIME específico (no vacío ni genérico como
//     application/octet-stream), exige que coincida exactamente con el sniffed;
//     si difiere, devuelve ErrMimeTypeMismatch (400).
//
// En caso de éxito devuelve el MIME sniffed, que es el único valor confiable
// para persistir en Drive/Postgres.
func ValidateAttachmentMime(declared string, data []byte) (string, error) {
	sniffed := SniffMimeType(data)
	if !IsAllowedAttachmentMimeType(sniffed) {
		return sniffed, fmt.Errorf("%w: %q", ErrUnsupportedMimeType, sniffed)
	}
	declared = normalizeMimeType(declared)
	if !isGenericMime(declared) && declared != sniffed {
		return sniffed, fmt.Errorf("%w: declarado %q, detectado %q", ErrMimeTypeMismatch, declared, sniffed)
	}
	return sniffed, nil
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
