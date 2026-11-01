package http

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/utils"
)

// CurriculaHandler maneja /curricula (todo filtrado por user_id del token).
type CurriculaHandler struct {
	svc *service.CurriculumService
}

func NewCurriculaHandler(svc *service.CurriculumService) *CurriculaHandler {
	return &CurriculaHandler{svc: svc}
}

type curriculumRequest struct {
	Name string `json:"name"`
}

func respondServiceError(c *gin.Context, op, userID string, started time.Time, err error) {
	var se *service.ServiceError
	if errors.As(err, &se) {
		status := utils.StatusForCode(se.Code)
		utils.RespondError(c, status, se.Code, se.Message)
		logAcademicOp(c, op, status, userID, started, "code="+se.Code)
		return
	}
	utils.RespondError(c, http.StatusInternalServerError, utils.ErrInternal, "Error interno")
	logAcademicOp(c, op, http.StatusInternalServerError, userID, started, "code="+utils.ErrInternal)
}

func requireUserID(c *gin.Context, op string, started time.Time) (string, bool) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == "" {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		logAcademicOp(c, op, http.StatusUnauthorized, "", started, "code="+utils.ErrUnauthorized)
		return "", false
	}
	return userID, true
}

// Create maneja POST /curricula → 201 {id}.
func (h *CurriculaHandler) Create(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "CreateCurriculum", started)
	if !ok {
		return
	}
	var req curriculumRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "Request inválido")
		logAcademicOp(c, "CreateCurriculum", http.StatusBadRequest, userID, started, "code="+utils.ErrBadRequest)
		return
	}
	cur, err := h.svc.Create(c.Request.Context(), userID, req.Name)
	if err != nil {
		respondServiceError(c, "CreateCurriculum", userID, started, err)
		return
	}
	logAcademicOp(c, "CreateCurriculum", http.StatusCreated, userID, started, "")
	c.JSON(http.StatusCreated, gin.H{"id": cur.ID})
}

// List maneja GET /curricula → 200 {items: [...]} (solo propias, nunca null).
func (h *CurriculaHandler) List(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "ListCurricula", started)
	if !ok {
		return
	}
	out, err := h.svc.List(c.Request.Context(), userID)
	if err != nil {
		respondServiceError(c, "ListCurricula", userID, started, err)
		return
	}
	logAcademicOp(c, "ListCurricula", http.StatusOK, userID, started, "")
	c.JSON(http.StatusOK, gin.H{"items": out})
}

// Get maneja GET /curricula/:id → 200 recurso; 404 si no existe o es ajeno.
func (h *CurriculaHandler) Get(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "GetCurriculum", started)
	if !ok {
		return
	}
	cur, err := h.svc.Get(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		respondServiceError(c, "GetCurriculum", userID, started, err)
		return
	}
	logAcademicOp(c, "GetCurriculum", http.StatusOK, userID, started, "")
	c.JSON(http.StatusOK, cur)
}

// Update maneja PATCH /curricula/:id → 200 recurso actualizado.
func (h *CurriculaHandler) Update(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "UpdateCurriculum", started)
	if !ok {
		return
	}
	var req curriculumRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "Request inválido")
		logAcademicOp(c, "UpdateCurriculum", http.StatusBadRequest, userID, started, "code="+utils.ErrBadRequest)
		return
	}
	cur, err := h.svc.Update(c.Request.Context(), userID, c.Param("id"), req.Name)
	if err != nil {
		respondServiceError(c, "UpdateCurriculum", userID, started, err)
		return
	}
	logAcademicOp(c, "UpdateCurriculum", http.StatusOK, userID, started, "")
	c.JSON(http.StatusOK, cur)
}

// Delete maneja DELETE /curricula/:id → 204 (cascada por FK).
func (h *CurriculaHandler) Delete(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "DeleteCurriculum", started)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), userID, c.Param("id")); err != nil {
		respondServiceError(c, "DeleteCurriculum", userID, started, err)
		return
	}
	logAcademicOp(c, "DeleteCurriculum", http.StatusNoContent, userID, started, "")
	c.AbortWithStatus(http.StatusNoContent)
}
