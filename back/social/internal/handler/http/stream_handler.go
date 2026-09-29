package http

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

type StreamHandler struct {
	svc *service.StreamTokenService
}

func NewStreamHandler(svc *service.StreamTokenService) *StreamHandler {
	return &StreamHandler{svc: svc}
}

// GET /groups/{id}/stream-token — contrato: agentApiContract.md sección 5.
// Token firmado con el API secret para que el cliente se conecte directo.
// Solo miembros (403 si no).
func (h *StreamHandler) Token(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("id"))
	if groupID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group ID is required", "code": "invalid_group_id"})
		return
	}
	if _, err := uuid.Parse(groupID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID format", "code": "invalid_group_id"})
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "code": "unauthorized"})
		return
	}

	token, channelID, err := h.svc.IssueToken(c.Request.Context(), groupID, userID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrGroupNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "group not found", "code": "group_not_found"})
		case errors.Is(err, service.ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "you must be a member of the group", "code": "forbidden"})
		case errors.Is(err, service.ErrStreamNotConfigured):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error(), "code": "stream_not_configured"})
		case errors.Is(err, service.ErrInvalidGroupID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_group_id"})
		case errors.Is(err, service.ErrInvalidUserID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_user_id"})
		default:
			log.Printf("StreamToken internal error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not issue stream token", "code": "internal"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":      token,
		"channel_id": channelID,
	})
}
