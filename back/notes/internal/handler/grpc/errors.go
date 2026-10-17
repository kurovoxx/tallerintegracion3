package grpc

import (
	"errors"

	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mapError traduce los errores tipados del dominio a códigos de status gRPC
// según la tabla documentada en v1/notes.proto. No inspecciona strings: usa
// errors.Is/errors.As y el código snake_case del ServiceError, igual que el
// handler HTTP (note_handler.go:handleServiceError). Nunca filtra trazas SQL ni
// detalles internos.
func mapError(err error) error {
	if err == nil {
		return nil
	}
	// 1. OAuth tipado: PERMISSION_DENIED genérico ("conecte su Drive").
	if drive.IsOAuthError(err) {
		return status.Error(codes.PermissionDenied, "conecte o renueve su Google Drive")
	}
	// 2. Errores de dominio con centinelas estructurados.
	var se *service.ServiceError
	if errors.As(err, &se) {
		switch {
		case errors.Is(err, service.ErrDriveUnavailable):
			return status.Error(codes.Unavailable, "servicio de almacenamiento no disponible")
		case errors.Is(err, service.ErrSourceAccessDenied):
			// Zero-knowledge: indistinguible de una nota inexistente.
			return status.Error(codes.NotFound, "nota no encontrada")
		case errors.Is(err, service.ErrInternalDatabase), errors.Is(err, service.ErrInternalServer):
			return status.Error(codes.Internal, "error interno")
		default:
			return status.Error(mapCode(se.Code), se.Message)
		}
	}
	// 3. Defensa en profundidad: *drive.DriveError crudo.
	var de *drive.DriveError
	if errors.As(err, &de) {
		switch de.Code {
		case 403, 404:
			return status.Error(codes.NotFound, "Nota no disponible en almacenamiento remoto")
		case 413:
			return status.Error(codes.InvalidArgument, "archivo demasiado grande")
		default:
			return status.Error(codes.Unavailable, "servicio de almacenamiento no disponible")
		}
	}
	// 4. Fallback seguro: nunca exponer el error crudo.
	return status.Error(codes.Internal, "error interno")
}

// mapCode traduce un código snake_case del catálogo utils a su categoría gRPC.
// El detalle (code) viaja en el mensaje del status; el cliente puede separarlo
// o consumir solo la categoría gruesa.
func mapCode(code string) codes.Code {
	switch code {
	case utils.ErrNotFound, utils.ErrNoteUnavailable:
		return codes.NotFound
	case utils.ErrForbidden:
		return codes.PermissionDenied
	case utils.ErrConflict:
		// Versión desactualizada / carrera: ABORTED (reintentable).
		return codes.Aborted
	case utils.ErrAlreadySaved, utils.ErrAlreadyLiked:
		return codes.AlreadyExists
	case utils.ErrRateLimited:
		return codes.ResourceExhausted
	case utils.ErrUnauthorized, utils.ErrInvalidToken, utils.ErrTokenExpired:
		return codes.Unauthenticated
	case utils.ErrBadRequest, utils.ErrInvalidTitle, utils.ErrInvalidVisibility,
		utils.ErrInvalidSubjectID, utils.ErrInvalidAccessMode,
		utils.ErrFileTooLarge, utils.ErrPayloadTooLarge, utils.ErrUnsupportedMediaType:
		return codes.InvalidArgument
	default:
		return codes.Internal
	}
}
