package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/utils"
)

// SubjectsHandler maneja subjects anidados y directos.
type SubjectsHandler struct {
	svc *service.SubjectService
}

func NewSubjectsHandler(svc *service.SubjectService) *SubjectsHandler {
	return &SubjectsHandler{svc: svc}
}

type subjectRequest struct {
	Name                 *string  `json:"name"`
	Semester             *int     `json:"semester"`
	Credits              *int     `json:"credits"`
	TotalClasses         *int     `json:"total_classes"`
	MinAttendancePct     *float64 `json:"min_attendance_pct"`
	AverageDecimalPlaces *int     `json:"average_decimal_places"`
	Status               *string  `json:"status"`
}

func toSubjectInput(req subjectRequest) service.SubjectInput {
	return service.SubjectInput{
		Name:                 req.Name,
		Semester:             req.Semester,
		Credits:              req.Credits,
		TotalClasses:         req.TotalClasses,
		MinAttendancePct:     req.MinAttendancePct,
		AverageDecimalPlaces: req.AverageDecimalPlaces,
		Status:               req.Status,
	}
}

// Create maneja POST /curricula/:id/subjects → 201 {id}.
// 404 si la malla no existe o no es propia.
func (h *SubjectsHandler) Create(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "CreateSubject", started)
	if !ok {
		return
	}
	var req subjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "Request inválido")
		logAcademicOp(c, "CreateSubject", http.StatusBadRequest, userID, started, "code="+utils.ErrBadRequest)
		return
	}
	sub, err := h.svc.Create(c.Request.Context(), userID, c.Param("id"), toSubjectInput(req))
	if err != nil {
		respondServiceError(c, "CreateSubject", userID, started, err)
		return
	}
	logAcademicOp(c, "CreateSubject", http.StatusCreated, userID, started, "")
	c.JSON(http.StatusCreated, gin.H{"id": sub.ID})
}

// ListByCurriculum maneja GET /curricula/:id/subjects → 200 {items}.
func (h *SubjectsHandler) ListByCurriculum(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "ListSubjects", started)
	if !ok {
		return
	}
	out, err := h.svc.List(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		respondServiceError(c, "ListSubjects", userID, started, err)
		return
	}
	logAcademicOp(c, "ListSubjects", http.StatusOK, userID, started, "")
	c.JSON(http.StatusOK, gin.H{"items": out})
}

// Get maneja GET /subjects/:id → 200 recurso.
func (h *SubjectsHandler) Get(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "GetSubject", started)
	if !ok {
		return
	}
	sub, err := h.svc.Get(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		respondServiceError(c, "GetSubject", userID, started, err)
		return
	}
	logAcademicOp(c, "GetSubject", http.StatusOK, userID, started, "")
	c.JSON(http.StatusOK, sub)
}

// Update maneja PATCH /subjects/:id → 200 recurso actualizado.
func (h *SubjectsHandler) Update(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "UpdateSubject", started)
	if !ok {
		return
	}
	var req subjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "Request inválido")
		logAcademicOp(c, "UpdateSubject", http.StatusBadRequest, userID, started, "code="+utils.ErrBadRequest)
		return
	}
	sub, err := h.svc.Update(c.Request.Context(), userID, c.Param("id"), toSubjectInput(req))
	if err != nil {
		respondServiceError(c, "UpdateSubject", userID, started, err)
		return
	}
	logAcademicOp(c, "UpdateSubject", http.StatusOK, userID, started, "")
	c.JSON(http.StatusOK, sub)
}

// Delete maneja DELETE /subjects/:id → 204.
func (h *SubjectsHandler) Delete(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "DeleteSubject", started)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), userID, c.Param("id")); err != nil {
		respondServiceError(c, "DeleteSubject", userID, started, err)
		return
	}
	logAcademicOp(c, "DeleteSubject", http.StatusNoContent, userID, started, "")
	c.AbortWithStatus(http.StatusNoContent)
}
