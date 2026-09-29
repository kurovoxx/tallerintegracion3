package http

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

type SprintHandler struct {
	svc *service.SprintService
}

func NewSprintHandler(svc *service.SprintService) *SprintHandler {
	return &SprintHandler{
		svc: svc,
	}
}

type CreateSprintTaskRequest struct {
	Title          string   `json:"title" binding:"required,max=300"`
	AssignedTo     string   `json:"assigned_to" binding:"required"`
	SheetID        string   `json:"sheet_id"`
	Priority       string   `json:"priority" binding:"omitempty,oneof=alta media baja"`
	Status         string   `json:"status" binding:"omitempty,oneof=sin_empezar en_proceso listo"`
	EstimatedHours *float64 `json:"estimated_hours"`
}

func (h *SprintHandler) CreateSprintTask(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("id"))
	if groupID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group ID is required", "code": "invalid_group_id"})
		return
	}
	if _, err := uuid.Parse(groupID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID format", "code": "invalid_group_id"})
		return
	}

	var req CreateSprintTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_body"})
		return
	}
	if strings.TrimSpace(req.SheetID) != "" {
		if _, err := uuid.Parse(strings.TrimSpace(req.SheetID)); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sheet_id format", "code": "invalid_sheet_id"})
			return
		}
	}

	uidVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "code": "unauthorized"})
		return
	}
	userID, _ := uidVal.(string)

	view, err := h.svc.CreateSprintTask(
		c.Request.Context(),
		groupID,
		userID,
		strings.TrimSpace(req.SheetID),
		req.Title,
		strings.TrimSpace(req.AssignedTo),
		req.Priority,
		req.Status,
		req.EstimatedHours,
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrGroupNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "group not found", "code": "group_not_found"})
		case errors.Is(err, service.ErrSheetNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "sheet not found in group", "code": "sheet_not_found"})
		case errors.Is(err, service.ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "you must be a member of the group", "code": "forbidden"})
		case errors.Is(err, service.ErrInvalidGroupID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_group_id"})
		case errors.Is(err, service.ErrInvalidSheetID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_sheet_id"})
		case errors.Is(err, service.ErrInvalidUserID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_user_id"})
		case errors.Is(err, service.ErrInvalidTitle),
			errors.Is(err, service.ErrTitleTooLong),
			errors.Is(err, service.ErrInvalidPriority),
			errors.Is(err, service.ErrInvalidSprintStatus),
			errors.Is(err, service.ErrAssigneeRequired),
			errors.Is(err, service.ErrAssigneeNotMember),
			errors.Is(err, service.ErrInvalidEstimatedHours):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_body"})
		default:
			log.Printf("CreateSprintTask internal error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create sprint task", "code": "internal"})
		}
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "sprint task created successfully",
		"data":    sprintTaskToJSON(view),
	})
}

func (h *SprintHandler) ListSprintTasks(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("id"))
	if groupID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group ID is required", "code": "invalid_group_id"})
		return
	}
	if _, err := uuid.Parse(groupID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID format", "code": "invalid_group_id"})
		return
	}

	uidVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "code": "unauthorized"})
		return
	}
	userID, _ := uidVal.(string)

	views, err := h.svc.ListSprintTasks(
		c.Request.Context(),
		groupID,
		userID,
		strings.TrimSpace(c.Query("status")),
		strings.TrimSpace(c.Query("priority")),
		strings.TrimSpace(c.Query("sheet_id")),
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrGroupNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "group not found", "code": "group_not_found"})
		case errors.Is(err, service.ErrSheetNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "sheet not found in group", "code": "sheet_not_found"})
		case errors.Is(err, service.ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "you must be a member of the group", "code": "forbidden"})
		case errors.Is(err, service.ErrInvalidGroupID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_group_id"})
		case errors.Is(err, service.ErrInvalidSheetID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_sheet_id"})
		case errors.Is(err, service.ErrInvalidSprintStatus):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_status"})
		case errors.Is(err, service.ErrInvalidPriority):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_priority"})
		default:
			log.Printf("ListSprintTasks internal error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list sprint tasks", "code": "internal"})
		}
		return
	}

	data := make([]gin.H, 0, len(views))
	for _, v := range views {
		data = append(data, sprintTaskToJSON(v))
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

type UpdateSprintTaskRequest struct {
	Title          *string  `json:"title" binding:"omitempty,max=300"`
	AssignedTo     *string  `json:"assigned_to"`
	Priority       *string  `json:"priority" binding:"omitempty,oneof=alta media baja"`
	Status         *string  `json:"status" binding:"omitempty,oneof=sin_empezar en_proceso listo"`
	EstimatedHours *float64 `json:"estimated_hours"`
}

func (h *SprintHandler) UpdateSprintTask(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("id"))
	if groupID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group ID is required", "code": "invalid_group_id"})
		return
	}
	if _, err := uuid.Parse(groupID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID format", "code": "invalid_group_id"})
		return
	}
	taskID := strings.TrimSpace(c.Param("taskId"))
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sprint task ID is required", "code": "invalid_sprint_task_id"})
		return
	}
	if _, err := uuid.Parse(taskID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sprint task ID format", "code": "invalid_sprint_task_id"})
		return
	}

	var req UpdateSprintTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_body"})
		return
	}

	uidVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "code": "unauthorized"})
		return
	}
	userID, _ := uidVal.(string)

	view, err := h.svc.UpdateSprintTask(
		c.Request.Context(),
		groupID,
		userID,
		taskID,
		req.Title,
		req.AssignedTo,
		req.Priority,
		req.Status,
		req.EstimatedHours,
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrGroupNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "group not found", "code": "group_not_found"})
		case errors.Is(err, service.ErrSprintTaskNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "sprint task not found", "code": "sprint_task_not_found"})
		case errors.Is(err, service.ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "you must be a member of the group", "code": "forbidden"})
		case errors.Is(err, service.ErrInvalidGroupID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_group_id"})
		case errors.Is(err, service.ErrInvalidSprintTaskID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_sprint_task_id"})
		case errors.Is(err, service.ErrInvalidUserID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_user_id"})
		case errors.Is(err, service.ErrInvalidTitle),
			errors.Is(err, service.ErrTitleTooLong),
			errors.Is(err, service.ErrInvalidPriority),
			errors.Is(err, service.ErrInvalidSprintStatus),
			errors.Is(err, service.ErrAssigneeRequired),
			errors.Is(err, service.ErrAssigneeNotMember),
			errors.Is(err, service.ErrInvalidEstimatedHours),
			errors.Is(err, service.ErrNothingToPatch):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_body"})
		default:
			log.Printf("UpdateSprintTask internal error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update sprint task", "code": "internal"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "sprint task updated successfully",
		"data":    sprintTaskToJSON(view),
	})
}

func (h *SprintHandler) DeleteSprintTask(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("id"))
	if groupID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group ID is required", "code": "invalid_group_id"})
		return
	}
	if _, err := uuid.Parse(groupID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID format", "code": "invalid_group_id"})
		return
	}
	taskID := strings.TrimSpace(c.Param("taskId"))
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sprint task ID is required", "code": "invalid_sprint_task_id"})
		return
	}
	if _, err := uuid.Parse(taskID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sprint task ID format", "code": "invalid_sprint_task_id"})
		return
	}

	uidVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "code": "unauthorized"})
		return
	}
	userID, _ := uidVal.(string)

	view, err := h.svc.DeleteSprintTask(c.Request.Context(), groupID, userID, taskID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrGroupNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "group not found", "code": "group_not_found"})
		case errors.Is(err, service.ErrSprintTaskNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "sprint task not found", "code": "sprint_task_not_found"})
		case errors.Is(err, service.ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "you must be a member of the group", "code": "forbidden"})
		case errors.Is(err, service.ErrInvalidGroupID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_group_id"})
		case errors.Is(err, service.ErrInvalidSprintTaskID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_sprint_task_id"})
		default:
			log.Printf("DeleteSprintTask internal error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete sprint task", "code": "internal"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "sprint task deleted successfully",
		"data":    sprintTaskToJSON(view),
	})
}

func sprintTaskToJSON(view service.SprintTaskView) gin.H {
	return gin.H{
		"id":              uuidToString(view.Task.ID),
		"group_id":        uuidToString(view.Sheet.GroupID),
		"sheet_id":        uuidToString(view.Task.SheetID),
		"sheet_name":      view.Sheet.Name,
		"title":           view.Task.Title,
		"assigned_to":     uuidToString(view.Task.AssigneeUserID),
		"priority":        view.Task.Priority,
		"status":          view.Task.Status,
		"estimated_hours": numericToFloat(view.Task.EstimatedHours),
		"created_at":      timeToString(view.Task.CreatedAt),
		"updated_at":      timeToString(view.Task.UpdatedAt),
	}
}

func numericToFloat(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return 0
	}
	return f.Float64
}
