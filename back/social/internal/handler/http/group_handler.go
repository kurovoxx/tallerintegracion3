package http

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/utils"
)

type GroupHandler struct {
	svc *service.GroupService
}

func NewGroupHandler(svc *service.GroupService) *GroupHandler {
	return &GroupHandler{svc: svc}
}

// RegisterRoutes registra las rutas de grupos en un router ya protegido por
// el middleware de autenticación. main.go y los tests comparten esta lista.
func (h *GroupHandler) RegisterRoutes(r gin.IRoutes) {
	r.POST("/groups", h.Create)
	r.GET("/groups/me", h.ListMy)
	r.GET("/groups/:id", h.Get)
	r.POST("/groups/:id/join", h.Join)
	r.POST("/groups/:id/invite/regenerate", h.RegenerateInvite)
	r.GET("/groups/:id/members", h.ListMembers)
	r.POST("/groups/:id/members/:user_id/kick", h.KickMember)
	r.POST("/groups/:id/members/:user_id/ban", h.BanMember)
	r.PATCH("/groups/:id/members/:user_id/role", h.ChangeRole)
	r.POST("/groups/:id/transfer-admin", h.TransferAdmin)
	r.POST("/groups/:id/leave", h.LeaveGroup)
	// Sucesión automática de admin al eliminar la cuenta (tarea 2_3_9)
	r.POST("/groups/account-deletion", h.AccountDeletion)
}

// respondGroupError traduce los errores de dominio de grupos a HTTP.
// forbiddenMsg personaliza el texto del 403 según la acción.
func respondGroupError(c *gin.Context, err error, forbiddenMsg string) {
	status, msg := http.StatusInternalServerError, "Error interno del servidor"
	switch {
	case errors.Is(err, service.ErrUnauthorized):
		status, msg = http.StatusUnauthorized, "No autorizado"
	case errors.Is(err, service.ErrForbidden):
		status, msg = http.StatusForbidden, forbiddenMsg
	case errors.Is(err, service.ErrBanned):
		status, msg = http.StatusForbidden, "Usuario baneado del grupo"
	case errors.Is(err, service.ErrNotFound):
		status, msg = http.StatusNotFound, "Grupo no encontrado"
	case errors.Is(err, service.ErrInvalidInviteToken):
		status, msg = http.StatusNotFound, "Token de invitación inválido o grupo inexistente"
	case errors.Is(err, service.ErrTargetNotFound):
		status, msg = http.StatusNotFound, "Usuario objetivo no encontrado en el grupo"
	case errors.Is(err, service.ErrTargetIsAdmin):
		status, msg = http.StatusBadRequest, "El usuario objetivo es administrador"
	case errors.Is(err, service.ErrCannotModifySelf):
		status, msg = http.StatusBadRequest, "No puedes aplicar esta acción sobre ti mismo"
	case errors.Is(err, service.ErrInvalidRole):
		status, msg = http.StatusBadRequest, "Rol inválido. Usa 'admin' o 'member'"
	case errors.Is(err, service.ErrCannotLeaveOnlyAdmin):
		status, msg = http.StatusBadRequest, "Eres el único administrador. Transfiere la administración antes de salir."
	default:
		log.Printf("social: error interno en grupos: %v", err)
	}
	c.JSON(status, gin.H{"error": msg})
}

// requireUser devuelve el user_id del JWT o responde 401.
func requireUser(c *gin.Context) (string, bool) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "No autorizado"})
	}
	return userID, ok
}

// POST /groups
// Body: {name, description?} — contrato: agentApiContract.md sección 4
type createGroupRequest struct {
	Name        string  `json:"name" binding:"required"`
	Description *string `json:"description"`
}

func (h *GroupHandler) Create(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	var req createGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "Request inválido: "+err.Error())
		return
	}
	group, err := h.svc.Create(c.Request.Context(), userID, req.Name, req.Description)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	// Contrato: 201 {group_id}
	c.JSON(http.StatusCreated, gin.H{"group_id": group.ID})
}

func handleServiceError(c *gin.Context, err error) {
	if se, ok := err.(*service.ServiceError); ok {
		status := utils.StatusForCode(se.Code)
		utils.RespondError(c, status, se.Code, se.Message)
		return
	}
	utils.RespondError(c, http.StatusInternalServerError, utils.ErrInternal, "Error interno")
}

// GET /groups/{id}
// Solo miembros; el invite_token se incluye únicamente para admins.
func (h *GroupHandler) Get(c *gin.Context) {
	userID, ok := requireUser(c)
	if !ok {
		return
	}
	group, err := h.svc.GetGroup(c.Request.Context(), c.Param("id"), userID)
	if err != nil {
		respondGroupError(c, err, "Acceso denegado: debes ser miembro del grupo")
		return
	}
	c.JSON(http.StatusOK, group)
}

// GET /groups/me
func (h *GroupHandler) ListMy(c *gin.Context) {
	userID, ok := requireUser(c)
	if !ok {
		return
	}
	groups, err := h.svc.ListMyGroups(c.Request.Context(), userID)
	if err != nil {
		respondGroupError(c, err, "")
		return
	}
	c.JSON(http.StatusOK, groups)
}

type joinGroupRequest struct {
	InviteToken string `json:"invite_token" binding:"required"`
}

// Join maneja POST /groups/{id}/join
func (h *GroupHandler) Join(c *gin.Context) {
	userID, ok := requireUser(c)
	if !ok {
		return
	}
	var req joinGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invite_token requerido"})
		return
	}
	if err := h.svc.JoinGroup(c.Request.Context(), c.Param("id"), req.InviteToken, userID); err != nil {
		respondGroupError(c, err, "Acceso denegado")
		return
	}
	// Contrato: 201 Created
	c.Status(http.StatusCreated)
}

// RegenerateInvite maneja POST /groups/{id}/invite/regenerate
func (h *GroupHandler) RegenerateInvite(c *gin.Context) {
	userID, ok := requireUser(c)
	if !ok {
		return
	}
	newToken, err := h.svc.RegenerateInvite(c.Request.Context(), c.Param("id"), userID)
	if err != nil {
		respondGroupError(c, err, "Acceso denegado: solo los administradores pueden regenerar el token")
		return
	}
	// Contrato: 200 {new_invite_token}
	c.JSON(http.StatusOK, gin.H{"new_invite_token": newToken})
}

// ListMembers maneja GET /groups/{id}/members
// Responde miembros con display_name/email cuando Identity los resuelve
// (campos omitidos si no, backward-compatible).
func (h *GroupHandler) ListMembers(c *gin.Context) {
	userID, ok := requireUser(c)
	if !ok {
		return
	}
	members, err := h.svc.ListMembersEnriched(c.Request.Context(), c.Param("id"), userID)
	if err != nil {
		respondGroupError(c, err, "Acceso denegado: debes ser miembro del grupo para ver los integrantes")
		return
	}
	c.JSON(http.StatusOK, members)
}

// KickMember maneja POST /groups/{id}/members/{user_id}/kick
func (h *GroupHandler) KickMember(c *gin.Context) {
	adminID, ok := requireUser(c)
	if !ok {
		return
	}
	if err := h.svc.KickMember(c.Request.Context(), c.Param("id"), adminID, c.Param("user_id")); err != nil {
		respondGroupError(c, err, "Acceso denegado: solo admins pueden expulsar")
		return
	}
	// Contrato: 204
	c.Status(http.StatusNoContent)
}

// BanMember maneja POST /groups/{id}/members/{user_id}/ban
func (h *GroupHandler) BanMember(c *gin.Context) {
	adminID, ok := requireUser(c)
	if !ok {
		return
	}
	if err := h.svc.BanMember(c.Request.Context(), c.Param("id"), adminID, c.Param("user_id")); err != nil {
		respondGroupError(c, err, "Acceso denegado: solo admins pueden banear")
		return
	}
	// Contrato: 204
	c.Status(http.StatusNoContent)
}

type changeRoleRequest struct {
	Role string `json:"role" binding:"required"`
}

// ChangeRole maneja PATCH /groups/{id}/members/{user_id}/role
func (h *GroupHandler) ChangeRole(c *gin.Context) {
	adminID, ok := requireUser(c)
	if !ok {
		return
	}
	var req changeRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Campo 'role' requerido ('admin' o 'member')"})
		return
	}
	if err := h.svc.ChangeRole(c.Request.Context(), c.Param("id"), adminID, c.Param("user_id"), req.Role); err != nil {
		respondGroupError(c, err, "Acceso denegado: solo admins pueden cambiar roles")
		return
	}
	c.Status(http.StatusOK)
}

type transferAdminRequest struct {
	NewAdminUserID string `json:"new_admin_user_id" binding:"required"`
}

// TransferAdmin maneja POST /groups/{id}/transfer-admin
func (h *GroupHandler) TransferAdmin(c *gin.Context) {
	adminID, ok := requireUser(c)
	if !ok {
		return
	}
	var req transferAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Campo 'new_admin_user_id' requerido"})
		return
	}
	if err := h.svc.TransferAdmin(c.Request.Context(), c.Param("id"), adminID, req.NewAdminUserID); err != nil {
		respondGroupError(c, err, "Acceso denegado: solo admins pueden transferir la administración")
		return
	}
	c.Status(http.StatusOK)
}

type leaveGroupRequest struct {
	CleanupSharedNotes bool `json:"cleanup_shared_notes"`
}

// LeaveGroup maneja POST /groups/{id}/leave
func (h *GroupHandler) LeaveGroup(c *gin.Context) {
	userID, ok := requireUser(c)
	if !ok {
		return
	}

	var req leaveGroupRequest
	// Se ignora el error de binding: el body puede venir vacío y
	// CleanupSharedNotes queda en false.
	_ = c.ShouldBindJSON(&req)

	// Token crudo para que el servicio lo reenvíe a Notes
	token := strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))

	if err := h.svc.LeaveGroup(c.Request.Context(), c.Param("id"), userID, req.CleanupSharedNotes, token); err != nil {
		respondGroupError(c, err, "No eres miembro de este grupo")
		return
	}
	// Contrato: 204
	c.Status(http.StatusNoContent)
}

// AccountDeletion maneja POST /groups/account-deletion (tarea 2_3_9).
// Lo invoca el flujo de eliminación de cuenta con el JWT del propio usuario:
// aplica la sucesión automática de admin en todos sus grupos.
func (h *GroupHandler) AccountDeletion(c *gin.Context) {
	userID, ok := requireUser(c)
	if !ok {
		return
	}
	if _, err := h.svc.HandleAccountDeletion(c.Request.Context(), userID); err != nil {
		respondGroupError(c, err, "")
		return
	}
	c.Status(http.StatusNoContent)
}
