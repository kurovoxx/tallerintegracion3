package service

import (
	"fmt"
	"strings"

	"github.com/kurovoxx/tallerintegracion3/back/ai/internal/model"
)

// AIService define la interfaz de generación de contenido.
// La implementación real (Gemini) se conecta en la tarea 4_5_6.
type AIService interface {
	Generate(req model.GenerateRequest) (model.GenerateResponse, error)
	// ConvertPDFToMarkdown recibe un PDF en bytes y devuelve el Markdown extraído.
	ConvertPDFToMarkdown(pdfData []byte, filename string) (string, error)
}

// MockAIService es una implementación placeholder para el scaffold.
// Devuelve una respuesta simulada sin llamar a ningún proveedor externo.
type MockAIService struct{}

func NewMockAIService() *MockAIService {
	return &MockAIService{}
}

func (s *MockAIService) Generate(req model.GenerateRequest) (model.GenerateResponse, error) {
	if strings.TrimSpace(req.Input) == "" {
		return model.GenerateResponse{}, fmt.Errorf("input vacío")
	}
	return model.GenerateResponse{
		ID:        "mock-001",
		Output:    "Respuesta simulada del microservicio IA (modo mock). Operación: " + req.Operation,
		Provider:  "mock",
	}, nil
}

func (s *MockAIService) ConvertPDFToMarkdown(pdfData []byte, filename string) (string, error) {
	if len(pdfData) == 0 {
		return "", fmt.Errorf("pdf vacío")
	}
	return fmt.Sprintf("# %s\n\n*(Conversión simulada - modo mock. El contenido real se obtiene con Gemini en la tarea 4_5-6)*\n\nEste es un placeholder del Markdown extraído de `%s` (%d bytes).", filename, filename, len(pdfData)), nil
}
