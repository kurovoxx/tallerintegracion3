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
		fail(c, "CreateMeeting", http.StatusBadRequest, "invalid_group_id", "group ID is required")
		return
	}
	if _, err := uuid.Parse(groupID); err != nil {
		fail(c, "CreateMeeting", http.StatusBadRequest, "invalid_group_id", "invalid group ID format")
		return
	}

	var req CreateMeetingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "CreateMeeting", http.StatusBadRequest, "invalid_body", err.Error())
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok {
		fail(c, "CreateMeeting", http.StatusUnauthorized, "unauthorized", "unauthorized")
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
			fail(c, "CreateMeeting", http.StatusNotFound, "group_not_found", "group not found")
		case errors.Is(err, service.ErrForbidden):
			fail(c, "CreateMeeting", http.StatusForbidden, "forbidden", "you must be a member of the group")
		case errors.Is(err, service.ErrInvalidGroupID):
			fail(c, "CreateMeeting", http.StatusBadRequest, "invalid_group_id", err.Error())
		case errors.Is(err, service.ErrInvalidUserID):
			fail(c, "CreateMeeting", http.StatusBadRequest, "invalid_user_id", err.Error())
		case errors.Is(err, service.ErrInvalidTitle),
			errors.Is(err, service.ErrTitleTooLong),
			errors.Is(err, service.ErrScheduledAtRequired),
			errors.Is(err, service.ErrInvalidScheduledAt),
			errors.Is(err, service.ErrInvalidAttendeeEmail),
			errors.Is(err, service.ErrTooManyAttendees):
			fail(c, "CreateMeeting", http.StatusBadRequest, "invalid_body", err.Error())
		default:
			log.Printf("CreateMeeting internal error: %v", err)
			fail(c, "CreateMeeting", http.StatusInternalServerError, "internal", "could not create meeting")
		}
		return
	}

	// Contrato: 201 {meeting_id}
	logOp(c, "CreateMeeting", http.StatusCreated, "meeting_id="+uuidToString(meeting.ID))
	c.JSON(http.StatusCreated, gin.H{
		"meeting_id": uuidToString(meeting.ID),
	})
}
