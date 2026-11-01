package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
)

type AccountDeletionHandler struct {
	coordinator *service.AccountDeletionCoordinator
}

func NewAccountDeletionHandler(coordinator *service.AccountDeletionCoordinator) *AccountDeletionHandler {
	return &AccountDeletionHandler{coordinator: coordinator}
}

// Delete always derives the target exclusively from the authenticated JWT.
// 202 means durable cleanup is pending and can be queried/retried with the
// same still-valid JWT; a background worker also resumes without that JWT.
func (h *AccountDeletionHandler) Delete(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	err := h.coordinator.DeleteAccount(c.Request.Context(), userID)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"status": "complete"})
	case errors.Is(err, service.ErrAccountCleanupPending), errors.Is(err, service.ErrAccountCleanupFailed), errors.Is(err, service.ErrInvalidCleanupResult):
		c.JSON(http.StatusAccepted, gin.H{"status": "pending"})
	case errors.Is(err, repository.ErrAccountNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "account_not_found"})
	default:
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "account_deletion_unavailable"})
	}
}

func (h *AccountDeletionHandler) Status(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	state, exists, err := h.coordinator.State(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "account_status_unavailable"})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"status": "not_started"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": state.Status, "updated_at": state.UpdatedAt})
}
