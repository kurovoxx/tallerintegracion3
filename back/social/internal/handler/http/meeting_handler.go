package http

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

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

// ListMeetings maneja GET /groups/{id}/meetings (tarea 3_5_14: ventana
// "próximas reuniones" del front). Solo miembros; scheduled_at >= now,
// en orden cronológico. 200 {data: [...]}.
func (h *MeetingHandler) ListMeetings(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("id"))
	if _, err := uuid.Parse(groupID); err != nil {
		fail(c, "ListMeetings", http.StatusBadRequest, "invalid_group_id", "invalid group ID format")
		return
	}
	userID, ok := middleware.GetUserID(c)
	if !ok {
		fail(c, "ListMeetings", http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	meetings, err := h.svc.ListUpcomingMeetings(c.Request.Context(), groupID, userID, time.Now())
	if err != nil {
		switch {
		case errors.Is(err, service.ErrGroupNotFound):
			fail(c, "ListMeetings", http.StatusNotFound, "group_not_found", "group not found")
		case errors.Is(err, service.ErrForbidden):
			fail(c, "ListMeetings", http.StatusForbidden, "forbidden", "you must be a member of the group")
		case errors.Is(err, service.ErrInvalidGroupID):
			fail(c, "ListMeetings", http.StatusBadRequest, "invalid_group_id", err.Error())
		case errors.Is(err, service.ErrInvalidUserID):
			fail(c, "ListMeetings", http.StatusBadRequest, "invalid_user_id", err.Error())
		default:
			log.Printf("ListMeetings internal error: %v", err)
			fail(c, "ListMeetings", http.StatusInternalServerError, "internal", "could not list meetings")
		}
		return
	}
	data := make([]gin.H, 0, len(meetings))
	for _, m := range meetings {
		var desc *string
		if m.Description.Valid {
			s := m.Description.String
			desc = &s
		}
		var scheduled string
		if m.ScheduledAt.Valid {
			scheduled = m.ScheduledAt.Time.UTC().Format(time.RFC3339)
		}
		data = append(data, gin.H{
			"id":           uuidToString(m.ID),
			"group_id":     uuidToString(m.GroupID),
			"title":        m.Title,
			"description":  desc,
			"scheduled_at": scheduled,
			"created_by":   uuidToString(m.CreatedByUserID),
		})
	}
	logOp(c, "ListMeetings", http.StatusOK, "")
	c.JSON(http.StatusOK, gin.H{"data": data})
}
