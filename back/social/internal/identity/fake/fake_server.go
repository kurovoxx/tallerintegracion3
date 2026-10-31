package fake

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	identityv1 "github.com/kurovoxx/tallerintegracion3/back/social/internal/gen/identityv1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrNoUser indica que el usuario no existe en el fake.
var ErrNoUser = errors.New("user not found")

// Server es un servidor gRPC falso de Identity para tests.
// Implementa IdentityServiceServer y permite configurar usuarios, tokens y
// respuestas por caso de prueba. Correr con bufconn (in-memory, sin red).
type Server struct {
	identityv1.UnimplementedIdentityServiceServer

	mu sync.Mutex

	// users mapa user_id -> datos del usuario.
	users map[string]*identityv1.User

	// validTokens mapa token JWT -> user_id válido.
	validTokens map[string]string

	// oauthTokens mapa user_id -> access token de Google (para GetOAuthToken).
	oauthTokens map[string]string

	// failVerify si es true, VerifyToken siempre devuelve Unauthenticated.
	failVerify bool

	// failGetUser si es true, GetUser siempre devuelve NotFound.
	failGetUser bool

	// failLookupUsers si es true, LookupUsers siempre devuelve error interno.
	failLookupUsers bool
}

// NewServer crea un servidor falso vacío.
func NewServer() *Server {
	return &Server{
		users:       make(map[string]*identityv1.User),
		validTokens: make(map[string]string),
		oauthTokens: make(map[string]string),
	}
}

// AddUser agrega un usuario al fake.
func (s *Server) AddUser(userID, email string, displayName ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := &identityv1.User{
		UserId:      userID,
		Email:       email,
		DisplayName: nil,
	}
	if len(displayName) > 0 {
		dn := displayName[0]
		u.DisplayName = &dn
	}
	s.users[userID] = u
}

// AddToken asocia un token JWT con un user_id para que VerifyToken lo acepte.
func (s *Server) AddToken(token, userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.validTokens[token] = userID
}

// AddOAuthToken asocia un access token de Google con un user_id para
// GetOAuthToken.
func (s *Server) AddOAuthToken(userID, accessToken string, expiresAt ...time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.oauthTokens[userID] = accessToken
	_ = expiresAt
}

// SetFailVerify hace que VerifyToken siempre devuelva Unauthenticated.
func (s *Server) SetFailVerify(fail bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failVerify = fail
}

// SetFailGetUser hace que GetUser siempre devuelva NotFound.
func (s *Server) SetFailGetUser(fail bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failGetUser = fail
}

// SetFailLookupUsers hace que LookupUsers siempre devuelva error interno.
func (s *Server) SetFailLookupUsers(fail bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failLookupUsers = fail
}

// ClearUsers elimina todos los usuarios.
func (s *Server) ClearUsers() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users = make(map[string]*identityv1.User)
}

// GetUser devuelve los datos públicos de un usuario.
func (s *Server) GetUser(ctx context.Context, req *identityv1.GetUserRequest) (*identityv1.GetUserResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.failGetUser {
		return nil, status.Error(codes.Internal, "user directory failure")
	}

	userID := strings.TrimSpace(req.GetUserId())
	if userID == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id vacío")
	}

	u, ok := s.users[userID]
	if !ok {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	var dn *string
	if name := u.GetDisplayName(); name != "" {
		dn = &name
	}
	return &identityv1.GetUserResponse{
		UserId:      u.GetUserId(),
		Email:       u.GetEmail(),
		DisplayName: dn,
	}, nil
}

// LookupUsers devuelve los datos de varios usuarios en lote.
func (s *Server) LookupUsers(ctx context.Context, req *identityv1.LookupUsersRequest) (*identityv1.LookupUsersResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.failLookupUsers {
		return nil, status.Error(codes.Internal, "user directory failure")
	}

	out := make([]*identityv1.User, 0, len(req.GetUserIds()))
	for _, id := range req.GetUserIds() {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		u, ok := s.users[id]
		if !ok {
			continue // inexistentes se omiten, no es error
		}
		var dn *string
		if name := u.GetDisplayName(); name != "" {
			dn = &name
		}
		out = append(out, &identityv1.User{
			UserId:      u.GetUserId(),
			Email:       u.GetEmail(),
			DisplayName: dn,
		})
	}
	return &identityv1.LookupUsersResponse{Users: out}, nil
}

// VerifyToken valida un JWT contra el fake. Si el token está registrado,
// devuelve el user_id asociado.
func (s *Server) VerifyToken(ctx context.Context, req *identityv1.VerifyTokenRequest) (*identityv1.VerifyTokenResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.failVerify {
		return nil, status.Error(codes.Unauthenticated, "token inválido")
	}

	token := strings.TrimSpace(req.GetToken())
	if token == "" {
		return nil, status.Error(codes.InvalidArgument, "token vacío")
	}

	userID, ok := s.validTokens[token]
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "token inválido")
	}

	return &identityv1.VerifyTokenResponse{
		Valid:  true,
		UserId: userID,
	}, nil
}

// GetOAuthToken devuelve el access token de Google configurado para el
// usuario, o NotFound si no tiene vinculación.
func (s *Server) GetOAuthToken(ctx context.Context, req *identityv1.GetOAuthTokenRequest) (*identityv1.GetOAuthTokenResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	userID := strings.TrimSpace(req.GetUserId())
	if userID == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id vacío")
	}

	accessToken, ok := s.oauthTokens[userID]
	if !ok {
		return nil, status.Error(codes.NotFound, "calendar not connected")
	}

	return &identityv1.GetOAuthTokenResponse{
		AccessToken: accessToken,
	}, nil
}

// ReportCalendarRevoked marca el token de Calendar del usuario como
// revocado. Es idempotente y nunca falla por no existir.
func (s *Server) ReportCalendarRevoked(ctx context.Context, req *identityv1.ReportCalendarRevokedRequest) (*identityv1.ReportCalendarRevokedResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	userID := strings.TrimSpace(req.GetUserId())
	if userID == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id vacío")
	}

	// Idempotente: eliminar el token OAuth si existía.
	delete(s.oauthTokens, userID)

	return &identityv1.ReportCalendarRevokedResponse{}, nil
}
