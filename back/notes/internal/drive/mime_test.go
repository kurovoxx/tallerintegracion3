package drive

import (
	"bytes"
	"context"
	"errors"
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

func TestSniffMimeType(t *testing.T) {
	if got := SniffMimeType(pngBytes); got != MimePNG {
		t.Fatalf("png = %q, want %q", got, MimePNG)
	}
	if got := SniffMimeType(jpegBytes); got != MimeJPEG {
		t.Fatalf("jpeg = %q, want %q", got, MimeJPEG)
	}
	if got := SniffMimeType(pdfBytes); got != MimePDF {
		t.Fatalf("pdf = %q, want %q", got, MimePDF)
	}
	if got := SniffMimeType(nil); got != "" {
		t.Fatalf("sin datos = %q, want vacío", got)
	}
	if got := SniffMimeType([]byte("solo texto")); got != "text/plain" {
		t.Fatalf("texto = %q, want text/plain", got)
	}
}

// TestSniffMimeTypeOnlyFirst512Bytes fija el límite del sniffing: la firma
// posterior al byte 512 no cuenta (el archivo es texto) y una firma válida al
// inicio sigue detectándose aunque el archivo sea mucho mayor.
func TestSniffMimeTypeOnlyFirst512Bytes(t *testing.T) {
	late := append(bytes.Repeat([]byte{0x41}, int(mimeSniffLen)), pngBytes...)
	if got := SniffMimeType(late); got != "text/plain" {
		t.Fatalf("firma fuera de los primeros 512 bytes = %q, want text/plain", got)
	}
	large := append(append([]byte{}, pngBytes...), bytes.Repeat([]byte{0x00}, 2048)...)
	if got := SniffMimeType(large); got != MimePNG {
		t.Fatalf("firma al inicio de archivo grande = %q, want %q", got, MimePNG)
	}
}

func TestIsAllowedAttachmentMimeType(t *testing.T) {
	for _, mt := range []string{MimeJPEG, MimePNG, MimePDF} {
		if !IsAllowedAttachmentMimeType(mt) {
			t.Fatalf("%q debe estar permitido", mt)
		}
	}
	for _, mt := range []string{"text/markdown", "text/plain", MimeOctet, "application/zip", ""} {
		if IsAllowedAttachmentMimeType(mt) {
			t.Fatalf("%q no debe estar permitido", mt)
		}
	}
}

// TestValidateAttachmentMime fija el contrato de sniffing estricto:
//   - el tipo sniffed debe estar en la whitelist (si no -> ErrUnsupportedMimeType);
//   - el MIME declarado específico debe coincidir con el sniffed
//     (si difiere -> ErrMimeTypeMismatch); el declarado genérico/vacío se acepta.
func TestValidateAttachmentMime(t *testing.T) {
	cases := []struct {
		name     string
		declared string
		data     []byte
		want     string
		wantErr  error
	}{
		{"png declarado y contenido png", MimePNG, pngBytes, MimePNG, nil},
		{"jpeg con parámetros", "image/jpeg; charset=binary", jpegBytes, MimeJPEG, nil},
		{"pdf declarado y contenido pdf", MimePDF, pdfBytes, MimePDF, nil},
		{"declarado genérico se resuelve por sniffing", MimeOctet, pngBytes, MimePNG, nil},
		{"declarado vacío se resuelve por sniffing", "", jpegBytes, MimeJPEG, nil},
		{"mismo tipo con mayúsculas y espacios", " IMAGE/PNG ", pngBytes, MimePNG, nil},
		{"declarado contradice contenido", MimeJPEG, pngBytes, MimePNG, ErrMimeTypeMismatch},
		{"pdf declarado con contenido png", MimePDF, pngBytes, MimePNG, ErrMimeTypeMismatch},
		{"contenido de texto no permitido", "text/plain; charset=utf-8", []byte("hola"), "", ErrUnsupportedMimeType},
		{"markdown no permitido", MimeMarkdown, []byte("# nota"), "", ErrUnsupportedMimeType},
		{"binario desconocido no permitido", MimeOctet, []byte{0x00, 0x01, 0x02}, "", ErrUnsupportedMimeType},
		{"sin datos no permitido", "", nil, "", ErrUnsupportedMimeType},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateAttachmentMime(tc.declared, tc.data)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("ValidateAttachmentMime(%q) err = %v, want %v", tc.declared, err, tc.wantErr)
			}
			if tc.wantErr == nil && got != tc.want {
				t.Fatalf("ValidateAttachmentMime(%q) = %q, want %q", tc.declared, got, tc.want)
			}
		})
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
