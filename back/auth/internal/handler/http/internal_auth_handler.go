package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/utils"
)

// InternalAuthHandler expone verificación de JWT para servicio-a-servicio
// (tarea 3_5_6: VerifyToken). Protegido con RequireInternalKey
// (X-Internal-Key), NO con JWT de usuario.
type InternalAuthHandler struct {
	jwt *service.JWTService
}

func NewInternalAuthHandler(jwt *service.JWTService) *InternalAuthHandler {
	return &InternalAuthHandler{jwt: jwt}
}

type verifyTokenRequest struct {
	Token string `json:"token" binding:"required"`
}

// Verify maneja POST /internal/auth/verify.
// Body: {token} → 200 {valid:true, user_id} si el access token es válido;
// 401 {error:{code:invalid_token|token_expired}} si no;
// 400 bad_request si falta el token.
func (h *InternalAuthHandler) Verify(c *gin.Context) {
	var req verifyTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Token) == "" {
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "token requerido")
		return
	}
	user, err := h.jwt.ValidarAccessToken(strings.TrimSpace(req.Token))
	if err != nil {
		code := utils.ErrInvalidToken
		if errors.Is(err, jwt.ErrTokenExpired) || strings.Contains(strings.ToLower(err.Error()), "expired") {
			code = utils.ErrTokenExpired
		}
		utils.RespondError(c, http.StatusUnauthorized, code, utils.MessageForCode(code))
		return
	}
	c.JSON(http.StatusOK, gin.H{"valid": true, "user_id": user.ID})
}
