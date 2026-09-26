package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
)

func TestGoBestEffort_EjecutaEnBackground(t *testing.T) {
	done := make(chan struct{})
	GoBestEffort("test", func(ctx context.Context) {
		defer close(done)
		if ctx == nil {
			t.Error("ctx no debe ser nil")
		}
	})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("GoBestEffort no ejecutó la función")
	}
}

func TestGoBestEffort_PanicNoTumba(t *testing.T) {
	done := make(chan struct{})
	GoBestEffort("test-panic", func(ctx context.Context) {
		defer close(done)
		panic("boom simulado")
	})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("el runner debe sobrevivir al panic")
	}
	// Si el panic se propagara, el proceso de tests moriría: llegar aquí es el assert.
}

func TestRunBestEffortChild_Reporta(t *testing.T) {
	if !RunBestEffortChild("ok", context.Background(), func(ctx context.Context) {}) {
		t.Fatal("sin panic debe reportar true")
	}
	if RunBestEffortChild("panico", context.Background(), func(ctx context.Context) {
		panic("boom")
	}) {
		t.Fatal("con panic debe reportar false")
	}
}

type countingNotifier struct {
	calls  *int32
	block  time.Duration
	panics bool
}

func (c *countingNotifier) OnMeetingCreated(ctx context.Context, meeting sqlc.SocialMeeting) {
	if c.panics {
		panic("notifier roto")
	}
	if c.block > 0 {
		select {
		case <-time.After(c.block):
		case <-ctx.Done():
			return
		}
	}
	atomic.AddInt32(c.calls, 1)
}

func TestMultiMeetingNotifier_ParaleloNoBloquea(t *testing.T) {
	old := bestEffortTimeout
	bestEffortTimeout = 100 * time.Millisecond
	defer func() { bestEffortTimeout = old }()

	var fastCalls, slowCalls int32
	fast := &countingNotifier{calls: &fastCalls}
	// Lento pero bien portado: respeta ctx y dormiría 300ms sin timeout.
	slow := &countingNotifier{calls: &slowCalls, block: 300 * time.Millisecond}
	m := NewMultiMeetingNotifier(slow, fast)

	start := time.Now()
	m.OnMeetingCreated(context.Background(), sqlc.SocialMeeting{})
	elapsed := time.Since(start)

	if atomic.LoadInt32(&fastCalls) != 1 {
		t.Fatalf("el rápido debe ejecutarse, got %d", fastCalls)
	}
	if atomic.LoadInt32(&slowCalls) != 0 {
		t.Fatalf("el timeout debe cortar al lento antes de completar, got %d", slowCalls)
	}
	// El timeout (100ms) debe cortar al lento (300ms): secuencial tomaría 300ms+.
	if elapsed > 250*time.Millisecond {
		t.Fatalf("el fan-out debe estar acotado por timeout, tomó %v", elapsed)
	}
}

func TestMultiMeetingNotifier_PanicAislado(t *testing.T) {
	var calls int32
	roto := &countingNotifier{panics: true}
	sano := &countingNotifier{calls: &calls}
	m := NewMultiMeetingNotifier(roto, sano, nil)

	m.OnMeetingCreated(context.Background(), sqlc.SocialMeeting{})

	// Esperar al sano (corre en goroutine).
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&calls) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatal("el panic de un hijo no debe afectar a los demás")
	}
}
