package http

import (
	"strings"
	"net/http"
	"log"
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

// GET /groups/{id}
func (h *GroupHandler) Get(c *gin.Context) {
	// Verificar que el usuario tenga un JWT válido
	_, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "No autorizado"})
		return
	}

	id := c.Param("id")
	group, err := h.svc.GetGroup(c.Request.Context(), id)
	
	if err != nil {
		if err == service.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Grupo no encontrado"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error interno del servidor"})
		return
	}

	c.JSON(http.StatusOK, group)
}



// GET /groups/me
func (h *GroupHandler) ListMy(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "No autorizado"})
		return
	}

	groups, err := h.svc.ListMyGroups(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error interno del servidor"})
		return
	}

	c.JSON(http.StatusOK, groups)
}

func handleServiceError(c *gin.Context, err error) {
	if se, ok := err.(*service.ServiceError); ok {
		status := utils.StatusForCode(se.Code)
		utils.RespondError(c, status, se.Code, se.Message)
		return
	}
	utils.RespondError(c, http.StatusInternalServerError, utils.ErrInternal, "Error interno")
}

type joinGroupRequest struct {
    InviteToken string `json:"invite_token" binding:"required"`
}

// Join maneja POST /groups/{id}/join
func (h *GroupHandler) Join(c *gin.Context) {
    userID, ok := middleware.GetUserID(c)
    if !ok {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "No autorizado"})
        return
    }

    groupID := c.Param("id")
    var req joinGroupRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invite_token requerido"})
        return
    }

    err := h.svc.JoinGroup(c.Request.Context(), groupID, req.InviteToken, userID)
    if err != nil {
        switch err {
        case service.ErrBanned:
            c.JSON(http.StatusForbidden, gin.H{"error": "Usuario baneado del grupo"})
        case service.ErrInvalidInviteToken:
            c.JSON(http.StatusNotFound, gin.H{"error": "Token de invitación inválido o grupo inexistente"})
        default:
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Error interno del servidor"})
        }
        return
    }

    // Contrato: 201 Created (sin body o con éxito)
    c.Status(http.StatusCreated)
}
// RegenerateInvite maneja POST /groups/{id}/invite/regenerate
func (h *GroupHandler) RegenerateInvite(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "No autorizado"})
		return
	}

	groupID := c.Param("id")
	newToken, err := h.svc.RegenerateInvite(c.Request.Context(), groupID, userID)
	if err != nil {
		switch err {
		case service.ErrForbidden:
			c.JSON(http.StatusForbidden, gin.H{"error": "Acceso denegado: solo los administradores pueden regenerar el token"})
		case service.ErrNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "Grupo no encontrado"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error interno del servidor"})
		}
		return
	}

	// Contrato: 200 {new_invite_token}
	c.JSON(http.StatusOK, gin.H{"new_invite_token": newToken})
}

func (h *GroupHandler) ListMembers(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "No autorizado"})
		return
	}

	groupID := c.Param("id")
	// c.Request.Context() provee el context.Context que pedía el error
	members, err := h.svc.ListMembers(c.Request.Context(), groupID, userID)
	if err != nil {
		switch err {
		case service.ErrForbidden:
			c.JSON(http.StatusForbidden, gin.H{"error": "Acceso denegado: debes ser miembro del grupo para ver los integrantes"})
		case service.ErrNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "Grupo no encontrado"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error interno del servidor"})
		}
		return
	}

	c.JSON(http.StatusOK, members)
}

// KickMember maneja POST /groups/{id}/members/{user_id}/kick
func (h *GroupHandler) KickMember(c *gin.Context) {
	adminID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "No autorizado"})
		return
	}

	groupID := c.Param("id")
	targetUserID := c.Param("user_id")

	err := h.svc.KickMember(c.Request.Context(), groupID, adminID, targetUserID)
	if err != nil {
		switch err {
		case service.ErrForbidden:
			c.JSON(http.StatusForbidden, gin.H{"error": "Acceso denegado: solo admins pueden expulsar"})
		case service.ErrCannotModifySelf:
			c.JSON(http.StatusBadRequest, gin.H{"error": "No puedes expulsarte a ti mismo"})
		case service.ErrTargetIsAdminOrNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "Usuario no encontrado o es administrador"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error interno del servidor"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Usuario expulsado exitosamente"})
}

// BanMember maneja POST /groups/{id}/members/{user_id}/ban
func (h *GroupHandler) BanMember(c *gin.Context) {
	adminID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "No autorizado"})
		return
	}

	groupID := c.Param("id")
	targetUserID := c.Param("user_id")

	err := h.svc.BanMember(c.Request.Context(), groupID, adminID, targetUserID)
	if err != nil {
		switch err {
		case service.ErrForbidden:
			c.JSON(http.StatusForbidden, gin.H{"error": "Acceso denegado: solo admins pueden banear"})
		case service.ErrCannotModifySelf:
			c.JSON(http.StatusBadRequest, gin.H{"error": "No puedes banearte a ti mismo"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error interno del servidor"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Usuario baneado exitosamente"})
}

type changeRoleRequest struct {
	Role string `json:"role" binding:"required"`
}

// ChangeRole maneja PATCH /groups/{id}/members/{user_id}/role
func (h *GroupHandler) ChangeRole(c *gin.Context) {
	adminID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "No autorizado"})
		return
	}

	groupID := c.Param("id")
	targetUserID := c.Param("user_id")
	if targetUserID == "" {
		targetUserID = c.Param("userId") // Atrapa el ID si en main.go está sin guion bajo
	}
	var req changeRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Campo 'role' requerido ('admin' o 'member')"})
		return
	}

	err := h.svc.ChangeRole(c.Request.Context(), groupID, adminID, targetUserID, req.Role)
	if err != nil {
		switch err {
		case service.ErrForbidden:
			c.JSON(http.StatusForbidden, gin.H{"error": "Acceso denegado: solo admins pueden cambiar roles"})
		case service.ErrCannotModifySelf:
			c.JSON(http.StatusBadRequest, gin.H{"error": "No puedes cambiar tu propio rol por esta vía"})
		case service.ErrInvalidRole:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Rol inválido. Usa 'admin' o 'member'"})
		case service.ErrTargetNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "Usuario objetivo no encontrado en el grupo"})
		default:
			log.Printf("ERROR REAL EN CHANGEROLE: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error interno del servidor"})
		}
		return
	}
	c.Status(http.StatusOK)
}

type transferAdminRequest struct {
	NewAdminUserID string `json:"new_admin_user_id" binding:"required"`
}

// TransferAdmin maneja POST /groups/{id}/transfer-admin
func (h *GroupHandler) TransferAdmin(c *gin.Context) {
	adminID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "No autorizado"})
		return
	}

	groupID := c.Param("id")

	var req transferAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Campo 'new_admin_user_id' requerido"})
		return
	}

	err := h.svc.TransferAdmin(c.Request.Context(), groupID, adminID, req.NewAdminUserID)
	if err != nil {
		switch err {
		case service.ErrForbidden:
			c.JSON(http.StatusForbidden, gin.H{"error": "Acceso denegado: solo admins pueden transferir la administración"})
		case service.ErrCannotModifySelf:
			c.JSON(http.StatusBadRequest, gin.H{"error": "No puedes transferir la administración a ti mismo"})
		case service.ErrTargetNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "El usuario destino no es miembro del grupo"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error interno del servidor"})
		}
		return
	}
	c.Status(http.StatusOK)
}

type leaveGroupRequest struct {
	CleanupSharedNotes bool `json:"cleanup_shared_notes"`
}

// LeaveGroup maneja POST /groups/{id}/leave
func (h *GroupHandler) LeaveGroup(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "No autorizado"})
		return
	}

	groupID := c.Param("id")

	var req leaveGroupRequest
	// Ignoramos el error de binding porque el body puede venir completamente vacío
	// y CleanupSharedNotes tomará el valor false por defecto.
	_ = c.ShouldBindJSON(&req)

	// Extraer el token crudo para pasarlo al servicio (y que este llame a Notes)
	authHeader := c.GetHeader("Authorization")
	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))

	err := h.svc.LeaveGroup(c.Request.Context(), groupID, userID, req.CleanupSharedNotes, token)
	if err != nil {
		switch err {
		case service.ErrForbidden:
			c.JSON(http.StatusForbidden, gin.H{"error": "No eres miembro de este grupo"})
		case service.ErrCannotLeaveOnlyAdmin:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Eres el único administrador. Transfiere la administración antes de salir."})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error interno del servidor"})
		}
		return
	}

	// El contrato exige 204 No Content
	c.Status(http.StatusNoContent)
}