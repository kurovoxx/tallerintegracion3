package service

import (
	"context"
	"errors"
	"strings"
	"time"

	notesv1 "github.com/kurovoxx/tallerintegracion3/back/social/internal/gen/notesv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const (
	// metadataInternalKey es la metadata gRPC equivalente al header HTTP
	// X-Internal-Key que Notes exige en el RPC UnshareAll.
	metadataInternalKey = "x-internal-key"
	// defaultNotesGRPCURL es el Service de k8s / nombre del contenedor en
	// compose. En local fuera de red docker, exportar
	// NOTES_GRPC_URL=localhost:9082.
	defaultNotesGRPCURL = "notes:9082"
	// notesUnshareTimeout acota la llamada de limpieza (gRPC y HTTP) para que
	// un Notes colgado no retenga la goroutine best-effort de salida de grupo.
	notesUnshareTimeout = 5 * time.Second
)

// NotesUnsharer limpia los apuntes que un usuario compartió en un grupo.
// Abstrae el transporte para permitir la migración gRPC→(fallback)HTTP y los
// tests.
type NotesUnsharer interface {
	UnshareAll(ctx context.Context, userID, groupID string) error
}

// notesGRPCUnsharer llama a Notes por gRPC (notes.v1.NotesService/UnshareAll)
// enviando el secreto interno en la metadata x-internal-key. dial es
// inyectable para tests (bufconn); nil usa grpc.NewClient.
type notesGRPCUnsharer struct {
	target string
	apiKey string
	dial   func(target string) (*grpc.ClientConn, error)
}

// newNotesGRPCUnsharer construye el cliente con default notes:9082.
func newNotesGRPCUnsharer(target, apiKey string) *notesGRPCUnsharer {
	target = strings.TrimSpace(target)
	if target == "" {
		target = defaultNotesGRPCURL
	}
	return &notesGRPCUnsharer{
		target: target,
		apiKey: strings.TrimSpace(apiKey),
		dial:   defaultNotesDial,
	}
}

// defaultNotesDial usa grpc.NewClient (conexión lazy, sin TLS: red interna).
func defaultNotesDial(target string) (*grpc.ClientConn, error) {
	return grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
}

// UnshareAll ejecuta el RPC interno. Falla cerrado si no hay dialer; el
// llamador decide el fallback HTTP.
func (u *notesGRPCUnsharer) UnshareAll(ctx context.Context, userID, groupID string) error {
	if u.dial == nil {
		return errors.New("notes grpc: dialer no configurado")
	}
	conn, err := u.dial(u.target)
	if err != nil {
		return err
	}
	defer conn.Close()
	client := notesv1.NewNotesServiceClient(conn)
	if u.apiKey != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, metadataInternalKey, u.apiKey)
	}
	_, err = client.UnshareAll(ctx, &notesv1.UnshareAllRequest{
		UserId:  userID,
		GroupId: groupID,
	})
	return err
}
