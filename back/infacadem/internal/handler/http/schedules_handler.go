package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/utils"
)

// SchedulesHandler maneja horarios de una materia propia.
type SchedulesHandler struct {
	svc *service.ScheduleService
}

func NewSchedulesHandler(svc *service.ScheduleService) *SchedulesHandler {
	return &SchedulesHandler{svc: svc}
}

type scheduleRequest struct {
	DayOfWeek *int    `json:"day_of_week"`
	StartTime string  `json:"start_time"`
	EndTime   string  `json:"end_time"`
	Modality  *string `json:"modality"`
}

// Create maneja POST /subjects/:id/schedules → 201 {id}.
// 404 si la materia no existe o no es propia.
func (h *SchedulesHandler) Create(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "CreateSchedule", started)
	if !ok {
		return
	}
	var req scheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "Request inválido")
		logAcademicOp(c, "CreateSchedule", http.StatusBadRequest, userID, started, "code="+utils.ErrBadRequest)
		return
	}
	if req.DayOfWeek == nil {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrInvalidDayOfWeek, utils.MessageForCode(utils.ErrInvalidDayOfWeek))
		logAcademicOp(c, "CreateSchedule", http.StatusBadRequest, userID, started, "code="+utils.ErrInvalidDayOfWeek)
		return
	}
	s, err := h.svc.Create(c.Request.Context(), userID, c.Param("id"), *req.DayOfWeek, req.StartTime, req.EndTime, req.Modality)
	if err != nil {
		respondServiceError(c, "CreateSchedule", userID, started, err)
		return
	}
	logAcademicOp(c, "CreateSchedule", http.StatusCreated, userID, started, "")
	c.JSON(http.StatusCreated, gin.H{"id": s.ID})
}

// ListBySubject maneja GET /subjects/:id/schedules → 200 {items}.
func (h *SchedulesHandler) ListBySubject(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "ListSchedules", started)
	if !ok {
		return
	}
	out, err := h.svc.List(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		respondServiceError(c, "ListSchedules", userID, started, err)
		return
	}
	items := make([]scheduleResponse, 0, len(out))
	for _, s := range out {
		items = append(items, toScheduleResponse(s))
	}
	logAcademicOp(c, "ListSchedules", http.StatusOK, userID, started, "")
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// Delete maneja DELETE /schedules/:id → 204.
func (h *SchedulesHandler) Delete(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "DeleteSchedule", started)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), userID, c.Param("id")); err != nil {
		respondServiceError(c, "DeleteSchedule", userID, started, err)
		return
	}
	logAcademicOp(c, "DeleteSchedule", http.StatusNoContent, userID, started, "")
	c.AbortWithStatus(http.StatusNoContent)
}
