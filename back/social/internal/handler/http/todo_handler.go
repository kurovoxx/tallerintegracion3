package http

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

type TodoHandler struct {
	svc *service.TodoService
}

func NewTodoHandler(svc *service.TodoService) *TodoHandler {
	return &TodoHandler{
		svc: svc,
	}
}

type CreateTodoRequest struct {
	Title      string `json:"title" binding:"required,max=300"`
	BoardID    string `json:"board_id"`
	Status     string `json:"status" binding:"omitempty,oneof=todo in_progress done"`
	AssignedTo string `json:"assigned_to"`
	DueDate    string `json:"due_date"`
}

func (h *TodoHandler) CreateTodo(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("id"))
	if groupID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group ID is required", "code": "invalid_group_id"})
		return
	}
	if _, err := uuid.Parse(groupID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID format", "code": "invalid_group_id"})
		return
	}

	var req CreateTodoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_body"})
		return
	}
	if strings.TrimSpace(req.BoardID) != "" {
		if _, err := uuid.Parse(strings.TrimSpace(req.BoardID)); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid board_id format", "code": "invalid_board_id"})
			return
		}
	}
	if strings.TrimSpace(req.AssignedTo) != "" {
		if _, err := uuid.Parse(strings.TrimSpace(req.AssignedTo)); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid assigned_to user ID format", "code": "invalid_user_id"})
			return
		}
	}

	if _, exists := c.Get("user_id"); !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "code": "unauthorized"})
		return
	}

	view, err := h.svc.CreateTodo(
		c.Request.Context(),
		groupID,
		strings.TrimSpace(req.BoardID),
		req.Title,
		req.Status,
		strings.TrimSpace(req.AssignedTo),
		strings.TrimSpace(req.DueDate),
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrGroupNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "group not found", "code": "group_not_found"})
		case errors.Is(err, service.ErrBoardNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "board not found in group", "code": "board_not_found"})
		case errors.Is(err, service.ErrInvalidGroupID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_group_id"})
		case errors.Is(err, service.ErrInvalidBoardID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_board_id"})
		case errors.Is(err, service.ErrInvalidUserID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_user_id"})
		case errors.Is(err, service.ErrInvalidTitle),
			errors.Is(err, service.ErrTitleTooLong),
			errors.Is(err, service.ErrInvalidStatus),
			errors.Is(err, service.ErrInvalidDueDate):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_body"})
		default:
			log.Printf("CreateTodo internal error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create todo", "code": "internal"})
		}
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "todo created successfully",
		"data":    todoToJSON(view),
	})
}

func (h *TodoHandler) ListTodos(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("id"))
	if groupID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group ID is required", "code": "invalid_group_id"})
		return
	}
	if _, err := uuid.Parse(groupID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID format", "code": "invalid_group_id"})
		return
	}

	if _, exists := c.Get("user_id"); !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "code": "unauthorized"})
		return
	}

	views, err := h.svc.ListTodos(
		c.Request.Context(),
		groupID,
		strings.TrimSpace(c.Query("status")),
		strings.TrimSpace(c.Query("board_id")),
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrGroupNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "group not found", "code": "group_not_found"})
		case errors.Is(err, service.ErrBoardNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "board not found in group", "code": "board_not_found"})
		case errors.Is(err, service.ErrInvalidGroupID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_group_id"})
		case errors.Is(err, service.ErrInvalidBoardID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_board_id"})
		case errors.Is(err, service.ErrInvalidStatus):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_status"})
		default:
			log.Printf("ListTodos internal error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list todos", "code": "internal"})
		}
		return
	}

	data := make([]gin.H, 0, len(views))
	for _, v := range views {
		data = append(data, todoToJSON(v))
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

type UpdateTodoRequest struct {
	Title      *string `json:"title" binding:"omitempty,max=300"`
	Status     *string `json:"status" binding:"omitempty,oneof=todo in_progress done"`
	AssignedTo *string `json:"assigned_to"`
	DueDate    *string `json:"due_date"`
}

func (h *TodoHandler) UpdateTodo(c *gin.Context) {
	todoID := strings.TrimSpace(c.Param("id"))
	if todoID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "todo ID is required", "code": "invalid_todo_id"})
		return
	}
	if _, err := uuid.Parse(todoID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid todo ID format", "code": "invalid_todo_id"})
		return
	}

	var req UpdateTodoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_body"})
		return
	}

	if _, exists := c.Get("user_id"); !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "code": "unauthorized"})
		return
	}

	view, err := h.svc.UpdateTodo(
		c.Request.Context(),
		todoID,
		req.Title,
		req.Status,
		req.AssignedTo,
		req.DueDate,
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTodoNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "todo not found", "code": "todo_not_found"})
		case errors.Is(err, service.ErrInvalidTodoID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_todo_id"})
		case errors.Is(err, service.ErrInvalidUserID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_user_id"})
		case errors.Is(err, service.ErrInvalidTitle),
			errors.Is(err, service.ErrTitleTooLong),
			errors.Is(err, service.ErrInvalidStatus),
			errors.Is(err, service.ErrInvalidDueDate),
			errors.Is(err, service.ErrNothingToPatch):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_body"})
		default:
			log.Printf("UpdateTodo internal error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update todo", "code": "internal"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "todo updated successfully",
		"data":    todoToJSON(view),
	})
}

func (h *TodoHandler) DeleteTodo(c *gin.Context) {
	todoID := strings.TrimSpace(c.Param("id"))
	if todoID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "todo ID is required", "code": "invalid_todo_id"})
		return
	}
	if _, err := uuid.Parse(todoID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid todo ID format", "code": "invalid_todo_id"})
		return
	}

	if _, exists := c.Get("user_id"); !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "code": "unauthorized"})
		return
	}

	view, err := h.svc.DeleteTodo(c.Request.Context(), todoID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTodoNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "todo not found", "code": "todo_not_found"})
		case errors.Is(err, service.ErrInvalidTodoID):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_todo_id"})
		default:
			log.Printf("DeleteTodo internal error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete todo", "code": "internal"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "todo deleted successfully",
		"data":    todoToJSON(view),
	})
}

func todoToJSON(view service.TodoTaskView) gin.H {
	return gin.H{
		"id":          uuidToString(view.Task.ID),
		"group_id":    uuidToString(view.Board.GroupID),
		"board_id":    uuidToString(view.Task.BoardID),
		"board_name":  view.Board.Name,
		"title":       view.Task.Title,
		"status":      view.Task.Status,
		"assigned_to": uuidToNil(view.Task.AssigneeUserID),
		"due_date":    dateToNil(view.Task.DueDate),
		"created_at":  timeToString(view.Task.CreatedAt),
		"updated_at":  timeToString(view.Task.UpdatedAt),
	}
}

func uuidToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	id, err := uuid.FromBytes(u.Bytes[:])
	if err != nil {
		return ""
	}
	return id.String()
}

func uuidToNil(u pgtype.UUID) *string {
	if !u.Valid {
		return nil
	}
	id, err := uuid.FromBytes(u.Bytes[:])
	if err != nil {
		return nil
	}
	s := id.String()
	return &s
}

func dateToNil(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.Format("2006-01-02")
	return &s
}

func timeToString(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}
