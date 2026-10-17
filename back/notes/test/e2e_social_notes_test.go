// Package test contiene la suite E2E cruzada Notes ↔ Social.
//
// A diferencia de los tests unitarios (que usan MemorySocialResolver), aquí se
// ejercita la integración REAL de Notes:
//
//   - Notes → Social: el SocialHTTPAdapter de producción (social_adapter.go)
//     consume un servidor HTTP que implementa el contrato interno
//     (/internal/groups/{id}/members, /member-emails, /users/{id}/followers-count),
//     tal como lo haría el Social real.
//   - Social → Notes: el servidor gRPC de Notes (NotesService) real sobre
//     bufconn recibe UnshareAll autenticado con x-internal-key, exactamente la
//     llamada que emite Social al salir/expulsar a un miembro (notes_grpc.go).
//
// El resto del stack es el de producción: NoteService con stores en memoria,
// MockClient de Drive (grants/revokes nominales) y los interceptores gRPC.
package test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	notesv1 "github.com/kurovoxx/tallerintegracion3/back/notes/internal/gen/notesv1/v1"
	grpcHandler "github.com/kurovoxx/tallerintegracion3/back/notes/internal/handler/grpc"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/interceptor"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const (
	e2eSecret      = "e2e-jwt-secret"
	e2eInternalKey = "e2e-internal-key"
	e2eIssuer      = "apuntes-auth"
	e2eAudience    = "apuntes-client"
)

// --- Fake Social: implementa el contrato HTTP interno consumido por Notes ---

type socialMember struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

type fakeSocial struct {
	mu          sync.Mutex
	members     map[string][]socialMember
	emails      map[string][]string
	followers   map[string]int
	failMembers bool // simula Social caído en /members (500)
}

func newFakeSocial() *fakeSocial {
	return &fakeSocial{
		members:   map[string][]socialMember{},
		emails:    map[string][]string{},
		followers: map[string]int{},
	}
}

func (f *fakeSocial) addMember(groupID, userID, role string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.members[groupID] = append(f.members[groupID], socialMember{UserID: userID, Role: role})
}

func (f *fakeSocial) setEmails(groupID string, emails []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.emails[groupID] = emails
}

func (f *fakeSocial) setFailMembers(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failMembers = v
}

func (f *fakeSocial) serveHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 || parts[0] != "internal" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	switch {
	case parts[1] == "groups" && parts[3] == "members":
		if f.failMembers {
			http.Error(w, "social down", http.StatusInternalServerError)
			return
		}
		writeJSON(w, f.members[parts[2]])
	case parts[1] == "groups" && parts[3] == "member-emails":
		writeJSON(w, f.emails[parts[2]])
	case parts[1] == "users" && parts[3] == "followers-count":
		writeJSON(w, map[string]int{"followers_count": f.followers[parts[2]]})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// --- Harness E2E ---

type e2e struct {
	social *fakeSocial
	svc    *service.NoteService
	shared *service.MemorySharedStore
	drive  *drive.MockClient
	client notesv1.NotesServiceClient
}

func newE2E(t *testing.T) *e2e {
	t.Helper()

	fs := newFakeSocial()
	socialSrv := httptest.NewServer(http.HandlerFunc(fs.serveHTTP))
	t.Cleanup(socialSrv.Close)
	// Adaptador HTTP REAL de Notes apuntando al fake Social.
	adapter := service.NewSocialHTTPAdapter(socialSrv.URL, 5*time.Second, e2eInternalKey)

	driveMock := drive.NewMockClient()
	noteStore := service.NewMemoryNoteStore()
	attStore := service.NewMemoryAttachmentStore()
	savedStore := service.NewMemorySavedStore()
	likeStore := service.NewMemoryLikeStore()
	likeStore.SetNoteStore(noteStore)
	sharedStore := service.NewMemorySharedStore()

	svc := service.NewNoteService(noteStore, attStore, savedStore, likeStore, sharedStore, driveMock, adapter)
	svc.SetMemberDirectory(adapter)

	validator := &middleware.SimpleHS256Validator{
		Secret:   []byte(e2eSecret),
		Issuer:   e2eIssuer,
		Audience: e2eAudience,
	}
	grpcSrv := grpc.NewServer(grpc.ChainUnaryInterceptor(
		interceptor.NewAuthRouter(
			interceptor.NewJWTAuth(validator),
			interceptor.NewInternalAuth(e2eInternalKey),
		),
	))
	notesv1.RegisterNotesServiceServer(grpcSrv, grpcHandler.NewNotesServer(svc))

	lis := bufconn.Listen(1024 * 1024)
	go func() { _ = grpcSrv.Serve(lis) }()
	t.Cleanup(grpcSrv.GracefulStop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return &e2e{
		social: fs,
		svc:    svc,
		shared: sharedStore,
		drive:  driveMock,
		client: notesv1.NewNotesServiceClient(conn),
	}
}

func (e *e2e) userCtx(t *testing.T, userID string) context.Context {
	t.Helper()
	claims := middleware.ClaimsPersonalizadas{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    e2eIssuer,
			Audience:  jwt.ClaimStrings{e2eAudience},
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(e2eSecret))
	if err != nil {
		t.Fatal(err)
	}
	return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+tok)
}

// internalCtx replica la llamada servicio-a-servicio de Social (x-internal-key).
func (e *e2e) internalCtx() context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), "x-internal-key", e2eInternalKey)
}

func (e *e2e) createNote(t *testing.T, owner, title string) string {
	t.Helper()
	resp, err := e.client.CreateNote(e.userCtx(t, owner), &notesv1.CreateNoteRequest{
		Title: title, Visibility: "private", Content: strptr("contenido"),
	})
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	return resp.GetNoteId()
}

func strptr(s string) *string { return &s }

func wantCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if got := status.Code(err); got != want {
		t.Fatalf("code = %s (err=%v); want %s", got, err, want)
	}
}

func revokedEmails(calls []drive.GrantCall) map[string]bool {
	out := map[string]bool{}
	for _, c := range calls {
		if c.Email != "" {
			out[c.Email] = true
		}
	}
	return out
}

// --- Tests E2E ---

// Flujo integral restricted: compartir consulta membresía + correos, otorga
// permisos nominales en Drive y persistirlos; la salida del miembro (UnshareAll
// vía gRPC interno) limpia shared_notes y revoca nomalmente los permisos.
func TestE2EShareRestrictedThenUnshareAllCleansUp(t *testing.T) {
	e := newE2E(t)
	ctx := context.Background()
	owner, group := uuid.NewString(), uuid.NewString()
	e.social.addMember(group, owner, "admin")
	e.social.setEmails(group, []string{"ana@example.com", "ben@example.com"})

	noteID := e.createNote(t, owner, "apunte E2E restricted")

	// 1) Compartir restricted: consulta Social y otorga permisos nominales.
	sh, err := e.client.ShareNote(e.userCtx(t, owner), &notesv1.ShareNoteRequest{
		NoteId: noteID, GroupId: group, AccessMode: "restricted",
	})
	if err != nil {
		t.Fatalf("share restricted: %v", err)
	}
	if sh.GetSharedNoteId() == "" {
		t.Fatal("shared_note_id vacío")
	}
	if len(e.drive.GrantCalls) != 2 {
		t.Fatalf("esperaba 2 grants nominales, got %v", e.drive.GrantCalls)
	}
	for _, gc := range e.drive.GrantCalls {
		if gc.Role != "reader" || gc.PermissionID == "" {
			t.Fatalf("grant sin role=reader/permission_id: %+v", gc)
		}
	}
	rows, _ := e.shared.ListManagedPermissions(ctx, noteID)
	if len(rows) != 2 {
		t.Fatalf("esperaba 2 filas de permisos administrados, got %+v", rows)
	}
	shares, _, _ := e.shared.ListByGroup(ctx, group, 10, "")
	if len(shares) != 1 {
		t.Fatalf("el share no quedó persistido: %+v", shares)
	}

	// 2) El miembro sale del grupo: Social dispara UnshareAll (gRPC interno).
	if _, err := e.client.UnshareAll(e.internalCtx(), &notesv1.UnshareAllRequest{
		UserId: owner, GroupId: group,
	}); err != nil {
		t.Fatalf("unshare all: %v", err)
	}

	// 3) Limpieza: sin shares, sin filas de permisos y revocación nominal.
	shares, _, _ = e.shared.ListByGroup(ctx, group, 10, "")
	if len(shares) != 0 {
		t.Fatalf("shared_notes no limpiada: %+v", shares)
	}
	rows, _ = e.shared.ListManagedPermissions(ctx, noteID)
	if len(rows) != 0 {
		t.Fatalf("drive_managed_permissions no limpiada: %+v", rows)
	}
	revoked := revokedEmails(e.drive.RevokeCalls)
	if !revoked["ana@example.com"] || !revoked["ben@example.com"] {
		t.Fatalf("no se revocaron los permisos nominales: %v", e.drive.RevokeCalls)
	}
}

// Flujo link: se otorga permiso anyone y UnshareAll lo revoca por ID.
func TestE2EShareLinkThenUnshareAllRevokesLink(t *testing.T) {
	e := newE2E(t)
	ctx := context.Background()
	owner, group := uuid.NewString(), uuid.NewString()
	e.social.addMember(group, owner, "admin")

	noteID := e.createNote(t, owner, "apunte E2E link")
	if _, err := e.client.ShareNote(e.userCtx(t, owner), &notesv1.ShareNoteRequest{
		NoteId: noteID, GroupId: group, AccessMode: "link",
	}); err != nil {
		t.Fatalf("share link: %v", err)
	}
	if len(e.drive.LinkGrantCalls) != 1 {
		t.Fatalf("esperaba 1 link grant, got %v", e.drive.LinkGrantCalls)
	}
	if _, err := e.client.UnshareAll(e.internalCtx(), &notesv1.UnshareAllRequest{
		UserId: owner, GroupId: group,
	}); err != nil {
		t.Fatalf("unshare all: %v", err)
	}
	shares, _, _ := e.shared.ListByGroup(ctx, group, 10, "")
	if len(shares) != 0 {
		t.Fatalf("share link no limpiado: %+v", shares)
	}
	if len(e.drive.RevokeIDCalls) == 0 {
		t.Fatalf("el permiso link no se revocó: %v", e.drive.RevokeIDCalls)
	}
}

// Fail-closed: si Social falla al validar membresía, ShareNote falla de forma
// controlada y NO deja estado huérfano (ni share ni permisos en Drive).
func TestE2EShareFailClosedWhenSocialDown(t *testing.T) {
	e := newE2E(t)
	ctx := context.Background()
	owner, group := uuid.NewString(), uuid.NewString()
	e.social.addMember(group, owner, "admin")
	e.social.setFailMembers(true) // Social caído en /members

	noteID := e.createNote(t, owner, "apunte E2E fail-closed")

	_, err := e.client.ShareNote(e.userCtx(t, owner), &notesv1.ShareNoteRequest{
		NoteId: noteID, GroupId: group, AccessMode: "link",
	})
	wantCode(t, err, codes.Internal)

	// Sin estado huérfano.
	shares, _, _ := e.shared.ListByGroup(ctx, group, 10, "")
	if len(shares) != 0 {
		t.Fatalf("share persistido tras fallo de Social: %+v", shares)
	}
	rows, _ := e.shared.ListManagedPermissions(ctx, noteID)
	if len(rows) != 0 {
		t.Fatalf("permisos creados tras fallo de Social: %+v", rows)
	}
	if len(e.drive.LinkGrantCalls) != 0 || len(e.drive.GrantCalls) != 0 || len(e.drive.RevokeIDCalls) != 0 {
		t.Fatalf("Drive mutó sin intención durable: links=%v grants=%v revokes=%v",
			e.drive.LinkGrantCalls, e.drive.GrantCalls, e.drive.RevokeIDCalls)
	}
}

// Si Social no reconoce al autor como miembro (grupo inexistente/vacío), el
// share se rechaza con PermissionDenied y no persiste nada.
func TestE2EShareNotMemberIsPermissionDenied(t *testing.T) {
	e := newE2E(t)
	ctx := context.Background()
	owner, group := uuid.NewString(), uuid.NewString()
	// El autor no está en la lista de miembros del grupo.
	noteID := e.createNote(t, owner, "apunte E2E no-miembro")

	_, err := e.client.ShareNote(e.userCtx(t, owner), &notesv1.ShareNoteRequest{
		NoteId: noteID, GroupId: group, AccessMode: "link",
	})
	wantCode(t, err, codes.PermissionDenied)

	shares, _, _ := e.shared.ListByGroup(ctx, group, 10, "")
	if len(shares) != 0 {
		t.Fatalf("share persistido sin membresía: %+v", shares)
	}
	if len(e.drive.LinkGrantCalls) != 0 {
		t.Fatalf("Drive mutó sin membresía: %v", e.drive.LinkGrantCalls)
	}
}

// Idempotencia: UnshareAll sobre un grupo sin notas compartidas o con un
// usuario inexistente responde OK y puede repetirse sin efectos.
func TestE2EUnshareAllIdempotentWithoutShares(t *testing.T) {
	e := newE2E(t)
	user, group := uuid.NewString(), uuid.NewString()
	for i := 0; i < 2; i++ {
		if _, err := e.client.UnshareAll(e.internalCtx(), &notesv1.UnshareAllRequest{
			UserId: user, GroupId: group,
		}); err != nil {
			t.Fatalf("intento %d: unshare all sin shares debe ser OK: %v", i, err)
		}
	}
	// Usuario inexistente pero bien formado: también OK.
	if _, err := e.client.UnshareAll(e.internalCtx(), &notesv1.UnshareAllRequest{
		UserId: uuid.NewString(), GroupId: group,
	}); err != nil {
		t.Fatalf("unshare all de usuario inexistente debe ser OK: %v", err)
	}
}

// Expulsión masiva: UnshareAll limpia todos los shares del usuario en el grupo
// sin tocar los de otro usuario en el mismo grupo.
func TestE2EUnshareAllOnlyAffectsTargetUser(t *testing.T) {
	e := newE2E(t)
	ctx := context.Background()
	owner, other, group := uuid.NewString(), uuid.NewString(), uuid.NewString()
	e.social.addMember(group, owner, "admin")
	e.social.addMember(group, other, "admin")

	ownerNote := e.createNote(t, owner, "nota owner")
	otherNote := e.createNote(t, other, "nota other")
	for _, s := range []struct{ uid, note string }{{owner, ownerNote}, {other, otherNote}} {
		if _, err := e.client.ShareNote(e.userCtx(t, s.uid), &notesv1.ShareNoteRequest{
			NoteId: s.note, GroupId: group, AccessMode: "link",
		}); err != nil {
			t.Fatalf("share %s: %v", s.uid, err)
		}
	}

	if _, err := e.client.UnshareAll(e.internalCtx(), &notesv1.UnshareAllRequest{
		UserId: owner, GroupId: group,
	}); err != nil {
		t.Fatalf("unshare all owner: %v", err)
	}
	shares, _, _ := e.shared.ListByGroup(ctx, group, 10, "")
	if len(shares) != 1 || shares[0].NoteID != otherNote {
		t.Fatalf("debe quedar solo el share del otro usuario: %+v", shares)
	}
}
