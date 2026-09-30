package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/stream"
)

var errStreamBoom = errors.New("boom stream")

// Escenario: admin crea el grupo (canal existe en Stream) y luego se une un
// segundo miembro. Ambos deben quedar como miembros del canal al pedir su
// token, sin tocar nada global de Stream.

func syncEnv() (gid, admin, member, lateJoiner string, store *MemoryMeetingStore, mock *stream.MockClient, svc *StreamTokenService) {
	gid, admin, member, lateJoiner = uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	store = NewMemoryMeetingStore()
	store.AddGroup(gid, admin, member)
	store.SetStreamChannel(gid, ChannelIDForGroup(gid))
	mock = stream.NewMockClient()
	svc = NewStreamTokenServiceWithKey(store, "test-secret", "test-key")
	svc.SetClient(mock)
	return gid, admin, member, lateJoiner, store, mock, svc
}

func ensuredHas(t *testing.T, mock *stream.MockClient, channelID, userID string) {
	t.Helper()
	for _, c := range mock.Ensured {
		if c.ChannelType == ChannelTypeStream && c.ChannelID == channelID && c.UserID == userID {
			return
		}
	}
	t.Fatalf("usuario %s no quedó como miembro de %s: %+v", userID, channelID, mock.Ensured)
}

// A. Miembro real obtiene token y membresía Stream del canal de SU grupo.
func TestStreamTokenSync_MiembroQuedaEnCanal(t *testing.T) {
	gid, _, member, _, _, mock, svc := syncEnv()
	token, channelID, _, err := svc.IssueTokenWithKey(context.Background(), gid, member)
	if err != nil {
		t.Fatalf("miembro debe obtener token: %v", err)
	}
	if token == "" {
		t.Fatal("token vacío")
	}
	ensuredHas(t, mock, channelID, member)
}

// B. Segundo miembro unido DESPUÉS de creado el canal también entra.
func TestStreamTokenSync_MiembroTardioQuedaEnCanal(t *testing.T) {
	gid, admin, member, lateJoiner, store, mock, svc := syncEnv()
	// El admin pide primero (canal ya sincronizado para él).
	if _, _, _, err := svc.IssueTokenWithKey(context.Background(), gid, admin); err != nil {
		t.Fatalf("admin: %v", err)
	}
	// Luego se une otro miembro al grupo Sigma (sin canal previo en Stream).
	store.AddGroup(gid, admin, member, lateJoiner)
	if _, _, _, err := svc.IssueTokenWithKey(context.Background(), gid, lateJoiner); err != nil {
		t.Fatalf("miembro tardío debe obtener token: %v", err)
	}
	ensuredHas(t, mock, ChannelIDForGroup(gid), lateJoiner)
}

// C. Pedir el token N veces es idempotente (sin error ni duplicados).
func TestStreamTokenSync_Idempotente(t *testing.T) {
	gid, _, member, _, _, mock, svc := syncEnv()
	for range 3 {
		if _, _, _, err := svc.IssueTokenWithKey(context.Background(), gid, member); err != nil {
			t.Fatalf("reintento debe funcionar: %v", err)
		}
	}
	count := 0
	for _, c := range mock.Ensured {
		if c.UserID == member {
			count++
		}
	}
	if count != 3 {
		t.Fatalf("cada emisión sincroniza una vez, got %d", count)
	}
}

// D. Ajeno recibe 403 y Stream ni se toca.
func TestStreamTokenSync_AjenoNoTocaStream(t *testing.T) {
	gid, _, _, _, _, mock, svc := syncEnv()
	stranger := uuid.NewString()
	if _, _, _, err := svc.IssueTokenWithKey(context.Background(), gid, stranger); err != ErrForbidden {
		t.Fatalf("ajeno debe dar ErrForbidden, got %v", err)
	}
	if len(mock.Ensured) != 0 || len(mock.Calls) != 0 {
		t.Fatalf("Stream no debe tocarse para ajenos: %+v %+v", mock.Ensured, mock.Calls)
	}
}

// Fallo de Stream al sincronizar: se reporta como sync (503 en handler),
// no como token válido que luego daría 403 críptico en el SDK.
func TestStreamTokenSync_FalloStreamEsError(t *testing.T) {
	gid, _, member, _, _, mock, svc := syncEnv()
	mock.EnsureErr = errStreamBoom
	if _, _, _, err := svc.IssueTokenWithKey(context.Background(), gid, member); err == nil {
		t.Fatal("fallo de Stream debe ser error")
	} else if !errors.Is(err, ErrStreamSyncFailed) {
		t.Fatalf("debe ser ErrStreamSyncFailed, got %v", err)
	}
}
