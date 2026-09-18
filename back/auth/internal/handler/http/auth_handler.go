package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/utils"
)

// Handler: parsea request, valida forma, llama Service, mapea errores a HTTP.
// Sin lógica de negocio (masterprompt 2).
type AuthHandler struct {
	svc *service.AuthService
}

func NewAuthHandler(svc *service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

type registerRequest struct {
	Email       string  `json:"email" binding:"required"`
	Password    string  `json:"password" binding:"required"`
	Role        string  `json:"role" binding:"required"`
	DisplayName string  `json:"display_name" binding:"required"`
	PhotoURL    *string `json:"photo_url"`
	Phone       *string `json:"phone"`
	Institution *string `json:"institution"`
	Description *string `json:"description"`
	Visibility  *string `json:"visibility"`
}

type registerResponse struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

type loginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type loginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// Login maneja POST /auth/login (sin auth) según agentApiContract.md:54.
// Verifica bcrypt, genera JWT corto + refresh hasheado.
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "Request inválido")
		return
	}
	res, err := h.svc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		if se, ok := err.(*service.ServiceError); ok {
			status := utils.StatusForCode(se.Code)
			utils.RespondError(c, status, se.Code, se.Message)
			return
		}
		utils.RespondError(c, http.StatusInternalServerError, "internal_error", "Error interno")
		return
	}
	c.JSON(http.StatusOK, loginResponse{
		AccessToken:  res.AccessToken,
		RefreshToken: res.RefreshToken,
		ExpiresIn:    res.ExpiresIn,
	})
}

// Register maneja POST /auth/register (sin auth).
// Contrato: 201 {id,email,role,created_at} o error {error:{code,message}}.
func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Distinguir campo faltante para dar código específico si es posible
		// Fallback genérico a 400 con código más cercano
		// Si falta role -> invalid_role, si email malform -> invalid_email, etc.
		// Para simplicidad, inspeccionamos el JSON error string
		msg := err.Error()
		// Si falta email o formato, el Service volverá a validar; aquí retornamos 400 genérico
		// Pero intentamos mapear rápido para UX
		if contains(msg, "Email") && contains(msg, "required") {
			utils.RespondError(c, http.StatusBadRequest, utils.ErrInvalidEmail, utils.MessageForCode(utils.ErrInvalidEmail))
			return
		}
		if contains(msg, "Role") {
			utils.RespondError(c, http.StatusBadRequest, utils.ErrInvalidRole, utils.MessageForCode(utils.ErrInvalidRole))
			return
		}
		if contains(msg, "Password") {
			utils.RespondError(c, http.StatusBadRequest, utils.ErrWeakPassword, utils.MessageForCode(utils.ErrWeakPassword))
			return
		}
		if contains(msg, "DisplayName") || contains(msg, "display_name") {
			utils.RespondError(c, http.StatusBadRequest, utils.ErrInvalidDisplayName, utils.MessageForCode(utils.ErrInvalidDisplayName))
			return
		}
		if contains(msg, "Visibility") || contains(msg, "visibility") {
			utils.RespondError(c, http.StatusBadRequest, utils.ErrInvalidVisibility, utils.MessageForCode(utils.ErrInvalidVisibility))
			return
		}
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "Request inválido: "+msg)
		return
	}

	user, err := h.svc.Register(c.Request.Context(), req.Email, req.Password, req.Role, req.DisplayName, req.PhotoURL, req.Phone, req.Institution, req.Description, req.Visibility)
	if err != nil {
		if se, ok := err.(*service.ServiceError); ok {
			status := utils.StatusForCode(se.Code)
			utils.RespondError(c, status, se.Code, se.Message)
			return
		}
		// Error no mapeado -> 500 pero con formato contrato
		utils.RespondError(c, http.StatusInternalServerError, "internal_error", "Error interno")
		return
	}

	resp := registerResponse{
		ID:        user.ID,
		Email:     user.Email,
		Role:      user.Role,
		CreatedAt: user.CreatedAt.UTC().Format("2006-01-02T15:04:05.000000Z07:00"), // timestamptz ISO8601
	}
	// Gin por defecto no escapa; usar ISO8601 con offset
	utils.RespondSuccess(c, http.StatusCreated, resp)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}
