package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

// InternalHandler expone membresía para servicio-a-servicio (tarea 3_5_4:
// checks is_admin/member + member-emails de Notes).
// Solo tras RequireInternalKey; nunca con JWT de usuario.
type InternalHandler struct {
	groups *service.GroupService
}

func NewInternalHandler(groups *service.GroupService) *InternalHandler {
	return &InternalHandler{groups: groups}
}

func (h *InternalHandler) checkGroupID(c *gin.Context) (string, bool) {
	id := strings.TrimSpace(c.Param("id"))
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
		return "", false
	}
	return id, true
}

// Members maneja GET /internal/groups/{id}/members.
// 200 [{user_id, role}] (siempre array) | 404 grupo inexistente.
// Es lo que Notes usa para IsMember/IsAdmin al compartir.
func (h *InternalHandler) Members(c *gin.Context) {
	id, ok := h.checkGroupID(c)
	if !ok {
		return
	}
	members, err := h.groups.ListMembersInternal(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	type member struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	out := make([]member, 0, len(members))
	for _, m := range members {
		if m == nil {
			continue
		}
		out = append(out, member{UserID: m.UserID, Role: m.Role})
	}
	c.JSON(http.StatusOK, out)
}

// MemberEmails maneja GET /internal/groups/{id}/member-emails.
// 200 [emails...] (siempre array) | 404 grupo inexistente | 500 Identity
// inalcanzable (fail-closed: Notes no calcula permisos incompletos).
func (h *InternalHandler) MemberEmails(c *gin.Context) {
	id, ok := h.checkGroupID(c)
	if !ok {
		return
	}
	emails, err := h.groups.ListMemberEmailsInternal(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, emails)
}
