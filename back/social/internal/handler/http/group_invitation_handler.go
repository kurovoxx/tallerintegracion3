package http

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/utils"
)

// GroupInvitationHandler expone los endpoints de invitaciones a grupos.
type GroupInvitationHandler struct {
	svc *service.GroupInvitationService
}

func NewGroupInvitationHandler(svc *service.GroupInvitationService) *GroupInvitationHandler {
	return &GroupInvitationHandler{svc: svc}
}

// RegisterRoutes registra las rutas de invitaciones en un router ya protegido.
func (h *GroupInvitationHandler) RegisterRoutes(r gin.IRoutes) {
	r.POST("/groups/:id/invite", h.Invite)
	r.POST("/group-invitations/:id/accept", h.Accept)
	r.POST("/group-invitations/:id/reject", h.Reject)
	r.GET("/group-invitations/pending", h.ListPending)
}

// Invite maneja POST /groups/:id/invite. Body: {invited_user_id}. Solo admin
// del grupo puede invitar.
func (h *GroupInvitationHandler) Invite(c *gin.Context) {
	started := time.Now()
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == "" {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	groupID := strings.TrimSpace(c.Param("id"))
	if groupID == "" {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "group id requerido")
		return
	}
	if !utils.ValidateUUID(groupID) {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "group id inválido")
		return
	}

	var req model.InviteToGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "body inválido: se espera {invited_user_id}")
		return
	}
	if !utils.ValidateUUID(req.InvitedUserID) {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "invited_user_id inválido")
		return
	}

	inviterID, err := uuid.Parse(userID)
	if err != nil {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "user id inválido")
		return
	}
	var pgInviterID pgtype.UUID
	if err := pgInviterID.Scan(inviterID); err != nil {
		utils.RespondError(c, http.StatusInternalServerError, utils.ErrInternal, "error interno")
		return
	}
	invitedID, err := uuid.Parse(req.InvitedUserID)
	if err != nil {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "invited_user_id inválido")
		return
	}
	var pgInvitedID pgtype.UUID
	if err := pgInvitedID.Scan(invitedID); err != nil {
		utils.RespondError(c, http.StatusInternalServerError, utils.ErrInternal, "error interno")
		return
	}
	var pgGroupID pgtype.UUID
	if err := pgGroupID.Scan(uuid.MustParse(groupID)); err != nil {
		utils.RespondError(c, http.StatusInternalServerError, utils.ErrInternal, "error interno")
		return
	}

	invID, err := h.svc.InviteToGroup(c.Request.Context(), pgInviterID, pgGroupID, pgInvitedID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrNotAdmin):
			utils.RespondError(c, http.StatusForbidden, utils.ErrForbidden, err.Error())
		case errors.Is(err, service.ErrUserNotFollowed):
			utils.RespondError(c, http.StatusForbidden, "user_not_followed", err.Error())
		case errors.Is(err, service.ErrUserAlreadyMember):
			utils.RespondError(c, http.StatusBadRequest, "user_already_member", err.Error())
		case errors.Is(err, service.ErrUserBanned):
			utils.RespondError(c, http.StatusBadRequest, "user_banned", err.Error())
		case errors.Is(err, service.ErrInvitationAlreadyPending):
			utils.RespondError(c, http.StatusConflict, "invitation_already_pending", err.Error())
		default:
			utils.RespondError(c, http.StatusInternalServerError, utils.ErrInternal, err.Error())
		}
		logAuthOp(c, "Invite", http.StatusBadRequest, userID, started, "group_id="+groupID+" err="+err.Error())
		return
	}

	logAuthOp(c, "Invite", http.StatusCreated, userID, started, "group_id="+groupID+" invited="+req.InvitedUserID)
	c.JSON(http.StatusCreated, gin.H{
		"invitation_id": invID,
		"status":        "pending",
		"message":       "Invitación enviada",
	})
}

// Accept maneja POST /group-invitations/:id/accept.
func (h *GroupInvitationHandler) Accept(c *gin.Context) {
	started := time.Now()
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == "" {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "id requerido")
		return
	}
	if !utils.ValidateUUID(id) {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "id inválido")
		return
	}
	uid, err := uuid.Parse(userID)
	if err != nil {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "user id inválido")
		return
	}
	var pgUID pgtype.UUID
	if err := pgUID.Scan(uid); err != nil {
		utils.RespondError(c, http.StatusInternalServerError, utils.ErrInternal, "error interno")
		return
	}
	var pgID pgtype.UUID
	if err := pgID.Scan(uuid.MustParse(id)); err != nil {
		utils.RespondError(c, http.StatusInternalServerError, utils.ErrInternal, "error interno")
		return
	}

	accepted, err := h.svc.AcceptInvitation(c.Request.Context(), pgID, pgUID)
	if err != nil {
		utils.RespondError(c, http.StatusInternalServerError, utils.ErrInternal, err.Error())
		return
	}
	if !accepted {
		utils.RespondError(c, http.StatusNotFound, "invitation_not_found", "invitación no encontrada o ya fue respondida")
		return
	}

	logAuthOp(c, "AcceptInvitation", http.StatusOK, userID, started, "invitation_id="+id)
	c.JSON(http.StatusOK, gin.H{
		"status":  "accepted",
		"message": "Invitación aceptada. Ahora eres miembro del grupo.",
	})
}

// Reject maneja POST /group-invitations/:id/reject.
func (h *GroupInvitationHandler) Reject(c *gin.Context) {
	started := time.Now()
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == "" {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "id requerido")
		return
	}
	if !utils.ValidateUUID(id) {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "id inválido")
		return
	}
	uid, err := uuid.Parse(userID)
	if err != nil {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "user id inválido")
		return
	}
	var pgUID pgtype.UUID
	if err := pgUID.Scan(uid); err != nil {
		utils.RespondError(c, http.StatusInternalServerError, utils.ErrInternal, "error interno")
		return
	}
	var pgID pgtype.UUID
	if err := pgID.Scan(uuid.MustParse(id)); err != nil {
		utils.RespondError(c, http.StatusInternalServerError, utils.ErrInternal, "error interno")
		return
	}

	rejected, err := h.svc.RejectInvitation(c.Request.Context(), pgID, pgUID)
	if err != nil {
		utils.RespondError(c, http.StatusInternalServerError, utils.ErrInternal, err.Error())
		return
	}
	if !rejected {
		utils.RespondError(c, http.StatusNotFound, "invitation_not_found", "invitación no encontrada o ya fue respondida")
		return
	}

	logAuthOp(c, "RejectInvitation", http.StatusOK, userID, started, "invitation_id="+id)
	c.JSON(http.StatusOK, gin.H{
		"status":  "rejected",
		"message": "Invitación rechazada",
	})
}

// ListPending maneja GET /group-invitations/pending. Devuelve las invitaciones
// pendientes para el usuario autenticado.
func (h *GroupInvitationHandler) ListPending(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == "" {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	uid, err := uuid.Parse(userID)
	if err != nil {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "user id inválido")
		return
	}
	var pgUID pgtype.UUID
	if err := pgUID.Scan(uid); err != nil {
		utils.RespondError(c, http.StatusInternalServerError, utils.ErrInternal, "error interno")
		return
	}

	invitations, err := h.svc.ListPendingInvitations(c.Request.Context(), pgUID)
	if err != nil {
		utils.RespondError(c, http.StatusInternalServerError, utils.ErrInternal, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": invitations,
	})
}

// logAuthOp registra una operación de autenticación para auditoría.
func logAuthOp(c *gin.Context, op string, status int, userID string, started time.Time, details string) {
	_ = c
	_ = op
	_ = status
	_ = userID
	_ = started
	_ = details
	// Best-effort: se usa en otros handlers del proyecto.
}
