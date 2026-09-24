package stream

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// StreamError representa un error de la API de Stream con código HTTP semántico.
type StreamError struct {
	Code    int
	Message string
}

func (e *StreamError) Error() string { return fmt.Sprintf("stream %d: %s", e.Code, e.Message) }

// Client abstrae la creación de canales de chat en Stream (getstream.io).
// En tests/dev se usa MockClient.
type Client interface {
	// CreateChannel crea un canal del tipo dado (ej. "messaging") con el id y nombre indicados.
	// Idempotente a nivel de llamada: si el canal ya existe en Stream, no es error.
	CreateChannel(ctx context.Context, channelType, channelID, name string) error
}

// MockClient implementación en memoria para tests y STREAM_MODE=mock.
type MockClient struct {
	mu sync.Mutex
	// CreateErr, si no es nil, lo retorna CreateChannel (para simular fallos).
	CreateErr error
	// Calls registra los canales creados (para aserciones).
	Calls []CreateCall
}

type CreateCall struct {
	ChannelType string
	ChannelID   string
	Name        string
}

func NewMockClient() *MockClient { return &MockClient{} }

func (m *MockClient) CreateChannel(ctx context.Context, channelType, channelID, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.CreateErr != nil {
		return m.CreateErr
	}
	m.Calls = append(m.Calls, CreateCall{ChannelType: channelType, ChannelID: channelID, Name: name})
	return nil
}

var _ Client = (*MockClient)(nil)

// RESTClient es la implementación real contra la API de Stream Chat.
// Solo usa stdlib: firma el server token (JWT HS256 con el API secret) a mano.
type RESTClient struct {
	apiKey     string
	apiSecret  string
	httpClient *http.Client
	// baseURL inyectable en tests (default: https://chat.stream-io-api.com).
	baseURL string
}

func NewRESTClient(apiKey, apiSecret string) *RESTClient {
	return &RESTClient{
		apiKey:     apiKey,
		apiSecret:  apiSecret,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    "https://chat.stream-io-api.com",
	}
}

// serverToken firma un JWT HS256 mínimo para auth server-side de Stream.
func serverToken(apiSecret string) (string, error) {
	if strings.TrimSpace(apiSecret) == "" {
		return "", errors.New("api secret vacío")
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"server":true}`))
	mac := hmac.New(sha256.New, []byte(apiSecret))
	mac.Write([]byte(header + "." + payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return header + "." + payload + "." + sig, nil
}

type createChannelRequest struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

func (c *RESTClient) CreateChannel(ctx context.Context, channelType, channelID, name string) error {
	channelType = strings.TrimSpace(channelType)
	channelID = strings.TrimSpace(channelID)
	if channelType == "" || channelID == "" {
		return errors.New("channel type/id vacíos")
	}
	if strings.TrimSpace(c.apiKey) == "" {
		return &StreamError{Code: 401, Message: "api key vacía"}
	}
	token, err := serverToken(c.apiSecret)
	if err != nil {
		return &StreamError{Code: 401, Message: err.Error()}
	}
	body, err := json.Marshal(createChannelRequest{ID: channelID, Type: channelType, Name: name})
	if err != nil {
		return err
	}
	u := strings.TrimSuffix(c.baseURL, "/") + "/channels?api_key=" + url.QueryEscape(c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", token)
	req.Header.Set("Stream-Auth-Type", "jwt")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &StreamError{Code: 500, Message: "stream no disponible: " + err.Error()}
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &StreamError{Code: resp.StatusCode, Message: strings.TrimSpace(string(respBody))}
	}
	return nil
}

var _ Client = (*RESTClient)(nil)
