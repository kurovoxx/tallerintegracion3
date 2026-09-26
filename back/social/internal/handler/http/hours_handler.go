package http

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

type HoursHandler struct {
	svc *service.HoursService
}

func NewHoursHandler(svc *service.HoursService) *HoursHandler {
	return &HoursHandler{
		svc: svc,
	}
}

type LogHoursRequest struct {
	LogDate string  `json:"log_date" binding:"required"`
	Hours   float64 `json:"hours" binding:"gte=0"`
}

func (h *HoursHandler) LogHours(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("taskId"))
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task ID is required", "code": "invalid_sprint_task_id"})
		return
	}
	if _, err := uuid.Parse(taskID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task ID format", "code": "invalid_sprint_task_id"})
		return
	}

	var req LogHoursRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_body"})
		return
	}

	if _, exists := c.Get("user_id"); !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "code": "unauthorized"})
		return
	}

	entry, created, err := h.svc.LogHours(
		c.Request.Context(),
		taskID,
		strings.TrimSpace(req.LogDate),
		req.Hours,
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrSprintTaskNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "sprint task not found", "code": "sprint_task_not_found"})
		case errors.Is(err, service.ErrInvalidSprintTaskID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_sprint_task_id"})
		case errors.Is(err, service.ErrInvalidLogDate),
			errors.Is(err, service.ErrInvalidHours):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_body"})
		default:
			log.Printf("LogHours internal error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not register hours", "code": "internal"})
		}
		return
	}

	if created {
		c.JSON(http.StatusCreated, gin.H{
			"message": "daily hours registered successfully",
			"data":    dailyHoursToJSON(entry),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "daily hours updated successfully",
		"data":    dailyHoursToJSON(entry),
	})
}

func (h *HoursHandler) ListHours(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("taskId"))
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task ID is required", "code": "invalid_sprint_task_id"})
		return
	}
	if _, err := uuid.Parse(taskID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task ID format", "code": "invalid_sprint_task_id"})
		return
	}

	if _, exists := c.Get("user_id"); !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "code": "unauthorized"})
		return
	}

	entries, total, err := h.svc.ListHours(
		c.Request.Context(),
		taskID,
		strings.TrimSpace(c.Query("from")),
		strings.TrimSpace(c.Query("to")),
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrSprintTaskNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "sprint task not found", "code": "sprint_task_not_found"})
		case errors.Is(err, service.ErrInvalidSprintTaskID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_sprint_task_id"})
		case errors.Is(err, service.ErrInvalidLogDate):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_body"})
		default:
			log.Printf("ListHours internal error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list hours", "code": "internal"})
		}
		return
	}

	data := make([]gin.H, 0, len(entries))
	for _, e := range entries {
		data = append(data, dailyHoursToJSON(e))
	}
	c.JSON(http.StatusOK, gin.H{"data": data, "total_hours": total})
}

func dailyHoursToJSON(e sqlc.SocialSprintSheetDailyHour) gin.H {
	return gin.H{
		"id":       uuidToString(e.ID),
		"task_id":  uuidToString(e.TaskID),
		"log_date": dateToNil(e.LogDate),
		"hours":    numericToFloat(e.Hours),
	}
}
