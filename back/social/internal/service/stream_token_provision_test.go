package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/stream"
)

// fakeRemoteStream simula el estado REMOTO de Stream: canales que existen o
// no, miembros por canal y registro de operaciones. No tiene operación de
// borrado a propósito: el código bajo prueba jamás debe borrar nada.
type fakeRemoteStream struct {
	mu       sync.Mutex
	channels map[string]bool
	members  map[string]map[string]bool
	ops      []string // "create:ID" / "ensure:ID:USER"
	createErr error
}

func newFakeRemote() *fakeRemoteStream {
	return &fakeRemoteStream{
		channels: map[string]bool{},
		members:  map[string]map[string]bool{},
	}
}

func (f *fakeRemoteStream) CreateChannel(_ context.Context, channelType, channelID, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ops = append(f.ops, "create:"+channelID)
	if f.createErr != nil {
		return f.createErr
	}
	// Get-or-create: si ya existe lo retorna sin tocar nada.
	f.channels[channelID] = true
	return nil
}

func (f *fakeRemoteStream) EnsureMember(_ context.Context, _, channelID, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ops = append(f.ops, "ensure:"+channelID+":"+userID)
	if !f.channels[channelID] {
		return &stream.StreamError{Code: 404, Message: `{"message":"channel not found"}`}
	}
	if f.members[channelID] == nil {
		f.members[channelID] = map[string]bool{}
	}
	f.members[channelID][userID] = true
	return nil
}

func (f *fakeRemoteStream) SendMessage(_ context.Context, _, _, _, _ string) error {
	return nil
}

func (f *fakeRemoteStream) isMember(channelID, userID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.members[channelID][userID]
}

func provisionEnv(withRow bool) (gid, admin, member string, store *MemoryMeetingStore, remote *fakeRemoteStream, svc *StreamTokenService) {
	gid, admin, member = uuid.NewString(), uuid.NewString(), uuid.NewString()
	store = NewMemoryMeetingStore()
	store.AddGroup(gid, admin, member)
	if withRow {
		store.SetStreamChannel(gid, ChannelIDForGroup(gid))
	}
	remote = newFakeRemote()
	svc = NewStreamTokenServiceWithKey(store, "test-secret", "test-key")
	svc.SetClient(remote)
	return gid, admin, member, store, remote, svc
}

// 1. Canal remoto ya existente: sigue funcionando, sin borrados ni cambios.
func TestProvision_CanalExistenteSigueFuncionando(t *testing.T) {
	gid, _, member, _, remote, svc := provisionEnv(true)
	remote.channels[ChannelIDForGroup(gid)] = true
	if _, _, _, err := svc.IssueTokenWithKey(context.Background(), gid, member); err != nil {
		t.Fatalf("canal existente debe funcionar: %v", err)
	}
	if !remote.isMember(ChannelIDForGroup(gid), member) {
		t.Fatal("miembro debe quedar en el canal")
	}
	for _, op := range remote.ops {
		if len(op) >= 6 && op[:6] == "delete" {
			t.Fatalf("jamás borrar: %v", remote.ops)
		}
	}
}

// 2. Sin canal remoto: se crea al pedir stream-token (self-healing).
func TestProvision_SinCanalRemotoSeCrea(t *testing.T) {
	gid, _, member, _, remote, svc := provisionEnv(true)
	if _, _, _, err := svc.IssueTokenWithKey(context.Background(), gid, member); err != nil {
		t.Fatalf("debe autocrear el canal: %v", err)
	}
	if !remote.channels[ChannelIDForGroup(gid)] {
		t.Fatal("el canal remoto debe existir tras el token")
	}
	if !remote.isMember(ChannelIDForGroup(gid), member) {
		t.Fatal("miembro debe quedar en el canal creado")
	}
}

// 3. Mapping DB existente pero canal remoto ausente: se recupera con el MISMO id.
func TestProvision_MappingHuerfanoSeRecupera(t *testing.T) {
	gid, _, member, _, remote, svc := provisionEnv(true)
	token, channelID, _, err := svc.IssueTokenWithKey(context.Background(), gid, member)
	if err != nil {
		t.Fatalf("mapping huérfano debe recuperarse: %v", err)
	}
	if token == "" || channelID != ChannelIDForGroup(gid) {
		t.Fatalf("channel id debe conservarse: %q", channelID)
	}
	remote.mu.Lock()
	exists := remote.channels[channelID]
	remote.mu.Unlock()
	if !exists {
		t.Fatal("el canal remoto debe haberse creado")
	}
}

// 4. Grupo nuevo (sin fila DB): provisioning completo con id determinístico.
func TestProvision_GrupoNuevoCompleto(t *testing.T) {
	gid, _, member, _, remote, svc := provisionEnv(false)
	_, channelID, _, err := svc.IssueTokenWithKey(context.Background(), gid, member)
	if err != nil {
		t.Fatalf("grupo nuevo debe provisionarse: %v", err)
	}
	if channelID != ChannelIDForGroup(gid) {
		t.Fatalf("id determinístico esperado, got %q", channelID)
	}
	if !remote.isMember(channelID, member) {
		t.Fatal("miembro debe quedar en el canal nuevo")
	}
}

// 5. Llamada repetida: idempotente, canal y mensajes intactos.
func TestProvision_RepetidaIdempotente(t *testing.T) {
	gid, _, member, _, remote, svc := provisionEnv(false)
	for range 3 {
		if _, _, _, err := svc.IssueTokenWithKey(context.Background(), gid, member); err != nil {
			t.Fatalf("reintento debe funcionar: %v", err)
		}
	}
	if !remote.channels[ChannelIDForGroup(gid)] || !remote.isMember(ChannelIDForGroup(gid), member) {
		t.Fatal("canal y membresía deben seguir intactos")
	}
}

// 6. Segundo miembro posterior: se agrega y funciona.
func TestProvision_SegundoMiembroTardio(t *testing.T) {
	gid, admin, member, store, remote, svc := provisionEnv(false)
	late := uuid.NewString()
	if _, _, _, err := svc.IssueTokenWithKey(context.Background(), gid, member); err != nil {
		t.Fatalf("primero: %v", err)
	}
	store.AddGroup(gid, admin, member, late)
	if _, _, _, err := svc.IssueTokenWithKey(context.Background(), gid, late); err != nil {
		t.Fatalf("tardío: %v", err)
	}
	if !remote.isMember(ChannelIDForGroup(gid), late) {
		t.Fatal("tardío debe quedar miembro")
	}
}

// 7. Ajeno: 403 y cero operación remota.
func TestProvision_Ajeno403SinOperacion(t *testing.T) {
	gid, _, _, _, remote, svc := provisionEnv(false)
	if _, _, _, err := svc.IssueTokenWithKey(context.Background(), gid, uuid.NewString()); err != ErrForbidden {
		t.Fatalf("ajeno debe dar ErrForbidden, got %v", err)
	}
	if len(remote.ops) != 0 {
		t.Fatalf("cero operación remota para ajenos: %v", remote.ops)
	}
}

// 8. Fallo real de CreateChannel: 503 controlado, sin token inválido.
func TestProvision_FalloCreateEsSyncFailed(t *testing.T) {
	gid, _, member, _, remote, svc := provisionEnv(false)
	remote.createErr = &stream.StreamError{Code: 500, Message: "caído"}
	token, _, _, err := svc.IssueTokenWithKey(context.Background(), gid, member)
	if token != "" {
		t.Fatal("sin canal no debe entregar token")
	}
	if !errors.Is(err, ErrStreamSyncFailed) {
		t.Fatalf("debe ser ErrStreamSyncFailed, got %v", err)
	}
}

// 9. Jamás se sobrescriben/borran channels: solo create (get-or-create) y
// ensure-member existen como operaciones remotas.
func TestProvision_SinOperacionesDestructivas(t *testing.T) {
	gid, _, member, _, remote, svc := provisionEnv(true)
	remote.channels[ChannelIDForGroup(gid)] = true
	if _, _, _, err := svc.IssueTokenWithKey(context.Background(), gid, member); err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, op := range remote.ops {
		if len(op) < 6 || (op[:6] != "create" && op[:6] != "ensure") {
			t.Fatalf("operación inesperada: %q en %v", op, remote.ops)
		}
	}
}
