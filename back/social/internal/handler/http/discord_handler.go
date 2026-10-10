package http

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

type DiscordHandler struct {
	svc *service.DiscordService
}

func NewDiscordHandler(svc *service.DiscordService) *DiscordHandler {
	return &DiscordHandler{svc: svc}
}

// RegisterRoutes registra las rutas de Discord en un router ya protegido por
// el middleware de autenticación. main.go y los tests comparten esta lista.
func (h *DiscordHandler) RegisterRoutes(r gin.IRoutes) {
	r.GET("/groups/:id/discord-config", h.GetConfig)
	r.PUT("/groups/:id/discord-config", h.PutConfig)
}

// GetConfig maneja GET /groups/{id}/discord-config.
// Solo miembros. 200 {server_name, invite_url, webhook_url?};
// 404 discord_not_configured si nunca se configuró.
func (h *DiscordHandler) GetConfig(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("id"))
	if groupID == "" {
		fail(c, "GetConfig", http.StatusBadRequest, "invalid_group_id", "group ID is required")
		return
	}
	if _, err := uuid.Parse(groupID); err != nil {
		fail(c, "GetConfig", http.StatusBadRequest, "invalid_group_id", "invalid group ID format")
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok {
		fail(c, "GetConfig", http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	cfg, err := h.svc.GetConfig(c.Request.Context(), groupID, userID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrGroupNotFound):
			fail(c, "GetConfig", http.StatusNotFound, "group_not_found", "group not found")
		case errors.Is(err, service.ErrForbidden):
			fail(c, "GetConfig", http.StatusForbidden, "forbidden", "you must be a member of the group")
		case errors.Is(err, service.ErrDiscordNotConfigured):
			fail(c, "GetConfig", http.StatusNotFound, "discord_not_configured", "discord not configured for this group")
		case errors.Is(err, service.ErrInvalidGroupID):
			fail(c, "GetConfig", http.StatusBadRequest, "invalid_group_id", err.Error())
		case errors.Is(err, service.ErrInvalidUserID):
			fail(c, "GetConfig", http.StatusBadRequest, "invalid_user_id", err.Error())
		default:
			log.Printf("GetDiscordConfig internal error: %v", err)
			fail(c, "GetConfig", http.StatusInternalServerError, "internal", "could not load discord config")
		}
		return
	}

	logOp(c, "PutDiscordConfig", http.StatusOK, "group_id="+groupID)
	c.JSON(http.StatusOK, gin.H{
		"server_name": pgTextToString(cfg.ServerName),
		"invite_url":  pgTextToString(cfg.InviteUrl),
		"webhook_url": pgTextToNil(cfg.WebhookUrl),
	})
}

// PUT /groups/{id}/discord-config — contrato: agentApiContract.md sección 5
// Body: {server_name, invite_url, webhook_url?} → 200. Solo admin.
type PutDiscordConfigRequest struct {
	ServerName string `json:"server_name" binding:"required,max=200"`
	InviteURL  string `json:"invite_url" binding:"required,max=500"`
	WebhookURL string `json:"webhook_url" binding:"max=500"`
}

func (h *DiscordHandler) PutConfig(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("id"))
	if groupID == "" {
		fail(c, "PutConfig", http.StatusBadRequest, "invalid_group_id", "group ID is required")
		return
	}
	if _, err := uuid.Parse(groupID); err != nil {
		fail(c, "PutConfig", http.StatusBadRequest, "invalid_group_id", "invalid group ID format")
		return
	}

	var req PutDiscordConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "PutConfig", http.StatusBadRequest, "invalid_body", err.Error())
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok {
		fail(c, "PutConfig", http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	cfg, err := h.svc.PutConfig(
		c.Request.Context(),
		groupID,
		userID,
		req.ServerName,
		req.InviteURL,
		req.WebhookURL,
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrGroupNotFound):
			fail(c, "PutConfig", http.StatusNotFound, "group_not_found", "group not found")
		case errors.Is(err, service.ErrForbidden):
			fail(c, "PutConfig", http.StatusForbidden, "forbidden", "only admins can configure discord")
		case errors.Is(err, service.ErrInvalidGroupID):
			fail(c, "PutConfig", http.StatusBadRequest, "invalid_group_id", err.Error())
		case errors.Is(err, service.ErrInvalidUserID):
			fail(c, "PutConfig", http.StatusBadRequest, "invalid_user_id", err.Error())
		case errors.Is(err, service.ErrServerNameEmpty),
			errors.Is(err, service.ErrInviteURLEmpty),
			errors.Is(err, service.ErrInvalidWebhookURL):
			fail(c, "PutConfig", http.StatusBadRequest, "invalid_body", err.Error())
		default:
			log.Printf("PutDiscordConfig internal error: %v", err)
			fail(c, "PutConfig", http.StatusInternalServerError, "internal", "could not save discord config")
		}
		return
	}

	logOp(c, "GetDiscordConfig", http.StatusOK, "group_id="+groupID)
	c.JSON(http.StatusOK, gin.H{
		"message":     "discord config saved successfully",
		"server_name": pgTextToString(cfg.ServerName),
		"invite_url":  pgTextToString(cfg.InviteUrl),
		"webhook_url": pgTextToNil(cfg.WebhookUrl),
	})
}

func pgTextToString(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}

func pgTextToNil(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	s := t.String
	return &s
}
