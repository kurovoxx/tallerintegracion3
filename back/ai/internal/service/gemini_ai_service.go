package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/kurovoxx/tallerintegracion3/back/ai/internal/config"
	"github.com/kurovoxx/tallerintegracion3/back/ai/internal/model"
)

// geminiRequest es el cuerpo de la petición a la API de Gemini.
type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
	GenerationConfig geminiGenerationConfig `json:"generationConfig"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	InlineData *geminiInlineData `json:"inline_data,omitempty"`
	Text     string            `json:"text,omitempty"`
}

type geminiInlineData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
}

type geminiGenerationConfig struct {
	Temperature     float64 `json:"temperature"`
	MaxOutputTokens int     `json:"maxOutputTokens"`
}

// geminiResponse es el cuerpo de la respuesta de Gemini.
type geminiResponse struct {
	Candidates []geminiCandidate `json:"candidates"`
	Error      *geminiError      `json:"error,omitempty"`
}

type geminiCandidate struct {
	Content geminiContent `json:"content"`
}

type geminiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// sanitizeGeminiError extrae solo el mensaje de error de Gemini sin exponer
// el cuerpo crudo (que puede filtrar la API key en URLs como ?key=...).
func sanitizeGeminiError(statusCode int, body []byte) string {
	var geminiResp geminiResponse
	if err := json.Unmarshal(body, &geminiResp); err == nil && geminiResp.Error != nil {
		return fmt.Sprintf("Gemini respondió %d: %s", statusCode, geminiResp.Error.Message)
	}
	return fmt.Sprintf("Gemini respondió %d: error del proveedor", statusCode)
}

// rateTracker almacena los timestamps de las requests por identificador.
type rateTracker struct {
	mu   sync.Mutex
	store map[string][]time.Time
}

func newRateTracker() *rateTracker {
	return &rateTracker{store: make(map[string][]time.Time)}
}

// Allow verifica si el identificador puede hacer una request (límite por hora).
// Devuelve false si se excedió el límite.
func (rt *rateTracker) Allow(key string, limit int) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-1 * time.Hour)

	// Filtrar timestamps dentro de la última hora.
	var valid []time.Time
	for _, t := range rt.store[key] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= limit {
		rt.store[key] = valid
		return false
	}

	valid = append(valid, now)
	rt.store[key] = valid
	return true
}

// GeminiAIService implementa AIService usando la API REST de Gemini.
type GeminiAIService struct {
	cfg        *config.Config
	httpClient *http.Client
	rateLimiter *rateTracker
}

func NewGeminiAIService(cfg *config.Config) *GeminiAIService {
	return &GeminiAIService{
		cfg:         cfg,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		rateLimiter: newRateTracker(),
	}
}

// Generate implementa AIService.Generate enviando texto a Gemini.
func (s *GeminiAIService) Generate(req model.GenerateRequest) (model.GenerateResponse, error) {
	if s.cfg.GeminiAPIKey == "" {
		return model.GenerateResponse{}, fmt.Errorf("GEMINI_API_KEY no configurada")
	}

	identificador := "default"
	if req.Input != "" {
		identificador = req.Input[:min(16, len(req.Input))]
	}
	if !s.rateLimiter.Allow(identificador, s.cfg.GeminiRateLimit) {
		return model.GenerateResponse{}, fmt.Errorf("rate limit excedido: máximo %d conversiones por hora", s.cfg.GeminiRateLimit)
	}

	text := req.Input
	if text == "" {
		return model.GenerateResponse{}, fmt.Errorf("input vacío")
	}

	payload := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: text},
				},
			},
		},
		GenerationConfig: geminiGenerationConfig{
			Temperature:     0.1,
			MaxOutputTokens: 8192,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return model.GenerateResponse{}, fmt.Errorf("error serializando petición: %w", err)
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", s.cfg.GeminiModel, s.cfg.GeminiAPIKey)

	var lastErr error
	for intento := 0; intento <= 2; intento++ { // 1 inicial + 2 reintentos
		if intento > 0 {
			time.Sleep(time.Duration(intento) * 2 * time.Second) // backoff: 2s, 4s
		}

		resp, err := s.httpClient.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			lastErr = fmt.Errorf("error conectando a Gemini: %w", err)
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("error leyendo respuesta de Gemini: %w", err)
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			lastErr = fmt.Errorf("%s", sanitizeGeminiError(resp.StatusCode, respBody))
			continue // reintentar
		}

		if resp.StatusCode != http.StatusOK {
			return model.GenerateResponse{}, fmt.Errorf("%s", sanitizeGeminiError(resp.StatusCode, respBody))
		}

		var geminiResp geminiResponse
		if err := json.Unmarshal(respBody, &geminiResp); err != nil {
			return model.GenerateResponse{}, fmt.Errorf("error parseando respuesta de Gemini: %w", err)
		}

		if geminiResp.Error != nil {
			return model.GenerateResponse{}, fmt.Errorf("Gemini error: %s", geminiResp.Error.Message)
		}

		if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
			return model.GenerateResponse{}, fmt.Errorf("Gemini no devolvió contenido")
		}

		output := geminiResp.Candidates[0].Content.Parts[0].Text
		return model.GenerateResponse{
			ID:       "gemini-" + randomID(),
			Output:   output,
			Provider: "gemini",
		}, nil
	}

	return model.GenerateResponse{}, fmt.Errorf("falló tras 2 reintentos: %w", lastErr)
}

// ConvertPDFToMarkdown implementa AIService.ConvertPDFToMarkdown.
// Envía el PDF a Gemini (gemini-1.5-flash) y devuelve el Markdown extraído.
func (s *GeminiAIService) ConvertPDFToMarkdown(pdfData []byte, filename string) (string, error) {
	if s.cfg.GeminiAPIKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY no configurada")
	}

	// Validar tamaño máximo (10 MB).
	maxBytes := int64(s.cfg.GeminiMaxFileSizeMB) * 1024 * 1024
	if int64(len(pdfData)) > maxBytes {
		return "", fmt.Errorf("archivo demasiado grande: %d MB (máximo %d MB)", len(pdfData)/(1024*1024), s.cfg.GeminiMaxFileSizeMB)
	}

	// Rate limit por nombre de archivo (o se podría usar user_id si se provee).
	key := "pdf:" + filename
	if !s.rateLimiter.Allow(key, s.cfg.GeminiRateLimit) {
		return "", fmt.Errorf("rate limit excedido: máximo %d conversiones de PDF por hora", s.cfg.GeminiRateLimit)
	}

	// Codificar PDF a base64 para enviarlo como inline_data.
	pdfBase64 := base64.StdEncoding.EncodeToString(pdfData)

	payload := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{
						InlineData: &geminiInlineData{
							MimeType: "application/pdf",
							Data:     pdfBase64,
						},
					},
					{
						Text: "Convierte este PDF a Markdown. Conserva la estructura del documento: encabezados, listas, tablas, texto en negrita/cursiva, etc. Devuelve solo el contenido Markdown, sin texto introductorio ni explicaciones adicionales.",
					},
				},
			},
		},
		GenerationConfig: geminiGenerationConfig{
			Temperature:     0.1,
			MaxOutputTokens: 8192,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("error serializando petición: %w", err)
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", s.cfg.GeminiModel, s.cfg.GeminiAPIKey)

	var lastErr error
	for intento := 0; intento <= 2; intento++ { // 1 inicial + 2 reintentos
		if intento > 0 {
			time.Sleep(time.Duration(intento) * 2 * time.Second) // backoff: 2s, 4s
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			cancel()
			lastErr = fmt.Errorf("error creando petición: %w", err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			cancel()
			lastErr = fmt.Errorf("error conectando a Gemini: %w", err)
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancel()
		if err != nil {
			lastErr = fmt.Errorf("error leyendo respuesta de Gemini: %w", err)
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			lastErr = fmt.Errorf("%s", sanitizeGeminiError(resp.StatusCode, respBody))
			continue // reintentar
		}

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("%s", sanitizeGeminiError(resp.StatusCode, respBody))
		}

		var geminiResp geminiResponse
		if err := json.Unmarshal(respBody, &geminiResp); err != nil {
			return "", fmt.Errorf("error parseando respuesta de Gemini: %w", err)
		}

		if geminiResp.Error != nil {
			return "", fmt.Errorf("Gemini error: %s", geminiResp.Error.Message)
		}

		if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
			return "", fmt.Errorf("Gemini no devolvió contenido")
		}

		markdown := geminiResp.Candidates[0].Content.Parts[0].Text
		return markdown, nil
	}

	return "", fmt.Errorf("falló tras 2 reintentos: %w", lastErr)
}

func randomID() string {
	b := make([]byte, 8)
	for i := range b {
		b[i] = 'a' + byte(i%26) // simple identificador determinístico suficiente para mock de ID
	}
	return string(b)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
