package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
)

// Límite de envíos simultáneos (dispatchSend): 4 cupos compartidos entre
// registro y reenvío, ocupados antes de crear la goroutine.

// blockingMailer retiene cada envío hasta que se cierra release y mide la
// concurrencia real dentro del mailer.
type blockingMailer struct {
	release  chan struct{}
	entered  chan struct{}
	inFlight atomic.Int32
	maxSeen  atomic.Int32
	sent     atomic.Int32
	err      error
	panicky  bool
	once     sync.Once
}

// releaseAll deja pasar los envíos retenidos y los futuros (idempotente).
func (m *blockingMailer) releaseAll() { m.once.Do(func() { close(m.release) }) }

func newBlockingMailer() *blockingMailer {
	return &blockingMailer{release: make(chan struct{}), entered: make(chan struct{}, 200)}
}

func trackMax(inFlight, maxSeen *atomic.Int32) func() {
	n := inFlight.Add(1)
	for {
		max := maxSeen.Load()
		if n <= max || maxSeen.CompareAndSwap(max, n) {
			break
		}
	}
	return func() { inFlight.Add(-1) }
}

func (m *blockingMailer) Ready() bool { return true }
func (m *blockingMailer) SendVerificationEmail(_ context.Context, e VerificationEmail) error {
	defer trackMax(&m.inFlight, &m.maxSeen)()
	m.entered <- struct{}{}
	<-m.release
	if m.panicky {
		panic("smtp exploded for " + e.Recipient + " token " + e.Token)
	}
	m.sent.Add(1)
	return m.err
}

// countingStore cuenta las emisiones: un despacho descartado no debe llegar
// a generar ni guardar token.
type countingStore struct {
	*verificationStoreFake
	issueCalls atomic.Int32
}

func (c *countingStore) Issue(ctx context.Context, userID string, hash []byte, ttl time.Duration) (repository.IssueOutcome, error) {
	c.issueCalls.Add(1)
	return c.verificationStoreFake.Issue(ctx, userID, hash, ttl)
}

func (c *countingStore) hasToken(userID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.issued[userID] != nil
}

func (c *countingStore) releasedCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.released)
}

// syncLog permite leer el log mientras escriben las goroutines de envío.
type syncLog struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (l *syncLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *syncLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

func captureSyncLogs(t *testing.T) *syncLog {
	t.Helper()
	l := &syncLog{}
	prev := log.Writer()
	log.SetOutput(l)
	t.Cleanup(func() { log.SetOutput(prev) })
	return l
}

func dispatchUser(i int) *model.User {
	return &model.User{ID: fmt.Sprintf("user-%d", i), Email: fmt.Sprintf("user%d@example.com", i)}
}

// newDispatchEnv usa el despacho de producción (goroutines reales).
func newDispatchEnv(t *testing.T, users int) (*EmailVerificationService, *blockingMailer, *countingStore, *syncLog) {
	t.Helper()
	logs := captureSyncLogs(t)
	repo := newMockUserRepo()
	for i := 0; i < users; i++ {
		u := dispatchUser(i)
		repo.usersByEmail[u.Email] = u
	}
	mailer := newBlockingMailer()
	store := &countingStore{verificationStoreFake: newVerificationStoreFake()}
	svc, err := NewEmailVerificationService(repo, store, mailer, EmailVerificationConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mailer.releaseAll)
	return svc, mailer, store, logs
}

func waitEntered(t *testing.T, m *blockingMailer, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-m.entered:
		case <-time.After(3 * time.Second):
			t.Fatalf("solo %d de %d envíos empezaron", i, n)
		}
	}
}

func waitSlotsFree(t *testing.T, svc *EmailVerificationService) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for len(svc.slots) != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("cupos sin liberar: %d", len(svc.slots))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestEmailVerificationDispatch_RegisterReturnsWithoutWaitingSMTP(t *testing.T) {
	svc, mailer, store, logs := newDispatchEnv(t, 1)
	mailer.err = &SMTPError{Stage: "data", Code: 451}
	done := make(chan struct{})
	go func() {
		svc.SendForNewUser(context.Background(), dispatchUser(0))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SendForNewUser no debe esperar al SMTP")
	}
	waitEntered(t, mailer, 1) // el envío sigue en curso, retenido
	if len(svc.slots) != 1 {
		t.Fatal("el envío del registro ocupa un cupo del mismo semáforo")
	}
	mailer.releaseAll()
	waitSlotsFree(t, svc)
	if !strings.Contains(logs.String(), "op=Register user_id=user-0 step=send status=error smtp_stage=data smtp_code=451") {
		t.Fatalf("el fallo posterior queda en el log:\n%s", logs)
	}
	if store.releasedCount() != 1 {
		t.Fatal("tras el fallo SMTP se libera el enfriamiento para reenviar")
	}
}

func TestEmailVerificationDispatch_FifthIsSkippedImmediatelyWithoutToken(t *testing.T) {
	svc, mailer, store, logs := newDispatchEnv(t, 5)
	for i := 0; i < 4; i++ {
		if err := svc.Request(context.Background(), dispatchUser(i).Email); err != nil {
			t.Fatal(err)
		}
	}
	waitEntered(t, mailer, 4)
	if got := len(svc.slots); got != EmailVerificationMaxConcurrentSends {
		t.Fatalf("cupos ocupados=%d", got)
	}
	start := time.Now()
	if err := svc.Request(context.Background(), dispatchUser(4).Email); err != nil {
		t.Fatalf("el descarte por ocupación mantiene la respuesta genérica: %v", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("la quinta solicitud debe descartarse de inmediato, sin esperar cupo")
	}
	if store.issueCalls.Load() != 4 || store.hasToken("user-4") {
		t.Fatal("un despacho descartado no genera ni guarda token (no consume enfriamiento ni tope)")
	}
	if !strings.Contains(logs.String(), "[auth] email-verification op=SendVerificationEmail user_id=user-4 step=dispatch status=skipped reason=busy") {
		t.Fatalf("falta el log de descarte:\n%s", logs)
	}
	mailer.releaseAll()
	waitSlotsFree(t, svc)
	if mailer.sent.Load() != 4 {
		t.Fatalf("enviados=%d", mailer.sent.Load())
	}
	// Con cupo libre, el reenvío del descartado funciona de inmediato.
	if err := svc.Request(context.Background(), dispatchUser(4).Email); err != nil {
		t.Fatal(err)
	}
	waitEntered(t, mailer, 1)
	waitSlotsFree(t, svc)
	if mailer.sent.Load() != 5 || !store.hasToken("user-4") {
		t.Fatal("el reenvío posterior al descarte debe emitir y enviar")
	}
	if strings.Contains(logs.String(), "@example.com") {
		t.Fatalf("los logs no deben contener correos:\n%s", logs)
	}
}

func TestEmailVerificationDispatch_RegisterAndResendShareSlots(t *testing.T) {
	svc, mailer, store, logs := newDispatchEnv(t, 4)
	for i := 0; i < 4; i++ {
		svc.SendForNewUser(context.Background(), dispatchUser(i))
	}
	waitEntered(t, mailer, 4)
	// Los cupos los ocupó el registro: un reenvío también se descarta, y viceversa.
	_ = svc.Request(context.Background(), dispatchUser(0).Email)
	svc.SendForNewUser(context.Background(), &model.User{ID: "new-user", Email: "new@example.com"})
	out := logs.String()
	for _, want := range []string{
		"op=SendVerificationEmail user_id=user-0 step=dispatch status=skipped reason=busy",
		"op=Register user_id=new-user step=dispatch status=skipped reason=busy",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("falta %q en:\n%s", want, out)
		}
	}
	if store.issueCalls.Load() != 4 {
		t.Fatalf("issue=%d", store.issueCalls.Load())
	}
	mailer.releaseAll()
	waitSlotsFree(t, svc)
}

func TestEmailVerificationDispatch_NeverMoreThanFourUnderConcurrency(t *testing.T) {
	const requests = 60
	svc, mailer, store, logs := newDispatchEnv(t, requests)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			if i%2 == 0 {
				_ = svc.Request(context.Background(), dispatchUser(i).Email)
			} else {
				svc.SendForNewUser(context.Background(), dispatchUser(i))
			}
		}(i)
	}
	close(start)
	wg.Wait() // todas las llamadas volvieron sin esperar cupo
	waitEntered(t, mailer, 4)
	skipped := strings.Count(logs.String(), "status=skipped reason=busy")
	if int(store.issueCalls.Load()) != 4 || skipped != requests-4 {
		t.Fatalf("con los 4 cupos retenidos: issue=%d descartados=%d", store.issueCalls.Load(), skipped)
	}
	mailer.releaseAll()
	waitSlotsFree(t, svc)
	if mailer.maxSeen.Load() != EmailVerificationMaxConcurrentSends {
		t.Fatalf("concurrencia máxima observada %d", mailer.maxSeen.Load())
	}

	// Sin retener envíos: ráfagas sucesivas nunca superan 4 a la vez.
	quick := &quickMailer{}
	svc.mailer = quick
	for round := 0; round < 20; round++ {
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				svc.SendForNewUser(context.Background(), &model.User{ID: fmt.Sprintf("r%d-%d", round, i), Email: "x@example.com"})
			}(i)
		}
		wg.Wait()
	}
	waitSlotsFree(t, svc)
	if m := quick.maxSeen.Load(); m == 0 || m > EmailVerificationMaxConcurrentSends {
		t.Fatalf("concurrencia máxima observada %d", m)
	}
}

type quickMailer struct{ inFlight, maxSeen atomic.Int32 }

func (m *quickMailer) Ready() bool { return true }
func (m *quickMailer) SendVerificationEmail(context.Context, VerificationEmail) error {
	defer trackMax(&m.inFlight, &m.maxSeen)()
	time.Sleep(2 * time.Millisecond)
	return nil
}

func TestEmailVerificationDispatch_SlotsReleasedAfterSuccessFailureAndPanic(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		panicky bool
		logPart string
	}{
		{"success", nil, false, "step=send status=ok outcome=sent"},
		{"smtp failure", &SMTPError{Stage: "dial"}, false, "step=send status=error smtp_stage=dial"},
		{"panic", nil, true, "step=dispatch status=error cause=panic"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, mailer, store, logs := newDispatchEnv(t, 4)
			mailer.err, mailer.panicky = tc.err, tc.panicky
			mailer.releaseAll() // no retener
			for round := 0; round < 2; round++ {
				for i := 0; i < 4; i++ {
					if round == 1 {
						store.mu.Lock()
						delete(store.issued, dispatchUser(i).ID)
						store.mu.Unlock()
					}
					_ = svc.Request(context.Background(), dispatchUser(i).Email)
				}
				waitEntered(t, mailer, 4)
				waitSlotsFree(t, svc)
			}
			out := logs.String()
			if strings.Count(out, tc.logPart) != 8 {
				t.Fatalf("esperaba 8 × %q (los cupos se reutilizan):\n%s", tc.logPart, out)
			}
			if strings.Contains(out, "reason=busy") {
				t.Fatal("tras liberar los cupos no debe haber descartes")
			}
			if tc.err != nil && store.releasedCount() != 8 {
				t.Fatal("tras un fallo SMTP se libera el enfriamiento para reenviar")
			}
			if strings.Contains(out, "smtp exploded") || strings.Contains(out, "@example.com") {
				t.Fatalf("el panic no debe volcar su contenido al log:\n%s", out)
			}
		})
	}
}
