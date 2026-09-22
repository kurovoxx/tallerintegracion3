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
		c.JSON(http.StatusBadRequest, gin.H{"error": "group ID is required", "code": "invalid_group_id"})
		return
	}
	if _, err := uuid.Parse(groupID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID format", "code": "invalid_group_id"})
		return
	}

	var req PutDiscordConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_body"})
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "code": "unauthorized"})
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
			c.JSON(http.StatusNotFound, gin.H{"error": "group not found", "code": "group_not_found"})
		case errors.Is(err, service.ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "only admins can configure discord", "code": "forbidden"})
		case errors.Is(err, service.ErrInvalidGroupID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_group_id"})
		case errors.Is(err, service.ErrInvalidUserID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_user_id"})
		case errors.Is(err, service.ErrServerNameEmpty),
			errors.Is(err, service.ErrInviteURLEmpty),
			errors.Is(err, service.ErrInvalidWebhookURL):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_body"})
		default:
			log.Printf("PutDiscordConfig internal error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save discord config", "code": "internal"})
		}
		return
	}

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
