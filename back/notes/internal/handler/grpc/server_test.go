package grpc

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	notesv1 "github.com/kurovoxx/tallerintegracion3/back/notes/internal/gen/notesv1/v1"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/interceptor"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const (
	testSecret      = "grpc-test-secret"
	testInternalKey = "grpc-test-internal-key"
	testIssuer      = "apuntes-auth"
	testAudience    = "apuntes-client"
)

// harness levanta un NotesService gRPC real sobre bufconn con stores en memoria
// y MockClient de Drive, más los interceptores de producción (JWT + interno).
type harness struct {
	client notesv1.NotesServiceClient
	svc    *service.NoteService
	drive  *drive.MockClient
	social *service.MemorySocialResolver
	md     *service.MemoryMemberDirectory
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	driveMock := drive.NewMockClient()
	noteStore := service.NewMemoryNoteStore()
	attStore := service.NewMemoryAttachmentStore()
	savedStore := service.NewMemorySavedStore()
	likeStore := service.NewMemoryLikeStore()
	likeStore.SetNoteStore(noteStore)
	sharedStore := service.NewMemorySharedStore()
	social := service.NewMemorySocialResolver()
	svc := service.NewNoteService(noteStore, attStore, savedStore, likeStore, sharedStore, driveMock, social)
	md := service.NewMemoryMemberDirectory()
	svc.SetMemberDirectory(md)
	svc.SetAccountDeletionRepository(&testAccountDeletionStore{})

	validator := &middleware.SimpleHS256Validator{
		Secret:   []byte(testSecret),
		Issuer:   testIssuer,
		Audience: testAudience,
	}
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
		interceptor.NewAuthRouter(
			interceptor.NewJWTAuth(validator),
			interceptor.NewInternalAuth(testInternalKey),
		),
	))
	notesv1.RegisterNotesServiceServer(srv, NewNotesServer(svc))

	lis := bufconn.Listen(1024 * 1024)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.GracefulStop)

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

	return &harness{
		client: notesv1.NewNotesServiceClient(conn),
		svc:    svc,
		drive:  driveMock,
		social: social,
		md:     md,
	}
}

type testAccountDeletionStore struct{}

func (*testAccountDeletionStore) Begin(context.Context, string) (bool, error) { return false, nil }
func (*testAccountDeletionStore) Complete(context.Context, string) error      { return nil }
func (*testAccountDeletionStore) SaveGroupWork(context.Context, string, string, []model.AccountDeletionGroupNote, bool) error {
	return nil
}
func (*testAccountDeletionStore) PendingGroupWork(context.Context, string, string) ([]model.AccountDeletionGroupNote, error) {
	return nil, nil
}
func (*testAccountDeletionStore) CompleteGroupWork(context.Context, string, string, string) error {
	return nil
}

func mintToken(t *testing.T, userID string) string {
	t.Helper()
	claims := middleware.ClaimsPersonalizadas{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    testIssuer,
			Audience:  jwt.ClaimStrings{testAudience},
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// userCtx adjunta el JWT del llamante en la metadata authorization.
func (h *harness) userCtx(t *testing.T, userID string) context.Context {
	t.Helper()
	return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+mintToken(t, userID))
}

func (h *harness) internalCtx(key string) context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), "x-internal-key", key)
}

func wantCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if got := status.Code(err); got != want {
		t.Fatalf("code = %s (err=%v); want %s", got, err, want)
	}
}

func strptr(s string) *string { return &s }

// --- Autenticación ---

func TestUserRPCWithoutAuthorizationIsUnauthenticated(t *testing.T) {
	h := newHarness(t)
	_, err := h.client.ListMyNotes(context.Background(), &notesv1.ListMyNotesRequest{})
	wantCode(t, err, codes.Unauthenticated)
}

func TestUserRPCWithInvalidTokenIsUnauthenticated(t *testing.T) {
	h := newHarness(t)
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer no-es-un-jwt")
	_, err := h.client.ListMyNotes(ctx, &notesv1.ListMyNotesRequest{})
	wantCode(t, err, codes.Unauthenticated)
}

func TestUserRPCWithValidTokenPropagatesUserID(t *testing.T) {
	h := newHarness(t)
	owner := uuid.NewString()
	ctx := h.userCtx(t, owner)
	resp, err := h.client.CreateNote(ctx, &notesv1.CreateNoteRequest{
		Title:      "con token",
		Visibility: "private",
		Content:    strptr("cuerpo"),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := h.client.GetNote(ctx, &notesv1.GetNoteRequest{Id: resp.GetNoteId()})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.GetNote().GetUserId() != owner {
		t.Fatalf("user_id no propagado desde el JWT: %q", got.GetNote().GetUserId())
	}
}

func TestUnshareAllWithoutInternalKeyIsUnauthenticated(t *testing.T) {
	h := newHarness(t)
	_, err := h.client.UnshareAll(context.Background(), &notesv1.UnshareAllRequest{
		UserId:  uuid.NewString(),
		GroupId: uuid.NewString(),
	})
	wantCode(t, err, codes.Unauthenticated)
}

func TestUnshareAllWithUserJWTIsUnauthenticated(t *testing.T) {
	h := newHarness(t)
	// Un JWT de usuario válido NO autentica el RPC interno: falta x-internal-key.
	ctx := h.userCtx(t, uuid.NewString())
	_, err := h.client.UnshareAll(ctx, &notesv1.UnshareAllRequest{
		UserId:  uuid.NewString(),
		GroupId: uuid.NewString(),
	})
	wantCode(t, err, codes.Unauthenticated)
}

func TestUnshareAllWithInvalidInternalKeyIsUnauthenticated(t *testing.T) {
	h := newHarness(t)
	_, err := h.client.UnshareAll(h.internalCtx("clave-incorrecta"), &notesv1.UnshareAllRequest{
		UserId:  uuid.NewString(),
		GroupId: uuid.NewString(),
	})
	wantCode(t, err, codes.Unauthenticated)
}

func TestUnshareAllWithCorrectInternalKeySucceeds(t *testing.T) {
	h := newHarness(t)
	_, err := h.client.UnshareAll(h.internalCtx(testInternalKey), &notesv1.UnshareAllRequest{
		UserId:  uuid.NewString(),
		GroupId: uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("unshare all: %v", err)
	}
}

// --- Metadata de notas ---

func TestCreateNotePersistsAndReturnsID(t *testing.T) {
	h := newHarness(t)
	owner := uuid.NewString()
	ctx := h.userCtx(t, owner)
	resp, err := h.client.CreateNote(ctx, &notesv1.CreateNoteRequest{
		Title:      "Apunte gRPC",
		Visibility: "private",
		Content:    strptr("# Hola"),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if resp.GetNoteId() == "" {
		t.Fatal("note_id vacío")
	}
	got, err := h.client.GetNote(ctx, &notesv1.GetNoteRequest{Id: resp.GetNoteId()})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	n := got.GetNote()
	if n.GetTitle() != "Apunte gRPC" || n.GetVisibility() != "private" {
		t.Fatalf("metadata inesperada: %+v", n)
	}
	if n.GetContent() != "# Hola" {
		t.Fatalf("contenido inesperado: %q", n.GetContent())
	}
	if n.GetExternalFileId() == "" || n.GetDriveUrl() == "" {
		t.Fatalf("falta external_file_id/drive_url: %+v", n)
	}
}

func TestGetNoteForeignAndMissingAreNotFound(t *testing.T) {
	h := newHarness(t)
	owner, stranger := uuid.NewString(), uuid.NewString()
	resp, err := h.client.CreateNote(h.userCtx(t, owner), &notesv1.CreateNoteRequest{
		Title: "privada", Visibility: "private", Content: strptr("x"),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Nota privada ajena: zero-knowledge -> mismo NotFound que inexistente.
	_, err = h.client.GetNote(h.userCtx(t, stranger), &notesv1.GetNoteRequest{Id: resp.GetNoteId()})
	wantCode(t, err, codes.NotFound)
	// UUID inexistente.
	_, err = h.client.GetNote(h.userCtx(t, owner), &notesv1.GetNoteRequest{Id: uuid.NewString()})
	wantCode(t, err, codes.NotFound)
}

func TestUpdateNoteOptimisticVersionConflict(t *testing.T) {
	h := newHarness(t)
	owner := uuid.NewString()
	ctx := h.userCtx(t, owner)
	created, err := h.client.CreateNote(ctx, &notesv1.CreateNoteRequest{
		Title: "editable", Visibility: "private", Content: strptr("v1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Primera edición con la versión vigente (1) -> sube a 2.
	upd, err := h.client.UpdateNote(ctx, &notesv1.UpdateNoteRequest{
		Id: created.GetNoteId(), Content: strptr("v2"), ExpectedVersion: 1,
	})
	if err != nil {
		t.Fatalf("update v1->v2: %v", err)
	}
	if upd.GetNote().GetVersion() != 2 {
		t.Fatalf("version tras update = %d; want 2", upd.GetNote().GetVersion())
	}
	// Segunda edición con la versión ya desactualizada (1) -> ABORTED.
	_, err = h.client.UpdateNote(ctx, &notesv1.UpdateNoteRequest{
		Id: created.GetNoteId(), Content: strptr("v3"), ExpectedVersion: 1,
	})
	wantCode(t, err, codes.Aborted)
}

func TestUpdateNoteByStrangerIsNotFound(t *testing.T) {
	h := newHarness(t)
	owner := uuid.NewString()
	created, err := h.client.CreateNote(h.userCtx(t, owner), &notesv1.CreateNoteRequest{
		Title: "ajena", Visibility: "private", Content: strptr("x"),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.client.UpdateNote(h.userCtx(t, uuid.NewString()), &notesv1.UpdateNoteRequest{
		Id: created.GetNoteId(), Title: strptr("hack"),
	})
	wantCode(t, err, codes.NotFound)
}

func TestDeleteNoteRemovesIt(t *testing.T) {
	h := newHarness(t)
	owner := uuid.NewString()
	ctx := h.userCtx(t, owner)
	created, err := h.client.CreateNote(ctx, &notesv1.CreateNoteRequest{
		Title: "borrar", Visibility: "private", Content: strptr("x"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.client.DeleteNote(ctx, &notesv1.DeleteNoteRequest{Id: created.GetNoteId()}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = h.client.GetNote(ctx, &notesv1.GetNoteRequest{Id: created.GetNoteId()})
	wantCode(t, err, codes.NotFound)
}

func TestDeleteNoteByStrangerIsNotFound(t *testing.T) {
	h := newHarness(t)
	owner := uuid.NewString()
	created, err := h.client.CreateNote(h.userCtx(t, owner), &notesv1.CreateNoteRequest{
		Title: "ajena", Visibility: "private", Content: strptr("x"),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.client.DeleteNote(h.userCtx(t, uuid.NewString()), &notesv1.DeleteNoteRequest{Id: created.GetNoteId()})
	wantCode(t, err, codes.NotFound)
}

// --- Compartidas y grupo ---

func TestShareNoteLinkAndRestrictedThenUnshare(t *testing.T) {
	h := newHarness(t)
	owner, group := uuid.NewString(), uuid.NewString()
	h.social.AddAdmin(owner, group)
	h.md.SetEmails(group, []string{"ana@example.com", "ben@example.com"})
	ctx := h.userCtx(t, owner)
	created, err := h.client.CreateNote(ctx, &notesv1.CreateNoteRequest{
		Title: "compartir", Visibility: "private", Content: strptr("x"),
	})
	if err != nil {
		t.Fatal(err)
	}
	link, err := h.client.ShareNote(ctx, &notesv1.ShareNoteRequest{
		NoteId: created.GetNoteId(), GroupId: group, AccessMode: "link",
	})
	if err != nil || link.GetSharedNoteId() == "" {
		t.Fatalf("share link: %v %v", link, err)
	}
	restricted, err := h.client.ShareNote(ctx, &notesv1.ShareNoteRequest{
		NoteId: created.GetNoteId(), GroupId: group, AccessMode: "restricted",
	})
	if err != nil || restricted.GetSharedNoteId() == "" {
		t.Fatalf("share restricted: %v %v", restricted, err)
	}
	if len(h.drive.GrantCalls) != 2 {
		t.Fatalf("restricted debe otorgar a los miembros: %v", h.drive.GrantCalls)
	}
	if _, err := h.client.UnshareNote(ctx, &notesv1.UnshareNoteRequest{SharedNoteId: restricted.GetSharedNoteId()}); err != nil {
		t.Fatalf("unshare: %v", err)
	}
	// El link sigue vigente: la nota es accesible por cualquiera.
	_, err = h.client.GetNote(h.userCtx(t, uuid.NewString()), &notesv1.GetNoteRequest{Id: created.GetNoteId()})
	if err != nil {
		t.Fatalf("link debe seguir dando acceso: %v", err)
	}
}

func TestShareNoteByNonMemberIsPermissionDenied(t *testing.T) {
	h := newHarness(t)
	owner, group := uuid.NewString(), uuid.NewString()
	// owner NO es miembro del grupo.
	ctx := h.userCtx(t, owner)
	created, err := h.client.CreateNote(ctx, &notesv1.CreateNoteRequest{
		Title: "sin grupo", Visibility: "private", Content: strptr("x"),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.client.ShareNote(ctx, &notesv1.ShareNoteRequest{
		NoteId: created.GetNoteId(), GroupId: group, AccessMode: "link",
	})
	wantCode(t, err, codes.PermissionDenied)
}

func TestListGroupNotesPaginationAndOrdering(t *testing.T) {
	h := newHarness(t)
	owner, group := uuid.NewString(), uuid.NewString()
	h.social.AddAdmin(owner, group)
	ctx := h.userCtx(t, owner)

	wantIDs := map[string]bool{}
	for i := 0; i < 3; i++ {
		created, err := h.client.CreateNote(ctx, &notesv1.CreateNoteRequest{
			Title: "grupo", Visibility: "private", Content: strptr("x"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.client.ShareNote(ctx, &notesv1.ShareNoteRequest{
			NoteId: created.GetNoteId(), GroupId: group, AccessMode: "link",
		}); err != nil {
			t.Fatal(err)
		}
		wantIDs[created.GetNoteId()] = true
	}

	seen := map[string]bool{}
	pageToken := ""
	pages := 0
	for {
		resp, err := h.client.ListGroupNotes(ctx, &notesv1.ListGroupNotesRequest{
			GroupId: group,
			Page:    &notesv1.PageRequest{PageSize: 2, PageToken: pageToken},
		})
		if err != nil {
			t.Fatalf("list page %d: %v", pages, err)
		}
		pages++
		if len(resp.GetNotes()) > 2 {
			t.Fatalf("page_size=2 devolvió %d", len(resp.GetNotes()))
		}
		for _, nws := range resp.GetNotes() {
			if nws.GetShare() == nil || nws.GetShare().GetGroupId() != group {
				t.Fatalf("share inesperado: %+v", nws.GetShare())
			}
			if !wantIDs[nws.GetNote().GetId()] {
				t.Fatalf("nota ajena en el listado: %s", nws.GetNote().GetId())
			}
			seen[nws.GetNote().GetId()] = true
		}
		pageToken = resp.GetPage().GetNextPageToken()
		if pageToken == "" {
			break
		}
		if pages > 5 {
			t.Fatal("paginación sin fin")
		}
	}
	if len(seen) != 3 {
		t.Fatalf("paginación incompleta: %d/3", len(seen))
	}
	if pages != 2 {
		t.Fatalf("esperaba 2 páginas (2+1), got %d", pages)
	}
}

func TestListGroupNotesNonMemberIsPermissionDenied(t *testing.T) {
	h := newHarness(t)
	owner, group := uuid.NewString(), uuid.NewString()
	h.social.AddAdmin(owner, group)
	created, err := h.client.CreateNote(h.userCtx(t, owner), &notesv1.CreateNoteRequest{
		Title: "grupo", Visibility: "private", Content: strptr("x"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.client.ShareNote(h.userCtx(t, owner), &notesv1.ShareNoteRequest{
		NoteId: created.GetNoteId(), GroupId: group, AccessMode: "link",
	}); err != nil {
		t.Fatal(err)
	}
	_, err = h.client.ListGroupNotes(h.userCtx(t, uuid.NewString()), &notesv1.ListGroupNotesRequest{GroupId: group})
	wantCode(t, err, codes.PermissionDenied)
}

func TestGetNoteAccessResolvesWithoutDownloading(t *testing.T) {
	h := newHarness(t)
	owner, stranger := uuid.NewString(), uuid.NewString()
	ctx := h.userCtx(t, owner)
	created, err := h.client.CreateNote(ctx, &notesv1.CreateNoteRequest{
		Title: "acceso", Visibility: "private", Content: strptr("secreto"),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Autor: owner + can_write, sin descargar (el contenido no viaja aquí).
	acc, err := h.client.GetNoteAccess(ctx, &notesv1.GetNoteAccessRequest{Id: created.GetNoteId()})
	if err != nil {
		t.Fatalf("access owner: %v", err)
	}
	if a := acc.GetAccess(); a.GetAccessMode() != "owner" || !a.GetCanWrite() || !a.GetCanRead() {
		t.Fatalf("access owner inesperado: %+v", a)
	}
	// Tercero sobre nota privada: NotFound zero-knowledge.
	_, err = h.client.GetNoteAccess(h.userCtx(t, stranger), &notesv1.GetNoteAccessRequest{Id: created.GetNoteId()})
	wantCode(t, err, codes.NotFound)
	// Pasar a pública: tercero accede en modo public.
	if _, err := h.client.UpdateNote(ctx, &notesv1.UpdateNoteRequest{
		Id: created.GetNoteId(), Visibility: strptr("public"), ExpectedVersion: 1,
	}); err != nil {
		t.Fatalf("update public: %v", err)
	}
	acc, err = h.client.GetNoteAccess(h.userCtx(t, stranger), &notesv1.GetNoteAccessRequest{Id: created.GetNoteId()})
	if err != nil {
		t.Fatalf("access stranger public: %v", err)
	}
	if a := acc.GetAccess(); a.GetAccessMode() != "public" || !a.GetCanRead() || a.GetCanWrite() {
		t.Fatalf("access public inesperado: %+v", a)
	}
}
