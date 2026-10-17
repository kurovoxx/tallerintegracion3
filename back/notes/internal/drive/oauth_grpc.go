package drive

import (
	"context"
	"strings"
	"time"

	identityv1 "github.com/kurovoxx/tallerintegracion3/back/notes/internal/gen/identityv1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Mensaje de dominio emitido por Identity cuando la conexión OAuth no sirve
// (revocada, inválida o sin refresh posible). Depende del texto exacto del
// servidor Auth (ver back/auth/internal/handler/grpc/identity_server.go,
// toStatusOAuth); cualquier otro Unauthenticated es infraestructura y nunca
// debe verse como revocación del usuario (fail-closed).
const msgDriveConnectionInvalid = "drive_connection_invalid"

// GRPCOAuthTokenProvider obtiene el access token de Drive vía
// IdentityService.GetOAuthToken (tarea 3_4_6). Implementa TokenProvider con
// la misma semántica de errores que PGOAuthTokenStore, pero sin SQL ni
// refresh local: Identity renueva el token y Notes solo lo consume.
//
// No hay fallback a PostgreSQL: si Identity está caído, devuelve error de
// infraestructura (DriveError 500) para reintentar, nunca tokens ficticios.
type GRPCOAuthTokenProvider struct {
	Client  identityv1.IdentityServiceClient
	Key     string
	Timeout time.Duration
}

var _ TokenProvider = (*GRPCOAuthTokenProvider)(nil)

func (p *GRPCOAuthTokenProvider) timeout() time.Duration {
	if p.Timeout <= 0 {
		return 5 * time.Second
	}
	return p.Timeout
}

func (p *GRPCOAuthTokenProvider) ctx(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout())
	return metadata.AppendToOutgoingContext(ctx, "x-internal-key", strings.TrimSpace(p.Key)), cancel
}

// GetValidAccessToken devuelve el access_token vigente o un error tipado
// compatible con los llamantes actuales:
//   - *OAuthError si no hay conexión, está revocada/inválida o el usuario es
//     inválido: el frontend debe ofrecer reconectar Drive.
//   - *DriveError 403 si la respuesta no trae token utilizable.
//   - *DriveError 500 ante fallos de infraestructura (Auth caído, timeout,
//     Google indisponible, clave interna incorrecta, respuestas inesperadas).
//
// Notas de traducción: Identity colapsa en drive_connection_invalid varios
// casos que el store SQL distinguía (revocado, invalid_grant, sin refresh).
// Todos significan "la credencial guardada no sirve": se mapean a OAuthError
// (reconexión), que es accionable en los 3 casos y preserva el 403 hacia los
// handlers. Un Unauthenticated con otro mensaje es clave/red: 500, jamás
// falsa revocación. No se registran tokens ni claves.
func (p *GRPCOAuthTokenProvider) GetValidAccessToken(ctx context.Context, userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", &OAuthError{Message: "Conecte o renueve su Google Drive"}
	}
	ctx, cancel := p.ctx(ctx)
	defer cancel()
	resp, err := p.Client.GetOAuthToken(ctx, &identityv1.GetOAuthTokenRequest{
		UserId:   userID,
		Provider: identityv1.Provider_PROVIDER_GOOGLE_DRIVE,
	})
	if err != nil {
		switch status.Code(err) {
		case codes.NotFound:
			return "", &OAuthError{Message: "Conecte o renueve su Google Drive"}
		case codes.Unauthenticated:
			if msg, _ := status.FromError(err); msg.Message() == msgDriveConnectionInvalid {
				return "", &OAuthError{Message: "Conecte o renueve su Google Drive"}
			}
			return "", &DriveError{Code: 500, Message: "Servicio de almacenamiento no disponible"}
		default:
			return "", &DriveError{Code: 500, Message: "Servicio de almacenamiento no disponible"}
		}
	}
	if strings.TrimSpace(resp.GetAccessToken()) == "" {
		return "", &DriveError{Code: 403, Message: "Nota no disponible en almacenamiento remoto"}
	}
	return resp.GetAccessToken(), nil
}
