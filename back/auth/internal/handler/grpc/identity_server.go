// Package grpc expone identity.v1.IdentityService (usuarios, verificación
// de JWT y tokens OAuth) para llamadas servicio-a-servicio. La clave interna
// la exige el interceptor (internal/interceptor), no este handler.
package grpc

import (
	"context"
	"errors"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	identityv1 "github.com/kurovoxx/tallerintegracion3/back/auth/internal/gen/identityv1"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// oauthTokenService es lo único que GetOAuthToken necesita de los servicios
// OAuth: el token vigente (con refresh interno). Así los tests inyectan
// dobles sin Google ni PostgreSQL.
type oauthTokenService interface {
	GetValidAccessToken(ctx context.Context, userID string) (string, error)
}

// calendarService añade el reporte de revocación sobre oauthTokenService.
type calendarService interface {
	oauthTokenService
	ReportCalendarPermissionDenied(ctx context.Context, userID string) error
}

// oauthConnectionStore lee la fila OAuth para los metadatos opcionales
// (expires_at, external_email). *repository.OAuthRepository la implementa.
type oauthConnectionStore interface {
	GetByUserIDAndProvider(ctx context.Context, userID, provider string) (*model.OAuthConnection, error)
}

type IdentityServer struct {
	identityv1.UnimplementedIdentityServiceServer
	users    *service.UsersService
	jwt      *service.JWTService
	drive    oauthTokenService
	calendar calendarService
	oauth    oauthConnectionStore
}

func NewIdentityServer(users *service.UsersService, jwtSvc *service.JWTService, drive oauthTokenService, calendar calendarService, oauth oauthConnectionStore) *IdentityServer {
	return &IdentityServer{users: users, jwt: jwtSvc, drive: drive, calendar: calendar, oauth: oauth}
}

func (s *IdentityServer) GetUser(ctx context.Context, req *identityv1.GetUserRequest) (*identityv1.GetUserResponse, error) {
	id := strings.TrimSpace(req.GetUserId())
	// El repositorio castea a uuid[] en PG: un id malformado rompería la
	// query en vez de dar NotFound, así que se valida en el borde gRPC
	// (el HTTP y el repository quedan intactos).
	if !utils.ValidateUUID(id) {
		return nil, status.Error(codes.InvalidArgument, "user_id inválido (UUID esperado)")
	}
	out, err := s.users.Lookup(ctx, []string{id})
	if err != nil {
		return nil, status.Error(codes.Internal, "internal")
	}
	if len(out) == 0 {
		return nil, status.Error(codes.NotFound, "user_not_found")
	}
	u := out[0]
	resp := &identityv1.GetUserResponse{UserId: u.UserID, Email: u.Email}
	if u.DisplayName != nil {
		resp.DisplayName = u.DisplayName
	}
	return resp, nil
}

func (s *IdentityServer) LookupUsers(ctx context.Context, req *identityv1.LookupUsersRequest) (*identityv1.LookupUsersResponse, error) {
	// Paridad con el HTTP: vacíos se descartan en silencio, pero un id no
	// vacío malformado envenenaría el cast uuid[] de PG, así que el lote se
	// rechaza (InvalidArgument) en vez de fallar como Internal.
	ids := make([]string, 0, len(req.GetUserIds()))
	for _, id := range req.GetUserIds() {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if !utils.ValidateUUID(id) {
			return nil, status.Error(codes.InvalidArgument, "user_id inválido (UUID esperado)")
		}
		ids = append(ids, id)
	}
	out, err := s.users.Lookup(ctx, ids)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal")
	}
	users := make([]*identityv1.User, 0, len(out))
	for _, u := range out {
		users = append(users, &identityv1.User{UserId: u.UserID, Email: u.Email, DisplayName: u.DisplayName})
	}
	return &identityv1.LookupUsersResponse{Users: users}, nil
}

func (s *IdentityServer) VerifyToken(ctx context.Context, req *identityv1.VerifyTokenRequest) (*identityv1.VerifyTokenResponse, error) {
	// El token nunca se registra en logs: solo se valida.
	token := strings.TrimSpace(req.GetToken())
	if token == "" {
		return nil, status.Error(codes.InvalidArgument, "token requerido")
	}
	user, err := s.jwt.ValidarAccessToken(token)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, status.Error(codes.Unauthenticated, "token expirado")
		}
		return nil, status.Error(codes.Unauthenticated, "token inválido")
	}
	return &identityv1.VerifyTokenResponse{Valid: true, UserId: user.ID}, nil
}

func (s *IdentityServer) GetOAuthToken(ctx context.Context, req *identityv1.GetOAuthTokenRequest) (*identityv1.GetOAuthTokenResponse, error) {
	userID := strings.TrimSpace(req.GetUserId())
	// Las filas oauth comparan user_id uuid en PG: validar en el borde.
	if !utils.ValidateUUID(userID) {
		return nil, status.Error(codes.InvalidArgument, "user_id inválido (UUID esperado)")
	}
	var svc oauthTokenService
	var provider string
	switch req.GetProvider() {
	case identityv1.Provider_PROVIDER_GOOGLE_DRIVE:
		svc, provider = s.drive, model.ProviderGoogleDrive
	case identityv1.Provider_PROVIDER_GOOGLE_CALENDAR:
		svc, provider = s.calendar, model.ProviderGoogleCalendar
	default:
		return nil, status.Error(codes.InvalidArgument, "provider inválido")
	}
	// El token tampoco se registra en logs.
	token, err := svc.GetValidAccessToken(ctx, userID)
	if err != nil {
		return nil, toStatusOAuth(err)
	}
	resp := &identityv1.GetOAuthTokenResponse{AccessToken: token}
	// Metadatos best-effort desde la fila OAuth: si la lectura falla, el
	// token ya validado se devuelve igual sin ellos.
	if conn, cerr := s.oauth.GetByUserIDAndProvider(ctx, userID, provider); cerr == nil && conn != nil {
		if conn.ExpiresAt != nil && !conn.ExpiresAt.IsZero() {
			resp.ExpiresAt = timestamppb.New(*conn.ExpiresAt)
		}
		if conn.ExternalAccountEmail != nil {
			resp.ExternalEmail = conn.ExternalAccountEmail
		}
	}
	return resp, nil
}

func (s *IdentityServer) ReportCalendarRevoked(ctx context.Context, req *identityv1.ReportCalendarRevokedRequest) (*identityv1.ReportCalendarRevokedResponse, error) {
	userID := strings.TrimSpace(req.GetUserId())
	if !utils.ValidateUUID(userID) {
		return nil, status.Error(codes.InvalidArgument, "user_id inválido (UUID esperado)")
	}
	// Idempotente y sin fuga de existencia (igual que el HTTP): el error se
	// ignora a propósito y siempre es OK.
	_ = s.calendar.ReportCalendarPermissionDenied(ctx, userID)
	return &identityv1.ReportCalendarRevokedResponse{}, nil
}

// toStatusOAuth traduce los códigos de dominio OAuth (los mismos del HTTP)
// a códigos gRPC sin filtrar detalles internos.
func toStatusOAuth(err error) error {
	var se *service.ServiceError
	if errors.As(err, &se) {
		switch se.Code {
		case "bad_request":
			return status.Error(codes.InvalidArgument, se.Code)
		case "not_connected":
			return status.Error(codes.NotFound, se.Code)
		case "drive_connection_invalid", "calendar_connection_invalid":
			return status.Error(codes.Unauthenticated, se.Code)
		case "google_unavailable":
			return status.Error(codes.Unavailable, se.Code)
		}
	}
	return status.Error(codes.Internal, "internal")
}
