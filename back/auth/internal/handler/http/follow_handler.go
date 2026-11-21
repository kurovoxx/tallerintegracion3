package http

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/utils"
)

// FollowHandler maneja seguir/dejar de seguir usuarios.
// Endpoints protegidos por JWT de usuario.
// Todas las rutas usan public_id (identificador público de 12 chars) en la
// ruta, nunca el UUID interno.
type FollowHandler struct {
	svc *service.FollowService
}

func NewFollowHandler(svc *service.FollowService) *FollowHandler {
	return &FollowHandler{svc: svc}
}

// RegisterRoutes registra las rutas de follow en un router ya protegido.
func (h *FollowHandler) RegisterRoutes(r gin.IRoutes) {
	r.POST("/profile/follow/:public_id", h.Follow)
	r.DELETE("/profile/follow/:public_id", h.Unfollow)
	r.GET("/profile/follow-status/:public_id", h.FollowStatus)
	r.GET("/profile/followers/:public_id", h.ListFollowers)
	r.GET("/profile/following/:public_id", h.ListFollowing)
}

// followResponse es la respuesta de follow/unfollow/follow-status.
type followResponse struct {
	PublicID  string `json:"public_id"`
	Following bool   `json:"following"`
	Message   string `json:"message"`
}

// followedUserResponse expone un usuario seguido/seguidor mínimo.
type followedUserResponse struct {
	UserID        string  `json:"user_id"`
	PublicID      string  `json:"public_id,omitempty"`
	DisplayName   string  `json:"display_name"`
	PhotoThumbURL *string `json:"photo_thumb_url,omitempty"`
	FollowedAt    string  `json:"followed_at"`
}

// Follow maneja POST /profile/follow/:public_id.
func (h *FollowHandler) Follow(c *gin.Context) {
	started := time.Now()
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == "" {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	publicID := strings.TrimSpace(c.Param("public_id"))
	if publicID == "" {
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "public_id requerido")
		return
	}
	uid, err := uuid.Parse(userID)
	if err != nil {
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "user_id inválido")
		return
	}
	var pgUID pgtype.UUID
	if err := pgUID.Scan(uid); err != nil {
		utils.RespondError(c, http.StatusInternalServerError, "internal_error", "Error interno")
		return
	}

	created, err := h.svc.Follow(c.Request.Context(), pgUID, publicID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFollowSelf):
			utils.RespondError(c, http.StatusBadRequest, "cannot_follow_self", err.Error())
		case errors.Is(err, service.ErrUserNotFound):
			utils.RespondError(c, http.StatusNotFound, utils.ErrUserNotFound, utils.MessageForCode(utils.ErrUserNotFound))
		default:
			utils.RespondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}
		logAuthOp(c, "Follow", http.StatusBadRequest, userID, started, "public_id="+publicID+" err="+err.Error())
		return
	}

	msg := "Ahora sigues a este usuario."
	if !created {
		msg = "Ya seguías a este usuario."
	}
	logAuthOp(c, "Follow", http.StatusOK, userID, started, "public_id="+publicID+" created="+boolToString(created))
	c.JSON(http.StatusOK, followResponse{
		PublicID:  publicID,
		Following: true,
		Message:   msg,
	})
}

// Unfollow maneja DELETE /profile/follow/:public_id.
func (h *FollowHandler) Unfollow(c *gin.Context) {
	started := time.Now()
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == "" {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	publicID := strings.TrimSpace(c.Param("public_id"))
	if publicID == "" {
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "public_id requerido")
		return
	}
	uid, err := uuid.Parse(userID)
	if err != nil {
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "user_id inválido")
		return
	}
	var pgUID pgtype.UUID
	if err := pgUID.Scan(uid); err != nil {
		utils.RespondError(c, http.StatusInternalServerError, "internal_error", "Error interno")
		return
	}

	removed, err := h.svc.Unfollow(c.Request.Context(), pgUID, publicID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFollowSelf):
			utils.RespondError(c, http.StatusBadRequest, "cannot_unfollow_self", err.Error())
		case errors.Is(err, service.ErrUserNotFound):
			utils.RespondError(c, http.StatusNotFound, utils.ErrUserNotFound, utils.MessageForCode(utils.ErrUserNotFound))
		default:
			utils.RespondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}
		logAuthOp(c, "Unfollow", http.StatusBadRequest, userID, started, "public_id="+publicID+" err="+err.Error())
		return
	}

	msg := "Has dejado de seguir a este usuario."
	if !removed {
		msg = "Ya no seguías a este usuario."
	}
	logAuthOp(c, "Unfollow", http.StatusOK, userID, started, "public_id="+publicID+" removed="+boolToString(removed))
	c.JSON(http.StatusOK, followResponse{
		PublicID:  publicID,
		Following: false,
		Message:   msg,
	})
}

// FollowStatus maneja GET /profile/follow-status/:public_id.
func (h *FollowHandler) FollowStatus(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == "" {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	publicID := strings.TrimSpace(c.Param("public_id"))
	if publicID == "" {
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "public_id requerido")
		return
	}
	uid, err := uuid.Parse(userID)
	if err != nil {
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "user_id inválido")
		return
	}
	var pgUID pgtype.UUID
	if err := pgUID.Scan(uid); err != nil {
		utils.RespondError(c, http.StatusInternalServerError, "internal_error", "Error interno")
		return
	}

	following, err := h.svc.IsFollowing(c.Request.Context(), pgUID, publicID)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			utils.RespondError(c, http.StatusNotFound, utils.ErrUserNotFound, utils.MessageForCode(utils.ErrUserNotFound))
		} else {
			utils.RespondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}
		return
	}
	c.JSON(http.StatusOK, followResponse{
		PublicID:  publicID,
		Following: following,
		Message:   estadoSeguir(following),
	})
}

// ListFollowers maneja GET /profile/followers/:public_id.
// Devuelve quiénes siguen al usuario con public_id.
func (h *FollowHandler) ListFollowers(c *gin.Context) {
	publicID := strings.TrimSpace(c.Param("public_id"))
	if publicID == "" {
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "public_id requerido")
		return
	}
	users, err := h.svc.ListFollowers(c.Request.Context(), publicID)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			utils.RespondError(c, http.StatusNotFound, utils.ErrUserNotFound, utils.MessageForCode(utils.ErrUserNotFound))
		} else {
			utils.RespondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}
		return
	}
	out := make([]followedUserResponse, 0, len(users))
	for _, u := range users {
		pid := ""
		if u.PublicID != nil {
			pid = *u.PublicID
		}
		out = append(out, followedUserResponse{
			UserID:        u.UserID,
			PublicID:      pid,
			DisplayName:   u.DisplayName,
			PhotoThumbURL: u.PhotoThumbURL,
			FollowedAt:    u.FollowedAt.UTC().Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// ListFollowing maneja GET /profile/following/:public_id.
// Devuelve a quién sigue el usuario con public_id.
func (h *FollowHandler) ListFollowing(c *gin.Context) {
	publicID := strings.TrimSpace(c.Param("public_id"))
	if publicID == "" {
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "public_id requerido")
		return
	}
	users, err := h.svc.ListFollowing(c.Request.Context(), publicID)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			utils.RespondError(c, http.StatusNotFound, utils.ErrUserNotFound, utils.MessageForCode(utils.ErrUserNotFound))
		} else {
			utils.RespondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		}
		return
	}
	out := make([]followedUserResponse, 0, len(users))
	for _, u := range users {
		pid := ""
		if u.PublicID != nil {
			pid = *u.PublicID
		}
		out = append(out, followedUserResponse{
			UserID:        u.UserID,
			PublicID:      pid,
			DisplayName:   u.DisplayName,
			PhotoThumbURL: u.PhotoThumbURL,
			FollowedAt:    u.FollowedAt.UTC().Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func estadoSeguir(following bool) string {
	if following {
		return "Ya sigues a este usuario."
	}
	return "No sigues a este usuario."
}
