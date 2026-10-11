package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/service"
)

// AttendanceHandler expone el resumen de asistencia (solo lectura).
type AttendanceHandler struct {
	svc *service.AttendanceService
}

func NewAttendanceHandler(svc *service.AttendanceService) *AttendanceHandler {
	return &AttendanceHandler{svc: svc}
}

// Get maneja GET /subjects/:id/attendance → 200 resumen con la fórmula oficial.
func (h *AttendanceHandler) Get(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "GetAttendance", started)
	if !ok {
		return
	}
	out, err := h.svc.Summary(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		respondServiceError(c, "GetAttendance", userID, started, err)
		return
	}
	logAcademicOp(c, "GetAttendance", http.StatusOK, userID, started, "")
	c.JSON(http.StatusOK, out)
}
