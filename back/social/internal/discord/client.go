package discord

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

// WebhookError representa un error del webhook de Discord con código HTTP semántico.
type WebhookError struct {
	Code    int // 400, 401, 404, 429, 500...
	Message string
}

func (e *WebhookError) Error() string { return fmt.Sprintf("discord %d: %s", e.Code, e.Message) }

// Message es el aviso de reunión a publicar en el canal.
type Message struct {
	Title       string
	Description string
	ScheduledAt time.Time
	HasSchedule bool
}

// Client abstrae el envío de avisos al webhook de Discord del grupo.
// En tests se usa MockClient.
type Client interface {
	// SendMeetingCreated publica el aviso de una reunión recién agendada.
	SendMeetingCreated(ctx context.Context, webhookURL string, msg Message) error
}

// MockClient implementación en memoria para tests.
type MockClient struct {
	mu sync.Mutex
	// SendErr, si no es nil, lo retorna SendMeetingCreated (para simular fallos).
	SendErr error
	// LastURL y LastMessage guardan el último envío (para aserciones).
	LastURL     string
	LastMessage *Message
	Count       int
}

func NewMockClient() *MockClient { return &MockClient{} }

func (m *MockClient) SendMeetingCreated(ctx context.Context, webhookURL string, msg Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.SendErr != nil {
		return m.SendErr
	}
	m.Count++
	m.LastURL = webhookURL
	cp := msg
	m.LastMessage = &cp
	return nil
}

var _ Client = (*MockClient)(nil)

// WebhookClient es la implementación real: POST JSON al webhook_url (stdlib).
type WebhookClient struct {
	httpClient *http.Client
}

func NewWebhookClient() *WebhookClient {
	return &WebhookClient{httpClient: &http.Client{Timeout: 8 * time.Second}}
}

type webhookPayload struct {
	Content string `json:"content"`
}

// FormatMeetingMessage arma el texto del aviso. Exportada para tests.
func FormatMeetingMessage(msg Message) string {
	var b strings.Builder
	b.WriteString("📅 **Nueva reunión agendada**\n")
	b.WriteString("**" + strings.TrimSpace(msg.Title) + "**")
	if msg.HasSchedule {
		b.WriteString("\n🕒 " + msg.ScheduledAt.Format("2006-01-02 15:04 MST"))
	}
	if strings.TrimSpace(msg.Description) != "" {
		b.WriteString("\n" + strings.TrimSpace(msg.Description))
	}
	return b.String()
}

func (c *WebhookClient) SendMeetingCreated(ctx context.Context, webhookURL string, msg Message) error {
	webhookURL = strings.TrimSpace(webhookURL)
	if webhookURL == "" {
		return errors.New("webhook_url vacía")
	}
	if strings.TrimSpace(msg.Title) == "" {
		return errors.New("título vacío")
	}
	body, err := json.Marshal(webhookPayload{Content: FormatMeetingMessage(msg)})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &WebhookError{Code: 500, Message: "discord no disponible: " + err.Error()}
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &WebhookError{Code: resp.StatusCode, Message: strings.TrimSpace(string(respBody))}
	}
	return nil
}

var _ Client = (*WebhookClient)(nil)
