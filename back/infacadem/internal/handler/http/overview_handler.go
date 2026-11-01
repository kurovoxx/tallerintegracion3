package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/service"
)

// OverviewHandler expone las vistas agregadas de solo lectura.
type OverviewHandler struct {
	overview *service.OverviewService
	detail   *service.DetailService
}

func NewOverviewHandler(overview *service.OverviewService, detail *service.DetailService) *OverviewHandler {
	return &OverviewHandler{overview: overview, detail: detail}
}

type subjectDetailResponse struct {
	Subject    interface{}         `json:"subject"`
	Schedules  []scheduleResponse  `json:"schedules"`
	Grades     []gradeNodeResponse `json:"grades"`
	Average    interface{}         `json:"average"`
	Attendance interface{}         `json:"attendance"`
}

// Overview maneja GET /academic/overview → 200 {curricula, stats}.
func (h *OverviewHandler) Overview(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "GetOverview", started)
	if !ok {
		return
	}
	out, err := h.overview.Get(c.Request.Context(), userID)
	if err != nil {
		respondServiceError(c, "GetOverview", userID, started, err)
		return
	}
	logAcademicOp(c, "GetOverview", http.StatusOK, userID, started, "")
	c.JSON(http.StatusOK, out)
}

// SubjectDetail maneja GET /subjects/:id/detail → 200 vista completa.
// Misma forma que las rutas individuales; listas [] nunca null.
func (h *OverviewHandler) SubjectDetail(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "GetSubjectDetail", started)
	if !ok {
		return
	}
	d, err := h.detail.Get(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		respondServiceError(c, "GetSubjectDetail", userID, started, err)
		return
	}
	schedules := make([]scheduleResponse, 0, len(d.Schedules))
	for _, s := range d.Schedules {
		schedules = append(schedules, toScheduleResponse(s))
	}
	grades := make([]gradeNodeResponse, 0, len(d.Grades))
	for _, n := range d.Grades {
		grades = append(grades, toGradeNodeResponse(n))
	}
	logAcademicOp(c, "GetSubjectDetail", http.StatusOK, userID, started, "")
	c.JSON(http.StatusOK, subjectDetailResponse{
		Subject: d.Subject, Schedules: schedules, Grades: grades,
		Average: d.Average, Attendance: d.Attendance,
	})
}
