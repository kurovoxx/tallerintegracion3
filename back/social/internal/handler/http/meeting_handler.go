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

type MeetingHandler struct {
	svc *service.MeetingService
}

func NewMeetingHandler(svc *service.MeetingService) *MeetingHandler {
	return &MeetingHandler{svc: svc}
}

// POST /groups/{id}/meetings — contrato: agentApiContract.md sección 5
// Body: {title, description?, scheduled_at} → 201 {meeting_id}
type CreateMeetingRequest struct {
	Title         string   `json:"title" binding:"required,max=300"`
	Description   *string  `json:"description"`
	ScheduledAt   string   `json:"scheduled_at" binding:"required"`
	NotifyDiscord *bool    `json:"notify_discord"`
	Attendees     []string `json:"attendees"`
}

func (h *MeetingHandler) CreateMeeting(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("id"))
	if groupID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group ID is required", "code": "invalid_group_id"})
		return
	}
	if _, err := uuid.Parse(groupID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID format", "code": "invalid_group_id"})
		return
	}

	var req CreateMeetingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_body"})
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "code": "unauthorized"})
		return
	}

	meeting, err := h.svc.CreateMeeting(
		c.Request.Context(),
		groupID,
		userID,
		req.Title,
		req.Description,
		req.ScheduledAt,
		req.NotifyDiscord,
		req.Attendees,
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrGroupNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "group not found", "code": "group_not_found"})
		case errors.Is(err, service.ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "you must be a member of the group", "code": "forbidden"})
		case errors.Is(err, service.ErrInvalidGroupID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_group_id"})
		case errors.Is(err, service.ErrInvalidUserID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_user_id"})
		case errors.Is(err, service.ErrInvalidTitle),
			errors.Is(err, service.ErrTitleTooLong),
			errors.Is(err, service.ErrScheduledAtRequired),
			errors.Is(err, service.ErrInvalidScheduledAt),
			errors.Is(err, service.ErrInvalidAttendeeEmail),
			errors.Is(err, service.ErrTooManyAttendees):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_body"})
		default:
			log.Printf("CreateMeeting internal error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create meeting", "code": "internal"})
		}
		return
	}

	// Contrato: 201 {meeting_id}
	c.JSON(http.StatusCreated, gin.H{
		"meeting_id": uuidToString(meeting.ID),
	})
}
