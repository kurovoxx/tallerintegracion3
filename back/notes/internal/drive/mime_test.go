package drive

import (
	"context"
	"testing"
)

var (
	pngBytes  = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	jpegBytes = []byte("\xff\xd8\xff\xe0\x00\x10JFIF")
	pdfBytes  = []byte("%PDF-1.4\n1 0 obj")
)

func TestDetectMimeType(t *testing.T) {
	cases := []struct {
		name     string
		fileName string
		declared string
		data     []byte
		want     string
	}{
		// El MIME declarado específico siempre se preserva, incluso si la
		// extensión sugiere otra cosa (la señal del cliente manda).
		{"declarado especifico se preserva", "foto.png", "image/jpeg", pngBytes, MimeJPEG},
		{"declarado con charset se normaliza", "doc.pdf", "Application/PDF; charset=binary", pdfBytes, MimePDF},
		{"text/plain se preserva", "nota.png", "text/plain; charset=utf-8", pngBytes, "text/plain"},
		// Vacío u octet-stream se resuelve por extensión.
		{"vacio + png", "captura.png", "", pngBytes, MimePNG},
		{"vacio + jpg", "captura.jpg", "", nil, MimeJPEG},
		{"vacio + jpeg mayusculas", "CAPTURA.JPEG", "", nil, MimeJPEG},
		{"octet + pdf", "informe.pdf", MimeOctet, nil, MimePDF},
		{"octet con charset + md", "apunte.md", "application/octet-stream; charset=binary", nil, MimeMarkdown},
		{"octet + markdown", "apunte.markdown", MimeOctet, nil, MimeMarkdown},
		// Sin extensión conocida: sniffing del contenido.
		{"sniff png", "sin_extension", "", pngBytes, MimePNG},
		{"sniff jpeg", "sin_extension", MimeOctet, jpegBytes, MimeJPEG},
		{"sniff pdf", "sin_extension", "", pdfBytes, MimePDF},
		// Sin señal alguna: fallback genérico.
		{"desconocido", "archivo.bin", "", []byte{0x00, 0x01}, MimeOctet},
		{"sin nombre ni datos", "", "", nil, MimeOctet},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DetectMimeType(tc.fileName, tc.declared, tc.data); got != tc.want {
				t.Fatalf("DetectMimeType(%q, %q) = %q, want %q", tc.fileName, tc.declared, got, tc.want)
			}
		})
	}
}

func TestMimeTypeFromExtension(t *testing.T) {
	if got := MimeTypeFromExtension("nota.MD"); got != MimeMarkdown {
		t.Fatalf("MD mayúscula = %q", got)
	}
	if got := MimeTypeFromExtension("sin-extension"); got != "" {
		t.Fatalf("sin extensión = %q, want vacío", got)
	}
}

func TestMockClientPreservesMimeType(t *testing.T) {
	m := NewMockClient()
	ctx := context.Background()

	id, _, err := m.UploadAttachment(ctx, "user-1", "note-1", "foto.png", "", pngBytes, false)
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	if mt, ok := m.FileMimeType(id); !ok || mt != MimePNG {
		t.Fatalf("mime almacenado = %q (ok=%v), want %q", mt, ok, MimePNG)
	}

	id2, _, err := m.UploadAttachment(ctx, "user-1", "note-1", "foto.png", MimeJPEG, pngBytes, false)
	if err != nil {
		t.Fatalf("UploadAttachment declarado: %v", err)
	}
	if mt, _ := m.FileMimeType(id2); mt != MimeJPEG {
		t.Fatalf("mime declarado = %q, want %q", mt, MimeJPEG)
	}
}
