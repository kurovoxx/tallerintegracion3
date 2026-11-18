// Package identity es el cliente servicio-a-servicio de Notes contra
// identity.v1.IdentityService. Es independiente de STORAGE_MODE: la resolución
// de public_id se usa aunque Drive esté en modo mock.
package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	identityv1 "github.com/kurovoxx/tallerintegracion3/back/notes/internal/gen/identityv1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// metadataInternalKey es el equivalente gRPC del header X-Internal-Key.
const metadataInternalKey = "x-internal-key"

// Errores tipados que el servicio traduce a 404 (not found/invalid) o 503
// (unavailable), sin filtrar detalles del upstream al cliente HTTP.
var (
	ErrNotFound    = errors.New("identity: perfil público no encontrado")
	ErrInvalid     = errors.New("identity: public_id inválido")
	ErrUnavailable = errors.New("identity: servicio no disponible")
)

// PublicProfileClient resuelve identity.users.public_id al UUID interno.
type PublicProfileClient struct {
	client  identityv1.IdentityServiceClient
	key     string
	timeout time.Duration
}

// NewPublicProfileClient construye el cliente sobre una conexión gRPC ya
// creada (perezosa). timeout <= 0 usa 5s.
func NewPublicProfileClient(client identityv1.IdentityServiceClient, key string, timeout time.Duration) *PublicProfileClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &PublicProfileClient{client: client, key: strings.TrimSpace(key), timeout: timeout}
}

// ResolvePublicProfile devuelve el UUID interno o un error tipado. El UUID
// nunca se expone en la respuesta HTTP pública de Notes.
func (c *PublicProfileClient) ResolvePublicProfile(ctx context.Context, publicID string) (string, error) {
	if c == nil || c.client == nil {
		return "", ErrUnavailable
	}
	publicID = strings.ToLower(strings.TrimSpace(publicID))
	if publicID == "" {
		return "", ErrInvalid
	}
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	callCtx = metadata.AppendToOutgoingContext(callCtx, metadataInternalKey, c.key)
	resp, err := c.client.ResolvePublicProfile(callCtx, &identityv1.ResolvePublicProfileRequest{PublicId: publicID})
	if err != nil {
		switch status.Code(err) {
		case codes.NotFound:
			return "", ErrNotFound
		case codes.InvalidArgument:
			return "", ErrInvalid
		default:
			// Unauthenticated/PermissionDenied también es un fallo de
			// configuración/dependencia: se responde 503, no 404.
			return "", ErrUnavailable
		}
	}
	userID := strings.TrimSpace(resp.GetUserId())
	if userID == "" {
		return "", ErrUnavailable
	}
	return userID, nil
}

// LookupPublicIDs resuelve en lote UUID -> código público mediante
// IdentityService.LookupUsers (implementa service.AuthorPublicIDResolver).
// Solo devuelve el public_id: el correo y el resto de datos que trae
// LookupUsers se descartan aquí. Sin ids responde mapa vacío sin llamar.
func (c *PublicProfileClient) LookupPublicIDs(ctx context.Context, userIDs []string) (map[string]string, error) {
	out := map[string]string{}
	if len(userIDs) == 0 {
		return out, nil
	}
	if c == nil || c.client == nil {
		return nil, ErrUnavailable
	}
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	callCtx = metadata.AppendToOutgoingContext(callCtx, metadataInternalKey, c.key)
	resp, err := c.client.LookupUsers(callCtx, &identityv1.LookupUsersRequest{UserIds: userIDs})
	if err != nil {
		return nil, ErrUnavailable
	}
	for _, u := range resp.GetUsers() {
		id := strings.TrimSpace(u.GetUserId())
		code := strings.TrimSpace(u.GetPublicId())
		if id != "" && code != "" {
			out[id] = code
		}
	}
	return out, nil
}
