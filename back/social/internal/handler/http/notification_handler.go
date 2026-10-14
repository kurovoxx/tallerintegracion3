package http

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

// NotificationHandler expone las notificaciones in-app de reuniones
// (tarea 3_5_14: cajita de notificaciones del front). Solo auth JWT;
// cada usuario ve únicamente las suyas.
type NotificationHandler struct {
	svc *service.NotificationService
}

func NewNotificationHandler(svc *service.NotificationService) *NotificationHandler {
	return &NotificationHandler{svc: svc}
}

func notificationToJSON(it model.MeetingNotificationItem) gin.H {
	var readAt *string
	if it.ReadAt != nil {
		s := it.ReadAt.UTC().Format(time.RFC3339)
		readAt = &s
	}
	return gin.H{
		"id":           it.ID,
		"meeting_id":   it.MeetingID,
		"group_id":     it.GroupID,
		"title":        it.Title,
		"scheduled_at": it.ScheduledAt.UTC().Format(time.RFC3339),
		"read":         it.ReadAt != nil,
		"read_at":      readAt,
		"created_at":   it.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// ListMine maneja GET /notifications/me. 200 {data: [...]} (vacío = []).
func (h *NotificationHandler) ListMine(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		fail(c, "ListNotifications", http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	items, err := h.svc.ListMine(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, service.ErrInvalidUserID) {
			fail(c, "ListNotifications", http.StatusBadRequest, "invalid_user_id", err.Error())
			return
		}
		fail(c, "ListNotifications", http.StatusInternalServerError, "internal", "could not list notifications")
		return
	}
	data := make([]gin.H, 0, len(items))
	for _, it := range items {
		data = append(data, notificationToJSON(it))
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// MarkRead maneja POST /notifications/:id/read.
// 200 {message} | 404 ajena/inexistente/ya leída (sin filtrar).
func (h *NotificationHandler) MarkRead(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		fail(c, "MarkNotificationRead", http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	if err := h.svc.MarkRead(c.Request.Context(), userID, c.Param("id")); err != nil {
		if errors.Is(err, service.ErrMeetingNotFound) {
			fail(c, "MarkNotificationRead", http.StatusNotFound, "notification_not_found", "notification not found")
			return
		}
		if errors.Is(err, service.ErrInvalidUserID) {
			fail(c, "MarkNotificationRead", http.StatusBadRequest, "invalid_user_id", err.Error())
			return
		}
		fail(c, "MarkNotificationRead", http.StatusInternalServerError, "internal", "could not mark notification as read")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "notification marked as read"})
}
