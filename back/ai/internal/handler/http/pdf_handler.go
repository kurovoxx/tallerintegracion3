package httpHandler

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/ai/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/ai/internal/service"
)

// PDFToMarkdownResponse es la respuesta del endpoint de conversión.
type PDFToMarkdownResponse struct {
	Markdown string `json:"markdown"`
	Filename string `json:"filename"`
	Size     int    `json:"size_bytes"`
	Provider string `json:"provider"`
}

// PDFHandler expone el endpoint de conversión PDF→Markdown.
type PDFHandler struct {
	svc         service.AIService
	authMw      *middleware.AuthMiddleware
	rateLimiter *service.RateLimiter
}

func NewPDFHandler(svc service.AIService, authMw *middleware.AuthMiddleware, rateLimiter *service.RateLimiter) *PDFHandler {
	return &PDFHandler{svc: svc, authMw: authMw, rateLimiter: rateLimiter}
}

// requireAuthAndRateLimit valida el JWT y el rate limit por usuario.
// Devuelve el user_id y true si pasó, o false si ya respondió con error.
func (h *PDFHandler) requireAuthAndRateLimit(c *gin.Context) (string, bool) {
	// 1. Validar JWT
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized", "message": "user_id no encontrado en token"}})
		c.Abort()
		return "", false
	}

	// 2. Rate limit por usuario (5 conversiones por hora)
	if !h.rateLimiter.Allow(userID) {
		remaining := h.rateLimiter.Remaining(userID)
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": gin.H{
				"code":    "rate_limit_exceeded",
				"message": fmt.Sprintf("límite de %d conversiones por hora excedido. Intentá de vuelta más tarde.", h.rateLimiter.Limit()),
				"remaining": remaining,
			},
		})
		c.Abort()
		return "", false
	}

	return userID, true
}

// ConvertPDFToMarkdown recibe un archivo PDF (multipart/form-data, campo "file")
// y devuelve el Markdown extraído por Gemini.
func (h *PDFHandler) ConvertPDFToMarkdown(c *gin.Context) {
	// Validar auth y rate limit antes de procesar el archivo.
	if _, ok := h.requireAuthAndRateLimit(c); !ok {
		return
	}

	// Recibir el archivo.
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "invalid_body",
				"message": "se requiere el campo 'file' con el PDF",
			},
		})
		return
	}

	// Validar extensión.
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".pdf" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "invalid_file",
				"message": "solo se permiten archivos .pdf",
			},
		})
		return
	}

	// Validar MIME type.
	opened, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "file_read_error",
				"message": err.Error(),
			},
		})
		return
	}
	defer opened.Close()

	buffer := make([]byte, 512)
	if _, err := opened.Read(buffer); err != nil && err.Error() != "EOF" {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "file_read_error",
				"message": err.Error(),
			},
		})
		return
	}
	detected := http.DetectContentType(buffer)
	if detected != "application/pdf" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "invalid_file",
				"message": fmt.Sprintf("el archivo no es un PDF válido (detectado: %s)", detected),
			},
		})
		return
	}

	// Leer todo el contenido del archivo.
	if _, err := opened.Seek(0, 0); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "file_read_error",
				"message": err.Error(),
			},
		})
		return
	}

	pdfData, err := io.ReadAll(opened)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "file_read_error",
				"message": err.Error(),
			},
		})
		return
	}

	// Llamar al servicio de IA.
	markdown, err := h.svc.ConvertPDFToMarkdown(pdfData, file.Filename)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "conversion_failed",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, PDFToMarkdownResponse{
		Markdown: markdown,
		Filename: file.Filename,
		Size:     len(pdfData),
		Provider: "gemini",
	})
}
