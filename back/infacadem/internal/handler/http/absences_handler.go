package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/utils"
)

// AbsencesHandler maneja inasistencias de una materia propia.
type AbsencesHandler struct {
	svc *service.AbsenceService
}

func NewAbsencesHandler(svc *service.AbsenceService) *AbsencesHandler {
	return &AbsencesHandler{svc: svc}
}

type absenceRequest struct {
	Date      string  `json:"date"`
	Justified *bool   `json:"justified"`
	Source    *string `json:"source"`
	Type      *string `json:"type"`
}

// Create maneja POST /subjects/:id/absences → 201 {id}.
// 409 si ya hay una falta en esa fecha.
func (h *AbsencesHandler) Create(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "CreateAbsence", started)
	if !ok {
		return
	}
	var req absenceRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Date == "" {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "Request inválido: date (YYYY-MM-DD) requerido")
		logAcademicOp(c, "CreateAbsence", http.StatusBadRequest, userID, started, "code="+utils.ErrBadRequest)
		return
	}
	justified := req.Justified != nil && *req.Justified
	a, err := h.svc.Create(c.Request.Context(), userID, c.Param("id"), req.Date, justified, req.Source, req.Type)
	if err != nil {
		respondServiceError(c, "CreateAbsence", userID, started, err)
		return
	}
	logAcademicOp(c, "CreateAbsence", http.StatusCreated, userID, started, "")
	c.JSON(http.StatusCreated, gin.H{"id": a.ID})
}

// ListBySubject maneja GET /subjects/:id/absences → 200 {items} por fecha.
func (h *AbsencesHandler) ListBySubject(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "ListAbsences", started)
	if !ok {
		return
	}
	out, err := h.svc.List(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		respondServiceError(c, "ListAbsences", userID, started, err)
		return
	}
	items := make([]absenceResponse, 0, len(out))
	for _, a := range out {
		items = append(items, toAbsenceResponse(a))
	}
	logAcademicOp(c, "ListAbsences", http.StatusOK, userID, started, "")
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// Get maneja GET /absences/:id → 200.
func (h *AbsencesHandler) Get(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "GetAbsence", started)
	if !ok {
		return
	}
	a, err := h.svc.Get(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		respondServiceError(c, "GetAbsence", userID, started, err)
		return
	}
	logAcademicOp(c, "GetAbsence", http.StatusOK, userID, started, "")
	c.JSON(http.StatusOK, toAbsenceResponse(*a))
}

// Update maneja PATCH /absences/:id {justified, source, type} → 200.
func (h *AbsencesHandler) Update(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "UpdateAbsence", started)
	if !ok {
		return
	}
	var req struct {
		Justified *bool   `json:"justified"`
		Source    *string `json:"source"`
		Type      *string `json:"type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badBody(c, "UpdateAbsence", userID, started)
		return
	}
	a, err := h.svc.Update(c.Request.Context(), userID, c.Param("id"), service.AbsenceUpdate{
		Justified: req.Justified, Source: req.Source, Type: req.Type,
	})
	if err != nil {
		respondServiceError(c, "UpdateAbsence", userID, started, err)
		return
	}
	logAcademicOp(c, "UpdateAbsence", http.StatusOK, userID, started, "")
	c.JSON(http.StatusOK, toAbsenceResponse(*a))
}

// Delete maneja DELETE /absences/:id → 204.
func (h *AbsencesHandler) Delete(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "DeleteAbsence", started)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), userID, c.Param("id")); err != nil {
		respondServiceError(c, "DeleteAbsence", userID, started, err)
		return
	}
	logAcademicOp(c, "DeleteAbsence", http.StatusNoContent, userID, started, "")
	c.AbortWithStatus(http.StatusNoContent)
}
