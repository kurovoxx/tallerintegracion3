package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/service"
)

// RequirementsHandler maneja requisitos de aprobación de una materia propia.
type RequirementsHandler struct {
	svc *service.RequirementService
}

func NewRequirementsHandler(svc *service.RequirementService) *RequirementsHandler {
	return &RequirementsHandler{svc: svc}
}

type requirementRequest struct {
	SourceGradeID      *string  `json:"source_grade_id"`
	ChecksFinalAverage *bool    `json:"checks_final_average"`
	Comparator         *string  `json:"comparator"`
	ThresholdValue     *float64 `json:"threshold_value"`
	Action             *string  `json:"action"`
}

// Create maneja POST /subjects/:id/requirements → 201 {id}.
func (h *RequirementsHandler) Create(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "CreateRequirement", started)
	if !ok {
		return
	}
	var req requirementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badBody(c, "CreateRequirement", userID, started)
		return
	}
	out, err := h.svc.Create(c.Request.Context(), userID, c.Param("id"), service.RequirementInput{
		SourceGradeID: req.SourceGradeID, ChecksFinalAverage: req.ChecksFinalAverage,
		Comparator: req.Comparator, ThresholdValue: req.ThresholdValue, Action: req.Action,
	})
	if err != nil {
		respondServiceError(c, "CreateRequirement", userID, started, err)
		return
	}
	logAcademicOp(c, "CreateRequirement", http.StatusCreated, userID, started, "")
	c.JSON(http.StatusCreated, gin.H{"id": out.ID})
}

// List maneja GET /subjects/:id/requirements → 200 {items}.
func (h *RequirementsHandler) List(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "ListRequirements", started)
	if !ok {
		return
	}
	out, err := h.svc.List(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		respondServiceError(c, "ListRequirements", userID, started, err)
		return
	}
	logAcademicOp(c, "ListRequirements", http.StatusOK, userID, started, "")
	c.JSON(http.StatusOK, gin.H{"items": out})
}

// Delete maneja DELETE /requirements/:id → 204.
func (h *RequirementsHandler) Delete(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "DeleteRequirement", started)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), userID, c.Param("id")); err != nil {
		respondServiceError(c, "DeleteRequirement", userID, started, err)
		return
	}
	logAcademicOp(c, "DeleteRequirement", http.StatusNoContent, userID, started, "")
	c.AbortWithStatus(http.StatusNoContent)
}
