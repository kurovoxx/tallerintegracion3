package http

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/utils"
)

const notesErrorKey = "notes.operation.error"

// traceNotesOperation observes the existing response without writing to it.
func traceNotesOperation(c *gin.Context, route string) func() {
	started := time.Now()
	userID, _ := middleware.GetUserID(c)
	noteID := c.Param("id")
	if route == "/notes/shared/:id" || route == "/groups/:id/notes" {
		noteID = ""
	}
	attachmentID := c.Param("attachmentId")
	if attachmentID == "" {
		attachmentID = c.Param("attachment_id")
	}
	logger := slog.Default().With("method", c.Request.Method, "route", route,
		"user_id", userID, "note_id", noteID, "attachment_id", attachmentID)
	if route == "/notes/shared/:id" {
		sharedID := c.Param("sharedNoteId")
		if sharedID == "" {
			sharedID = c.Param("id")
		}
		logger = logger.With("shared_note_id", sharedID)
	}
	if route == "/groups/:id/notes" {
		logger = logger.With("group_id", c.Param("id"))
	}
	c.Request = c.Request.WithContext(utils.WithNotesLogger(c.Request.Context(), logger))
	logger.InfoContext(c.Request.Context(), fmt.Sprintf("[NOTES] INICIO %s %s | userID: %s | noteID: %s", c.Request.Method, route, userID, noteID))
	return func() {
		status := c.Writer.Status()
		attrs := []any{"status", status, "duration_ms", float64(time.Since(started).Microseconds()) / 1000}
		if status >= 400 {
			err, ok := c.Get(notesErrorKey)
			if !ok {
				err = status
			}
			logger.ErrorContext(c.Request.Context(), fmt.Sprintf("[NOTES] ERROR %s %s | error: %v", c.Request.Method, route, err), append(attrs, "error", err)...)
			return
		}
		logger.InfoContext(c.Request.Context(), fmt.Sprintf("[NOTES] OK %s %s | status: %d", c.Request.Method, route, status), attrs...)
	}
}

func respondNotesError(c *gin.Context, status int, code, message string) {
	if _, exists := c.Get(notesErrorKey); !exists {
		c.Set(notesErrorKey, fmt.Errorf("%s: %s", code, message))
	}
	utils.RespondError(c, status, code, message)
}
