package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// createPublicNoteForRateLimit crea una nota pública vía handler y devuelve su id.
func createPublicNoteForRateLimit(t *testing.T, r *gin.Engine, token, title string) string {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notes", strings.NewReader(`{"title":"`+title+`","visibility":"public","content":"data"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create nota pública esperaba 201 got %d body %s", w.Code, w.Body.String())
	}
	var cre map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &cre); err != nil {
		t.Fatalf("respuesta create inválida: %v", err)
	}
	if cre["note_id"] == "" {
		t.Fatal("note_id vacío")
	}
	return cre["note_id"]
}

// copyOnce ejecuta POST /notes/{id}/copy con el token dado.
func copyOnce(r *gin.Engine, noteID, token string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notes/"+noteID+"/copy", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	return w
}

// Rate limit del endpoint de copia: las 10 primeras clonaciones por usuario
// dentro de un minuto responden 201 y la petición 11 responde 429 con mensaje
// limpio; otro usuario no queda afectado.
func TestHandlerCopyRateLimitReturns429(t *testing.T) {
	r, _, _, _ := setupRouter()
	author := uuid.NewString()
	copier := uuid.NewString()
	at := genToken(author, "student")
	ct := genToken(copier, "student")
	noteID := createPublicNoteForRateLimit(t, r, at, "RateLimit")

	for i := 0; i < copyRateLimitMax; i++ {
		w := copyOnce(r, noteID, ct)
		if w.Code != http.StatusCreated {
			t.Fatalf("copy %d/%d esperaba 201 got %d body %s", i+1, copyRateLimitMax, w.Code, w.Body.String())
		}
	}

	w := copyOnce(r, noteID, ct)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("petición %d esperaba 429 got %d body %s", copyRateLimitMax+1, w.Code, w.Body.String())
	}
	var errResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("429 debe ser JSON válido: %v body=%s", err, w.Body.String())
	}
	if errResp.Error.Code != "rate_limited" {
		t.Fatalf("code esperado rate_limited got %q", errResp.Error.Code)
	}
	if !strings.Contains(errResp.Error.Message, "Límite de clonación excedido, intente más tarde") {
		t.Fatalf("mensaje limpio esperado, got %q", errResp.Error.Message)
	}

	// El límite es por usuario: otro usuario puede clonar sin problema.
	otherTok := genToken(uuid.NewString(), "student")
	w2 := copyOnce(r, noteID, otherTok)
	if w2.Code != http.StatusCreated {
		t.Fatalf("otro usuario no debe estar limitado, got %d body %s", w2.Code, w2.Body.String())
	}
}

// El limitador es seguro para concurrencia y su ventana se restablece al cabo
// de un minuto: exactamente copyRateLimitMax intentos simultáneos pasan y los
// timestamps vencidos liberan cupo.
func TestCopyRateLimiterConcurrencyAndWindowReset(t *testing.T) {
	l := newCopyRateLimiter()
	user := uuid.NewString()
	for i := 0; i < copyRateLimitMax; i++ {
		if ok, _ := l.allow(user); !ok {
			t.Fatalf("intento %d debe permitirse", i+1)
		}
	}
	if ok, _ := l.allow(user); ok {
		t.Fatal("el intento que supera el máximo dentro de la ventana debe rechazarse")
	}

	// Envejecer los timestamps del usuario: al vencer la ventana se recupera el cupo.
	l.mu.Lock()
	for i := range l.buckets[user] {
		l.buckets[user][i] = time.Now().Add(-2 * copyRateLimitWindow)
	}
	l.mu.Unlock()
	if ok, _ := l.allow(user); !ok {
		t.Fatal("tras vencer la ventana el usuario debe poder clonar de nuevo")
	}

	// Concurrencia: exactamente copyRateLimitMax de N intentos simultáneos pasan.
	concurrent := uuid.NewString()
	const attempts = 50
	allowed := make([]bool, attempts)
	var wg sync.WaitGroup
	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func(idx int) {
			defer wg.Done()
			ok, _ := l.allow(concurrent)
			allowed[idx] = ok
		}(i)
	}
	wg.Wait()
	pass := 0
	for _, ok := range allowed {
		if ok {
			pass++
		}
	}
	if pass != copyRateLimitMax {
		t.Fatalf("esperaba exactamente %d permitidos concurrentes, got %d", copyRateLimitMax, pass)
	}
}

// El límite global corta a los 100 clonaciones/minuto entre todos los
// usuarios: usuarios individuales con cupo libre reciben 429 cuando el total
// agregado alcanza el tope y el handler lo reporta con mensaje claro.
func TestCopyRateLimiterGlobalCap(t *testing.T) {
	l := newCopyRateLimiter()
	// 100 usuarios distintos, una clonación cada uno (tope por usuario no aplica).
	for i := 0; i < copyRateLimitGlobalMax; i++ {
		if ok, hitGlobal := l.allow("user-" + uuid.NewString()); !ok {
			t.Fatalf("clonación global %d debe permitirse (ok=%v hitGlobal=%v)", i+1, ok, hitGlobal)
		}
	}
	// Cualquier clonación adicional, de cualquier usuario, se rechaza por tope global.
	ok, hitGlobal := l.allow(uuid.NewString())
	if ok {
		t.Fatal("la clonación que supera el tope global debe rechazarse")
	}
	if !hitGlobal {
		t.Fatal("la denegación debe reportarse como límite global")
	}
	// Al envejecer toda la ventana, el cupo global se recupera.
	l.mu.Lock()
	for i := range l.globalTimes {
		l.globalTimes[i] = time.Now().Add(-2 * copyRateLimitWindow)
	}
	l.mu.Unlock()
	if ok, _ := l.allow(uuid.NewString()); !ok {
		t.Fatal("tras vencer la ventana el cupo global debe recuperarse")
	}
}

// A nivel HTTP: 100 clonaciones desde 100 usuarios distintos responden 201 y la
// clonación 101 (tope global) responde 429 con el mensaje de límite global; sin
// alcanzar jamás el tope por usuario.
func TestHandlerCopyGlobalRateLimitReturns429(t *testing.T) {
	r, _, _, _ := setupRouter()
	author := uuid.NewString()
	at := genToken(author, "student")
	noteID := createPublicNoteForRateLimit(t, r, at, "GlobalRate")
	for i := 0; i < copyRateLimitGlobalMax; i++ {
		tok := genToken(uuid.NewString(), "student")
		w := copyOnce(r, noteID, tok)
		if w.Code != http.StatusCreated {
			t.Fatalf("copy global %d/%d esperaba 201 got %d body %s", i+1, copyRateLimitGlobalMax, w.Code, w.Body.String())
		}
	}
	w := copyOnce(r, noteID, genToken(uuid.NewString(), "student"))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("clonación %d esperaba 429 global got %d body %s", copyRateLimitGlobalMax+1, w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Límite global de clonación excedido, intente más tarde") {
		t.Fatalf("mensaje de límite global claro esperado, got %q", w.Body.String())
	}
}
