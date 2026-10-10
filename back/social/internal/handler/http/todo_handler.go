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
		fail(c, "CreateTodo", http.StatusBadRequest, "invalid_group_id", "group ID is required")
		return
	}
	if _, err := uuid.Parse(groupID); err != nil {
		fail(c, "CreateTodo", http.StatusBadRequest, "invalid_group_id", "invalid group ID format")
		return
	}

	var req CreateTodoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "CreateTodo", http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if strings.TrimSpace(req.BoardID) != "" {
		if _, err := uuid.Parse(strings.TrimSpace(req.BoardID)); err != nil {
			fail(c, "CreateTodo", http.StatusBadRequest, "invalid_board_id", "invalid board_id format")
			return
		}
	}
	if strings.TrimSpace(req.AssignedTo) != "" {
		if _, err := uuid.Parse(strings.TrimSpace(req.AssignedTo)); err != nil {
			fail(c, "CreateTodo", http.StatusBadRequest, "invalid_user_id", "invalid assigned_to user ID format")
			return
		}
	}

	uidVal, exists := c.Get("user_id")
	if !exists {
		fail(c, "CreateTodo", http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	userID, _ := uidVal.(string)

	view, err := h.svc.CreateTodo(
		c.Request.Context(),
		groupID,
		userID,
		strings.TrimSpace(req.BoardID),
		req.Title,
		req.Status,
		strings.TrimSpace(req.AssignedTo),
		strings.TrimSpace(req.DueDate),
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrGroupNotFound):
			fail(c, "CreateTodo", http.StatusNotFound, "group_not_found", "group not found")
		case errors.Is(err, service.ErrBoardNotFound):
			fail(c, "CreateTodo", http.StatusNotFound, "board_not_found", "board not found in group")
		case errors.Is(err, service.ErrForbidden):
			fail(c, "CreateTodo", http.StatusForbidden, "forbidden", "you must be a member of the group")
		case errors.Is(err, service.ErrInvalidGroupID):
			fail(c, "CreateTodo", http.StatusBadRequest, "invalid_group_id", err.Error())
		case errors.Is(err, service.ErrInvalidBoardID):
			fail(c, "CreateTodo", http.StatusBadRequest, "invalid_board_id", err.Error())
		case errors.Is(err, service.ErrInvalidUserID):
			fail(c, "CreateTodo", http.StatusBadRequest, "invalid_user_id", err.Error())
		case errors.Is(err, service.ErrInvalidTitle),
			errors.Is(err, service.ErrTitleTooLong),
			errors.Is(err, service.ErrInvalidStatus),
			errors.Is(err, service.ErrInvalidDueDate),
			errors.Is(err, service.ErrAssigneeNotMember):
			fail(c, "CreateTodo", http.StatusBadRequest, "invalid_body", err.Error())
		default:
			log.Printf("CreateTodo internal error: %v", err)
			fail(c, "CreateTodo", http.StatusInternalServerError, "internal", "could not create todo")
		}
		return
	}

	logOp(c, "CreateTodo", http.StatusCreated, "task_id="+uuidToString(view.Task.ID))
	c.JSON(http.StatusCreated, gin.H{
		"message": "todo created successfully",
		"data":    todoToJSON(view),
	})
}

func (h *TodoHandler) ListTodos(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("id"))
	if groupID == "" {
		fail(c, "ListTodos", http.StatusBadRequest, "invalid_group_id", "group ID is required")
		return
	}
	if _, err := uuid.Parse(groupID); err != nil {
		fail(c, "ListTodos", http.StatusBadRequest, "invalid_group_id", "invalid group ID format")
		return
	}

	uidVal, exists := c.Get("user_id")
	if !exists {
		fail(c, "ListTodos", http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	userID, _ := uidVal.(string)

	views, err := h.svc.ListTodos(
		c.Request.Context(),
		groupID,
		userID,
		strings.TrimSpace(c.Query("status")),
		strings.TrimSpace(c.Query("board_id")),
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrGroupNotFound):
			fail(c, "ListTodos", http.StatusNotFound, "group_not_found", "group not found")
		case errors.Is(err, service.ErrBoardNotFound):
			fail(c, "ListTodos", http.StatusNotFound, "board_not_found", "board not found in group")
		case errors.Is(err, service.ErrForbidden):
			fail(c, "ListTodos", http.StatusForbidden, "forbidden", "you must be a member of the group")
		case errors.Is(err, service.ErrInvalidGroupID):
			fail(c, "ListTodos", http.StatusBadRequest, "invalid_group_id", err.Error())
		case errors.Is(err, service.ErrInvalidBoardID):
			fail(c, "ListTodos", http.StatusBadRequest, "invalid_board_id", err.Error())
		case errors.Is(err, service.ErrInvalidStatus):
			fail(c, "ListTodos", http.StatusBadRequest, "invalid_status", err.Error())
		default:
			log.Printf("ListTodos internal error: %v", err)
			fail(c, "ListTodos", http.StatusInternalServerError, "internal", "could not list todos")
		}
		return
	}

	data := make([]gin.H, 0, len(views))
	for _, v := range views {
		data = append(data, todoToJSON(v))
	}
	logOp(c, "ListTodos", http.StatusOK, "")
	c.JSON(http.StatusOK, gin.H{"data": data})
}

type UpdateTodoRequest struct {
	Title      *string `json:"title" binding:"omitempty,max=300"`
	Status     *string `json:"status" binding:"omitempty,oneof=todo in_progress done"`
	AssignedTo *string `json:"assigned_to"`
	DueDate    *string `json:"due_date"`
}

func (h *TodoHandler) UpdateTodo(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("id"))
	if groupID == "" {
		fail(c, "UpdateTodo", http.StatusBadRequest, "invalid_group_id", "group ID is required")
		return
	}
	if _, err := uuid.Parse(groupID); err != nil {
		fail(c, "UpdateTodo", http.StatusBadRequest, "invalid_group_id", "invalid group ID format")
		return
	}
	todoID := strings.TrimSpace(c.Param("taskId"))
	if todoID == "" {
		fail(c, "UpdateTodo", http.StatusBadRequest, "invalid_todo_id", "todo ID is required")
		return
	}
	if _, err := uuid.Parse(todoID); err != nil {
		fail(c, "UpdateTodo", http.StatusBadRequest, "invalid_todo_id", "invalid todo ID format")
		return
	}

	var req UpdateTodoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "UpdateTodo", http.StatusBadRequest, "invalid_body", err.Error())
		return
	}

	uidVal, exists := c.Get("user_id")
	if !exists {
		fail(c, "UpdateTodo", http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	userID, _ := uidVal.(string)

	view, err := h.svc.UpdateTodo(
		c.Request.Context(),
		groupID,
		userID,
		todoID,
		req.Title,
		req.Status,
		req.AssignedTo,
		req.DueDate,
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrGroupNotFound):
			fail(c, "UpdateTodo", http.StatusNotFound, "group_not_found", "group not found")
		case errors.Is(err, service.ErrTodoNotFound):
			fail(c, "UpdateTodo", http.StatusNotFound, "todo_not_found", "todo not found")
		case errors.Is(err, service.ErrForbidden):
			fail(c, "UpdateTodo", http.StatusForbidden, "forbidden", "you must be a member of the group")
		case errors.Is(err, service.ErrInvalidGroupID):
			fail(c, "UpdateTodo", http.StatusBadRequest, "invalid_group_id", err.Error())
		case errors.Is(err, service.ErrInvalidTodoID):
			fail(c, "UpdateTodo", http.StatusBadRequest, "invalid_todo_id", err.Error())
		case errors.Is(err, service.ErrInvalidUserID):
			fail(c, "UpdateTodo", http.StatusBadRequest, "invalid_user_id", err.Error())
		case errors.Is(err, service.ErrInvalidTitle),
			errors.Is(err, service.ErrTitleTooLong),
			errors.Is(err, service.ErrInvalidStatus),
			errors.Is(err, service.ErrInvalidDueDate),
			errors.Is(err, service.ErrAssigneeNotMember),
			errors.Is(err, service.ErrNothingToPatch):
			fail(c, "UpdateTodo", http.StatusBadRequest, "invalid_body", err.Error())
		default:
			log.Printf("UpdateTodo internal error: %v", err)
			fail(c, "UpdateTodo", http.StatusInternalServerError, "internal", "could not update todo")
		}
		return
	}

	logOp(c, "UpdateTodo", http.StatusOK, "task_id="+uuidToString(view.Task.ID))
	c.JSON(http.StatusOK, gin.H{
		"message": "todo updated successfully",
		"data":    todoToJSON(view),
	})
}

func (h *TodoHandler) DeleteTodo(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("id"))
	if groupID == "" {
		fail(c, "DeleteTodo", http.StatusBadRequest, "invalid_group_id", "group ID is required")
		return
	}
	if _, err := uuid.Parse(groupID); err != nil {
		fail(c, "DeleteTodo", http.StatusBadRequest, "invalid_group_id", "invalid group ID format")
		return
	}
	todoID := strings.TrimSpace(c.Param("taskId"))
	if todoID == "" {
		fail(c, "DeleteTodo", http.StatusBadRequest, "invalid_todo_id", "todo ID is required")
		return
	}
	if _, err := uuid.Parse(todoID); err != nil {
		fail(c, "DeleteTodo", http.StatusBadRequest, "invalid_todo_id", "invalid todo ID format")
		return
	}

	uidVal, exists := c.Get("user_id")
	if !exists {
		fail(c, "DeleteTodo", http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	userID, _ := uidVal.(string)

	view, err := h.svc.DeleteTodo(c.Request.Context(), groupID, userID, todoID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrGroupNotFound):
			fail(c, "DeleteTodo", http.StatusNotFound, "group_not_found", "group not found")
		case errors.Is(err, service.ErrTodoNotFound):
			fail(c, "DeleteTodo", http.StatusNotFound, "todo_not_found", "todo not found")
		case errors.Is(err, service.ErrForbidden):
			fail(c, "DeleteTodo", http.StatusForbidden, "forbidden", "you must be a member of the group")
		case errors.Is(err, service.ErrInvalidGroupID):
			fail(c, "DeleteTodo", http.StatusBadRequest, "invalid_group_id", err.Error())
		case errors.Is(err, service.ErrInvalidTodoID):
			fail(c, "DeleteTodo", http.StatusBadRequest, "invalid_todo_id", err.Error())
		default:
			log.Printf("DeleteTodo internal error: %v", err)
			fail(c, "DeleteTodo", http.StatusInternalServerError, "internal", "could not delete todo")
		}
		return
	}

	logOp(c, "DeleteTodo", http.StatusOK, "task_id="+uuidToString(view.Task.ID))
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
