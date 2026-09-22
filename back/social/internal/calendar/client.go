package calendar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// CalendarError representa un error de la API de Calendar con código HTTP semántico.
type CalendarError struct {
	Code    int // 401, 403, 404, 429, 500...
	Message string
}

func (e *CalendarError) Error() string { return fmt.Sprintf("calendar %d: %s", e.Code, e.Message) }

// IsUnauthorized indica token inválido/revocado: el llamador debe reportarlo a Auth.
func IsUnauthorized(err error) bool {
	var ce *CalendarError
	if errors.As(err, &ce) {
		return ce.Code == 401 || ce.Code == 403
	}
	return false
}

// Event es el evento mínimo a crear en Google Calendar.
type Event struct {
	Summary     string
	Description string
	Start       time.Time
	End         time.Time
}

// Client abstrae Google Calendar del creador de la reunión.
// En producción usa la API REST v3 con el access token de identity.oauth_connections.
// En tests/dev se usa MockClient.
type Client interface {
	// CreateEvent crea el evento en el calendario primario y retorna el event ID de Google.
	CreateEvent(ctx context.Context, accessToken string, event Event) (string, error)
}

// MockClient implementación en memoria para tests y CALENDAR_MODE=mock.
type MockClient struct {
	mu sync.Mutex
	// CreateErr, si no es nil, lo retorna CreateEvent (para simular fallos).
	CreateErr error
	// LastEvent guarda el último evento recibido (para aserciones en tests).
	LastEvent *Event
	// LastToken guarda el último access token recibido.
	LastToken string
	Count     int
}

func NewMockClient() *MockClient { return &MockClient{} }

// LastEventID retorna el último event ID generado (para aserciones en tests).
func (m *MockClient) LastEventID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Count == 0 {
		return ""
	}
	return fmt.Sprintf("mock_evt_%d", m.Count)
}

func (m *MockClient) CreateEvent(ctx context.Context, accessToken string, event Event) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.CreateErr != nil {
		return "", m.CreateErr
	}
	m.Count++
	ev := event
	m.LastEvent = &ev
	m.LastToken = accessToken
	return fmt.Sprintf("mock_evt_%d", m.Count), nil
}

var _ Client = (*MockClient)(nil)

// RESTClient es la implementación real contra la API REST v3 de Google Calendar.
// Solo usa stdlib (net/http) para no agregar dependencias al módulo.
type RESTClient struct {
	httpClient *http.Client
	// baseURL inyectable en tests (default: https://www.googleapis.com).
	baseURL string
}

func NewRESTClient() *RESTClient {
	return &RESTClient{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    "https://www.googleapis.com",
	}
}

type calendarDateTime struct {
	DateTime string `json:"dateTime"`
}

type createEventRequest struct {
	Summary     string           `json:"summary"`
	Description string           `json:"description,omitempty"`
	Start       calendarDateTime `json:"start"`
	End         calendarDateTime `json:"end"`
}

func (c *RESTClient) CreateEvent(ctx context.Context, accessToken string, event Event) (string, error) {
	if strings.TrimSpace(accessToken) == "" {
		return "", &CalendarError{Code: 401, Message: "access token vacío"}
	}
	if strings.TrimSpace(event.Summary) == "" {
		return "", errors.New("summary vacío")
	}
	end := event.End
	if end.IsZero() || !end.After(event.Start) {
		end = event.Start.Add(1 * time.Hour) // default: 1h
	}
	body, err := json.Marshal(createEventRequest{
		Summary:     event.Summary,
		Description: event.Description,
		Start:       calendarDateTime{DateTime: event.Start.Format(time.RFC3339)},
		End:         calendarDateTime{DateTime: end.Format(time.RFC3339)},
	})
	if err != nil {
		return "", err
	}
	url := strings.TrimSuffix(c.baseURL, "/") + "/calendar/v3/calendars/primary/events"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", &CalendarError{Code: 500, Message: "google no disponible: " + err.Error()}
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return "", &CalendarError{Code: resp.StatusCode, Message: "permiso denegado por Google (revocado o insuficiente)"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &CalendarError{Code: resp.StatusCode, Message: strings.TrimSpace(string(respBody))}
	}
	var data struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return "", fmt.Errorf("respuesta calendar inválida: %w", err)
	}
	if strings.TrimSpace(data.ID) == "" {
		return "", errors.New("Google no devolvió event id")
	}
	return data.ID, nil
}

var _ Client = (*RESTClient)(nil)
