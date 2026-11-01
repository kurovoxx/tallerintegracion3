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
		fail(c, "CreateSprintTask", http.StatusBadRequest, "invalid_group_id", "group ID is required")
		return
	}
	if _, err := uuid.Parse(groupID); err != nil {
		fail(c, "CreateSprintTask", http.StatusBadRequest, "invalid_group_id", "invalid group ID format")
		return
	}

	var req CreateSprintTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "CreateSprintTask", http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if strings.TrimSpace(req.SheetID) != "" {
		if _, err := uuid.Parse(strings.TrimSpace(req.SheetID)); err != nil {
			fail(c, "CreateSprintTask", http.StatusBadRequest, "invalid_sheet_id", "invalid sheet_id format")
			return
		}
	}

	uidVal, exists := c.Get("user_id")
	if !exists {
		fail(c, "CreateSprintTask", http.StatusUnauthorized, "unauthorized", "unauthorized")
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
			fail(c, "CreateSprintTask", http.StatusNotFound, "group_not_found", "group not found")
		case errors.Is(err, service.ErrSheetNotFound):
			fail(c, "CreateSprintTask", http.StatusNotFound, "sheet_not_found", "sheet not found in group")
		case errors.Is(err, service.ErrForbidden):
			fail(c, "CreateSprintTask", http.StatusForbidden, "forbidden", "you must be a member of the group")
		case errors.Is(err, service.ErrInvalidGroupID):
			fail(c, "CreateSprintTask", http.StatusBadRequest, "invalid_group_id", err.Error())
		case errors.Is(err, service.ErrInvalidSheetID):
			fail(c, "CreateSprintTask", http.StatusBadRequest, "invalid_sheet_id", err.Error())
		case errors.Is(err, service.ErrInvalidUserID):
			fail(c, "CreateSprintTask", http.StatusBadRequest, "invalid_user_id", err.Error())
		case errors.Is(err, service.ErrInvalidTitle),
			errors.Is(err, service.ErrTitleTooLong),
			errors.Is(err, service.ErrInvalidPriority),
			errors.Is(err, service.ErrInvalidSprintStatus),
			errors.Is(err, service.ErrAssigneeRequired),
			errors.Is(err, service.ErrAssigneeNotMember),
			errors.Is(err, service.ErrInvalidEstimatedHours):
			fail(c, "CreateSprintTask", http.StatusBadRequest, "invalid_body", err.Error())
		default:
			log.Printf("CreateSprintTask internal error: %v", err)
			fail(c, "CreateSprintTask", http.StatusInternalServerError, "internal", "could not create sprint task")
		}
		return
	}

	logOp(c, "CreateSprintTask", http.StatusCreated, "task_id="+uuidToString(view.Task.ID))
	c.JSON(http.StatusCreated, gin.H{
		"message": "sprint task created successfully",
		"data":    sprintTaskToJSON(view),
	})
}

func (h *SprintHandler) ListSprintTasks(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("id"))
	if groupID == "" {
		fail(c, "ListSprintTasks", http.StatusBadRequest, "invalid_group_id", "group ID is required")
		return
	}
	if _, err := uuid.Parse(groupID); err != nil {
		fail(c, "ListSprintTasks", http.StatusBadRequest, "invalid_group_id", "invalid group ID format")
		return
	}

	uidVal, exists := c.Get("user_id")
	if !exists {
		fail(c, "ListSprintTasks", http.StatusUnauthorized, "unauthorized", "unauthorized")
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
			fail(c, "ListSprintTasks", http.StatusNotFound, "group_not_found", "group not found")
		case errors.Is(err, service.ErrSheetNotFound):
			fail(c, "ListSprintTasks", http.StatusNotFound, "sheet_not_found", "sheet not found in group")
		case errors.Is(err, service.ErrForbidden):
			fail(c, "ListSprintTasks", http.StatusForbidden, "forbidden", "you must be a member of the group")
		case errors.Is(err, service.ErrInvalidGroupID):
			fail(c, "ListSprintTasks", http.StatusBadRequest, "invalid_group_id", err.Error())
		case errors.Is(err, service.ErrInvalidSheetID):
			fail(c, "ListSprintTasks", http.StatusBadRequest, "invalid_sheet_id", err.Error())
		case errors.Is(err, service.ErrInvalidSprintStatus):
			fail(c, "ListSprintTasks", http.StatusBadRequest, "invalid_status", err.Error())
		case errors.Is(err, service.ErrInvalidPriority):
			fail(c, "ListSprintTasks", http.StatusBadRequest, "invalid_priority", err.Error())
		default:
			log.Printf("ListSprintTasks internal error: %v", err)
			fail(c, "ListSprintTasks", http.StatusInternalServerError, "internal", "could not list sprint tasks")
		}
		return
	}

	data := make([]gin.H, 0, len(views))
	for _, v := range views {
		data = append(data, sprintTaskToJSON(v))
	}
	logOp(c, "ListSprintTasks", http.StatusOK, "")
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
		fail(c, "UpdateSprintTask", http.StatusBadRequest, "invalid_group_id", "group ID is required")
		return
	}
	if _, err := uuid.Parse(groupID); err != nil {
		fail(c, "UpdateSprintTask", http.StatusBadRequest, "invalid_group_id", "invalid group ID format")
		return
	}
	taskID := strings.TrimSpace(c.Param("taskId"))
	if taskID == "" {
		fail(c, "UpdateSprintTask", http.StatusBadRequest, "invalid_sprint_task_id", "sprint task ID is required")
		return
	}
	if _, err := uuid.Parse(taskID); err != nil {
		fail(c, "UpdateSprintTask", http.StatusBadRequest, "invalid_sprint_task_id", "invalid sprint task ID format")
		return
	}

	var req UpdateSprintTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "UpdateSprintTask", http.StatusBadRequest, "invalid_body", err.Error())
		return
	}

	uidVal, exists := c.Get("user_id")
	if !exists {
		fail(c, "UpdateSprintTask", http.StatusUnauthorized, "unauthorized", "unauthorized")
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
			fail(c, "UpdateSprintTask", http.StatusNotFound, "group_not_found", "group not found")
		case errors.Is(err, service.ErrSprintTaskNotFound):
			fail(c, "UpdateSprintTask", http.StatusNotFound, "sprint_task_not_found", "sprint task not found")
		case errors.Is(err, service.ErrForbidden):
			fail(c, "UpdateSprintTask", http.StatusForbidden, "forbidden", "you must be a member of the group")
		case errors.Is(err, service.ErrInvalidGroupID):
			fail(c, "UpdateSprintTask", http.StatusBadRequest, "invalid_group_id", err.Error())
		case errors.Is(err, service.ErrInvalidSprintTaskID):
			fail(c, "UpdateSprintTask", http.StatusBadRequest, "invalid_sprint_task_id", err.Error())
		case errors.Is(err, service.ErrInvalidUserID):
			fail(c, "UpdateSprintTask", http.StatusBadRequest, "invalid_user_id", err.Error())
		case errors.Is(err, service.ErrInvalidTitle),
			errors.Is(err, service.ErrTitleTooLong),
			errors.Is(err, service.ErrInvalidPriority),
			errors.Is(err, service.ErrInvalidSprintStatus),
			errors.Is(err, service.ErrAssigneeRequired),
			errors.Is(err, service.ErrAssigneeNotMember),
			errors.Is(err, service.ErrInvalidEstimatedHours),
			errors.Is(err, service.ErrNothingToPatch):
			fail(c, "UpdateSprintTask", http.StatusBadRequest, "invalid_body", err.Error())
		default:
			log.Printf("UpdateSprintTask internal error: %v", err)
			fail(c, "UpdateSprintTask", http.StatusInternalServerError, "internal", "could not update sprint task")
		}
		return
	}

	logOp(c, "UpdateSprintTask", http.StatusOK, "task_id="+uuidToString(view.Task.ID))
	c.JSON(http.StatusOK, gin.H{
		"message": "sprint task updated successfully",
		"data":    sprintTaskToJSON(view),
	})
}

func (h *SprintHandler) DeleteSprintTask(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("id"))
	if groupID == "" {
		fail(c, "DeleteSprintTask", http.StatusBadRequest, "invalid_group_id", "group ID is required")
		return
	}
	if _, err := uuid.Parse(groupID); err != nil {
		fail(c, "DeleteSprintTask", http.StatusBadRequest, "invalid_group_id", "invalid group ID format")
		return
	}
	taskID := strings.TrimSpace(c.Param("taskId"))
	if taskID == "" {
		fail(c, "DeleteSprintTask", http.StatusBadRequest, "invalid_sprint_task_id", "sprint task ID is required")
		return
	}
	if _, err := uuid.Parse(taskID); err != nil {
		fail(c, "DeleteSprintTask", http.StatusBadRequest, "invalid_sprint_task_id", "invalid sprint task ID format")
		return
	}

	uidVal, exists := c.Get("user_id")
	if !exists {
		fail(c, "DeleteSprintTask", http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	userID, _ := uidVal.(string)

	view, err := h.svc.DeleteSprintTask(c.Request.Context(), groupID, userID, taskID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrGroupNotFound):
			fail(c, "DeleteSprintTask", http.StatusNotFound, "group_not_found", "group not found")
		case errors.Is(err, service.ErrSprintTaskNotFound):
			fail(c, "DeleteSprintTask", http.StatusNotFound, "sprint_task_not_found", "sprint task not found")
		case errors.Is(err, service.ErrForbidden):
			fail(c, "DeleteSprintTask", http.StatusForbidden, "forbidden", "you must be a member of the group")
		case errors.Is(err, service.ErrInvalidGroupID):
			fail(c, "DeleteSprintTask", http.StatusBadRequest, "invalid_group_id", err.Error())
		case errors.Is(err, service.ErrInvalidSprintTaskID):
			fail(c, "DeleteSprintTask", http.StatusBadRequest, "invalid_sprint_task_id", err.Error())
		default:
			log.Printf("DeleteSprintTask internal error: %v", err)
			fail(c, "DeleteSprintTask", http.StatusInternalServerError, "internal", "could not delete sprint task")
		}
		return
	}

	logOp(c, "DeleteSprintTask", http.StatusOK, "task_id="+uuidToString(view.Task.ID))
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
