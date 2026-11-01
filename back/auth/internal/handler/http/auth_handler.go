package http

import (
	"log"
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
	DisplayName *string `json:"display_name"`
	PhotoURL    *string `json:"photo_url"`
	Phone       *string `json:"phone"`
	Institution *string `json:"institution"`
	Description *string `json:"description"`
	Visibility  *string `json:"visibility"`
}

type registerResponse struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
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

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type refreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Login maneja POST /auth/login (sin auth) según agentApiContract.md:54.
// Verifica bcrypt, genera JWT corto + refresh hasheado.
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("auth login fail email=? code=bad_request ip=%s", c.ClientIP())
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "Request inválido")
		return
	}
	res, err := h.svc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		if se, ok := err.(*service.ServiceError); ok {
			status := utils.StatusForCode(se.Code)
			log.Printf("auth login fail email=%s code=%s ip=%s", req.Email, se.Code, c.ClientIP())
			utils.RespondError(c, status, se.Code, se.Message)
			return
		}
		log.Printf("auth login fail email=%s code=internal_error ip=%s err=%v", req.Email, c.ClientIP(), err)
		utils.RespondError(c, http.StatusInternalServerError, "internal_error", "Error interno")
		return
	}
	log.Printf("auth login ok email=%s ip=%s", req.Email, c.ClientIP())
	c.JSON(http.StatusOK, loginResponse{
		AccessToken:  res.AccessToken,
		RefreshToken: res.RefreshToken,
		ExpiresIn:    res.ExpiresIn,
	})
}

// Refresh maneja POST /auth/refresh (sin auth) según agentApiContract.md:24.
// Valida refresh_token (hash bcrypt, no revocado, no expirado), lo rota y emite nuevo access_token.
// Contrato actual: 200 {access_token, expires_in} + nuevo refresh_token por rotación, 401 si revocado/expirado/inexistente.
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("auth refresh fail code=bad_request ip=%s", c.ClientIP())
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "Request inválido: refresh_token requerido")
		return
	}
	res, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		if se, ok := err.(*service.ServiceError); ok {
			status := utils.StatusForCode(se.Code)
			// Refresh siempre es 401 para token inválido/expirado, nunca 403
			if se.Code == utils.ErrInvalidToken || se.Code == utils.ErrTokenExpired || se.Code == utils.ErrUnauthorized {
				status = http.StatusUnauthorized
			}
			log.Printf("auth refresh fail code=%s ip=%s", se.Code, c.ClientIP())
			utils.RespondError(c, status, se.Code, se.Message)
			return
		}
		log.Printf("auth refresh fail code=internal_error ip=%s err=%v", c.ClientIP(), err)
		utils.RespondError(c, http.StatusInternalServerError, "internal_error", "Error interno")
		return
	}
	log.Printf("auth refresh ok ip=%s", c.ClientIP())
	c.JSON(http.StatusOK, refreshResponse{
		AccessToken:  res.AccessToken,
		RefreshToken: res.RefreshToken,
		ExpiresIn:    res.ExpiresIn,
	})
}

// Logout maneja POST /auth/logout (sin auth) — idempotente 204.
// Body {refresh_token}. Si token no existe, ya revocado o expirado, sigue 204 (no revela existencia).
// Solo 400 si JSON malformado o refresh_token vacío.
func (h *AuthHandler) Logout(c *gin.Context) {
	var req logoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("auth logout fail code=bad_request ip=%s", c.ClientIP())
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "Request inválido: refresh_token requerido")
		return
	}
	if err := h.svc.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		if se, ok := err.(*service.ServiceError); ok {
			if se.Code == "bad_request" {
				log.Printf("auth logout fail code=bad_request ip=%s", c.ClientIP())
				utils.RespondError(c, http.StatusBadRequest, se.Code, se.Message)
				return
			}
		}
		log.Printf("auth logout fail code=internal_error ip=%s err=%v", c.ClientIP(), err)
		utils.RespondError(c, http.StatusInternalServerError, "internal_error", "Error interno")
		return
	}
	log.Printf("auth logout ok ip=%s", c.ClientIP())
	c.AbortWithStatus(http.StatusNoContent)
}

// Register maneja POST /auth/register (sin auth).
// Contrato vigente (agentApiContract.md:13): 201 {id,email,created_at} con body {email,password} (display_name derivado del email).
func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		msg := err.Error()
		if contains(msg, "Email") && contains(msg, "required") {
			log.Printf("auth register fail email=? code=%s ip=%s", utils.ErrInvalidEmail, c.ClientIP())
			utils.RespondError(c, http.StatusBadRequest, utils.ErrInvalidEmail, utils.MessageForCode(utils.ErrInvalidEmail))
			return
		}
		if contains(msg, "Password") {
			log.Printf("auth register fail email=%s code=%s ip=%s", req.Email, utils.ErrWeakPassword, c.ClientIP())
			utils.RespondError(c, http.StatusBadRequest, utils.ErrWeakPassword, utils.MessageForCode(utils.ErrWeakPassword))
			return
		}
		if contains(msg, "Visibility") || contains(msg, "visibility") {
			log.Printf("auth register fail email=%s code=%s ip=%s", req.Email, utils.ErrInvalidVisibility, c.ClientIP())
			utils.RespondError(c, http.StatusBadRequest, utils.ErrInvalidVisibility, utils.MessageForCode(utils.ErrInvalidVisibility))
			return
		}
		log.Printf("auth register fail email=%s code=bad_request ip=%s", req.Email, c.ClientIP())
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "Request inválido: "+msg)
		return
	}

	user, err := h.svc.Register(c.Request.Context(), req.Email, req.Password, req.DisplayName, req.PhotoURL, req.Phone, req.Institution, req.Description, req.Visibility)
	if err != nil {
		if se, ok := err.(*service.ServiceError); ok {
			status := utils.StatusForCode(se.Code)
			log.Printf("auth register fail email=%s code=%s ip=%s", req.Email, se.Code, c.ClientIP())
			utils.RespondError(c, status, se.Code, se.Message)
			return
		}
		log.Printf("auth register fail email=%s code=internal_error ip=%s err=%v", req.Email, c.ClientIP(), err)
		utils.RespondError(c, http.StatusInternalServerError, "internal_error", "Error interno")
		return
	}

	log.Printf("auth register ok id=%s email=%s ip=%s", user.ID, user.Email, c.ClientIP())
	resp := registerResponse{
		ID:        user.ID,
		Email:     user.Email,
		CreatedAt: user.CreatedAt.UTC().Format("2006-01-02T15:04:05.000000Z07:00"),
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
