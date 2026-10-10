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

// Client abstrae el chat en Stream (getstream.io): canales y mensajes.
// En tests/dev se usa MockClient.
type Client interface {
	// CreateChannel garantiza que exista el canal (get-or-create server-side).
	// Idempotente: si el canal ya existe en Stream, lo retorna sin modificar
	// mensajes ni miembros. Nunca elimina ni recrea nada.
	CreateChannel(ctx context.Context, channelType, channelID, name, createdByID string) error
	// EnsureMember agrega al usuario como miembro del canal (server-side).
	// Idempotente: si ya es miembro, no es error. Solo llamarla después de
	// verificar en nuestra BD que el usuario pertenece al grupo Sigma.
	EnsureMember(ctx context.Context, channelType, channelID, userID string) error
	// SendMessage publica un mensaje de texto en el canal como el usuario indicado.
	// En modo server-side el sender no necesita existir previamente en Stream.
	SendMessage(ctx context.Context, channelType, channelID, senderID, text string) error
}

// MockClient implementación en memoria para tests y STREAM_MODE=mock.
type MockClient struct {
	mu sync.Mutex
	// CreateErr, si no es nil, lo retorna CreateChannel (para simular fallos).
	CreateErr error
	// EnsureErr, si no es nil, lo retorna EnsureMember (para simular fallos).
	EnsureErr error
	// SendErr, si no es nil, lo retorna SendMessage (para simular fallos).
	SendErr error
	// Calls registra los canales creados (para aserciones).
	Calls []CreateCall
	// Ensured registra las membresías garantizadas (para aserciones).
	Ensured []EnsureCall
	// Messages registra los mensajes enviados (para aserciones).
	Messages []SendCall
}

type CreateCall struct {
	ChannelType string
	ChannelID   string
	Name        string
	CreatedByID string
}

// EnsureCall registra una membresía garantizada en un canal.
type EnsureCall struct {
	ChannelType string
	ChannelID   string
	UserID      string
}

// SendCall registra un mensaje enviado al mock.
type SendCall struct {
	ChannelType string
	ChannelID   string
	SenderID    string
	Text        string
}

func NewMockClient() *MockClient { return &MockClient{} }

func (m *MockClient) CreateChannel(ctx context.Context, channelType, channelID, name, createdByID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.CreateErr != nil {
		return m.CreateErr
	}
	m.Calls = append(m.Calls, CreateCall{ChannelType: channelType, ChannelID: channelID, Name: name, CreatedByID: createdByID})
	return nil
}

func (m *MockClient) EnsureMember(ctx context.Context, channelType, channelID, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.EnsureErr != nil {
		return m.EnsureErr
	}
	m.Ensured = append(m.Ensured, EnsureCall{ChannelType: channelType, ChannelID: channelID, UserID: userID})
	return nil
}

func (m *MockClient) SendMessage(ctx context.Context, channelType, channelID, senderID, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.SendErr != nil {
		return m.SendErr
	}
	m.Messages = append(m.Messages, SendCall{ChannelType: channelType, ChannelID: channelID, SenderID: senderID, Text: text})
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

// signJWT firma un payload JSON como JWT HS256 con el API secret.
func signJWT(apiSecret string, payload []byte) (string, error) {
	if strings.TrimSpace(apiSecret) == "" {
		return "", errors.New("api secret vacío")
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(apiSecret))
	mac.Write([]byte(header + "." + encoded))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return header + "." + encoded + "." + sig, nil
}

// serverToken firma un JWT HS256 mínimo para auth server-side de Stream.
func serverToken(apiSecret string) (string, error) {
	token, err := signJWT(apiSecret, []byte(`{"server":true}`))
	if err != nil {
		return "", err
	}
	return token, nil
}

// SignUserToken firma el token de un usuario final para que el cliente
// (front) se conecte directo a Stream. Payload mínimo: {"user_id"}.
// Exportada para el endpoint GET /groups/:id/stream-token y sus tests.
func SignUserToken(apiSecret, userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", errors.New("user_id vacío")
	}
	payload, err := json.Marshal(map[string]string{"user_id": userID})
	if err != nil {
		return "", err
	}
	return signJWT(apiSecret, payload)
}

// queryChannelRequest es el get-or-create oficial de un canal:
// POST /channels/{type}/{id}/query. OJO: POST /channels a secas es
// QueryChannels (búsqueda), NO crea nada aunque responda 200.
type queryChannelRequest struct {
	State bool                 `json:"state"`
	Data  map[string]string    `json:"data,omitempty"`
}

// authHeaders valida credenciales, firma el server token y arma headers comunes.
func (c *RESTClient) authHeaders() (http.Header, error) {
	if strings.TrimSpace(c.apiKey) == "" {
		return nil, &StreamError{Code: 401, Message: "api key vacía"}
	}
	token, err := serverToken(c.apiSecret)
	if err != nil {
		return nil, &StreamError{Code: 401, Message: err.Error()}
	}
	h := http.Header{}
	h.Set("Authorization", token)
	h.Set("Stream-Auth-Type", "jwt")
	h.Set("Content-Type", "application/json")
	return h, nil
}

func (c *RESTClient) postJSON(ctx context.Context, path string, query string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	headers, err := c.authHeaders()
	if err != nil {
		return err
	}
	u := strings.TrimSuffix(c.baseURL, "/") + path + "?api_key=" + url.QueryEscape(c.apiKey) + query
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header = headers
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

func (c *RESTClient) CreateChannel(ctx context.Context, channelType, channelID, name, createdByID string) error {
	channelType = strings.TrimSpace(channelType)
	channelID = strings.TrimSpace(channelID)
	if channelType == "" || channelID == "" {
		return errors.New("channel type/id vacíos")
	}
	var data map[string]string
	if strings.TrimSpace(name) != "" || strings.TrimSpace(createdByID) != "" {
		data = map[string]string{}
		if strings.TrimSpace(name) != "" {
			data["name"] = strings.TrimSpace(name)
		}
		if strings.TrimSpace(createdByID) != "" {
			data["created_by_id"] = strings.TrimSpace(createdByID)
		}
	}
	path := "/channels/" + url.PathEscape(channelType) + "/" + url.PathEscape(channelID) + "/query"
	return c.postJSON(ctx, path, "", queryChannelRequest{State: true, Data: data})
}

type updateChannelRequest struct {
	AddMembers []string `json:"add_members"`
}

// EnsureMember agrega al usuario como miembro del canal vía update server-side.
// POST /channels/{type}/{id} {"add_members":[...]} es idempotente en Stream:
// si el usuario ya es miembro, la llamada tiene éxito sin duplicar.
func (c *RESTClient) EnsureMember(ctx context.Context, channelType, channelID, userID string) error {
	channelType = strings.TrimSpace(channelType)
	channelID = strings.TrimSpace(channelID)
	userID = strings.TrimSpace(userID)
	if channelType == "" || channelID == "" {
		return errors.New("channel type/id vacíos")
	}
	if userID == "" {
		return errors.New("user vacío")
	}
	path := "/channels/" + url.PathEscape(channelType) + "/" + url.PathEscape(channelID)
	return c.postJSON(ctx, path, "", updateChannelRequest{AddMembers: []string{userID}})
}

type sendMessageRequest struct {
	Text   string `json:"text"`
	UserID string `json:"user_id"`
}

// SendMessage publica un mensaje en el canal. Best-effort: el llamador loguea el error.
func (c *RESTClient) SendMessage(ctx context.Context, channelType, channelID, senderID, text string) error {
	channelType = strings.TrimSpace(channelType)
	channelID = strings.TrimSpace(channelID)
	senderID = strings.TrimSpace(senderID)
	if channelType == "" || channelID == "" {
		return errors.New("channel type/id vacíos")
	}
	if senderID == "" {
		return errors.New("sender vacío")
	}
	if strings.TrimSpace(text) == "" {
		return errors.New("texto vacío")
	}
	path := "/channels/" + url.PathEscape(channelType) + "/" + url.PathEscape(channelID) + "/message"
	return c.postJSON(ctx, path, "", sendMessageRequest{Text: text, UserID: senderID})
}

var _ Client = (*RESTClient)(nil)
