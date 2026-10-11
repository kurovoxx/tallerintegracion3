package http

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/utils"
)

// GradesHandler maneja grades, subnotas y promedio.
type GradesHandler struct {
	grades *service.GradeService
	avg    *service.AverageService
}

func NewGradesHandler(grades *service.GradeService, avg *service.AverageService) *GradesHandler {
	return &GradesHandler{grades: grades, avg: avg}
}

type gradeRequest struct {
	Name                  *string  `json:"name"`
	Score                 *int     `json:"score"`
	Weight                *float64 `json:"weight"`
	Status                *string  `json:"status"`
	ScheduledDate         *string  `json:"scheduled_date"`
	Topics                *string  `json:"topics"`
	NotifyDiscord         *bool    `json:"notify_discord"`
	GoogleCalendarEventID *string  `json:"google_calendar_event_id"`
	IsFinalExam           *bool    `json:"is_final_exam"`
}

// parseGradeInput decodifica el body; en update, un null JSON explícito en
// score/scheduled_date/topics/google_calendar_event_id pide NULL (Clear).
func parseGradeInput(c *gin.Context, forUpdate bool) (service.GradeInput, bool) {
	var req gradeRequest
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return service.GradeInput{}, false
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return service.GradeInput{}, false
	}
	in := service.GradeInput{
		Name: req.Name, Score: req.Score, Weight: req.Weight, Status: req.Status,
		ScheduledDate: req.ScheduledDate, Topics: req.Topics,
		NotifyDiscord: req.NotifyDiscord, GoogleCalendarEventID: req.GoogleCalendarEventID,
		IsFinalExam: req.IsFinalExam,
	}
	if forUpdate {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(body, &raw); err == nil {
			isNull := func(k string) bool {
				v, ok := raw[k]
				return ok && string(v) == "null"
			}
			in.ClearScore = isNull("score")
			in.ClearScheduledDate = isNull("scheduled_date")
			in.ClearTopics = isNull("topics")
			in.ClearCalendarEvent = isNull("google_calendar_event_id")
		}
	}
	return in, true
}

func badBody(c *gin.Context, op, userID string, started time.Time) {
	utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "Request inválido")
	logAcademicOp(c, op, http.StatusBadRequest, userID, started, "code="+utils.ErrBadRequest)
}

// Create maneja POST /subjects/:id/grades → 201 {id}.
func (h *GradesHandler) Create(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "CreateGrade", started)
	if !ok {
		return
	}
	in, valid := parseGradeInput(c, false)
	if !valid {
		badBody(c, "CreateGrade", userID, started)
		return
	}
	g, err := h.grades.Create(c.Request.Context(), userID, c.Param("id"), in)
	if err != nil {
		respondServiceError(c, "CreateGrade", userID, started, err)
		return
	}
	logAcademicOp(c, "CreateGrade", http.StatusCreated, userID, started, "")
	c.JSON(http.StatusCreated, gin.H{"id": g.ID})
}

// List maneja GET /subjects/:id/grades → 200 {items: [{grade, children}]}.
func (h *GradesHandler) List(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "ListGrades", started)
	if !ok {
		return
	}
	nodes, err := h.grades.List(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		respondServiceError(c, "ListGrades", userID, started, err)
		return
	}
	items := make([]gradeNodeResponse, 0, len(nodes))
	for _, n := range nodes {
		items = append(items, toGradeNodeResponse(n))
	}
	logAcademicOp(c, "ListGrades", http.StatusOK, userID, started, "")
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// Get maneja GET /grades/:id → 200.
func (h *GradesHandler) Get(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "GetGrade", started)
	if !ok {
		return
	}
	g, err := h.grades.Get(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		respondServiceError(c, "GetGrade", userID, started, err)
		return
	}
	logAcademicOp(c, "GetGrade", http.StatusOK, userID, started, "")
	c.JSON(http.StatusOK, toGradeResponse(*g))
}

// Update maneja PATCH /grades/:id → 200.
func (h *GradesHandler) Update(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "UpdateGrade", started)
	if !ok {
		return
	}
	in, valid := parseGradeInput(c, true)
	if !valid {
		badBody(c, "UpdateGrade", userID, started)
		return
	}
	g, err := h.grades.Update(c.Request.Context(), userID, c.Param("id"), in)
	if err != nil {
		respondServiceError(c, "UpdateGrade", userID, started, err)
		return
	}
	logAcademicOp(c, "UpdateGrade", http.StatusOK, userID, started, "")
	c.JSON(http.StatusOK, toGradeResponse(*g))
}

// Delete maneja DELETE /grades/:id → 204.
func (h *GradesHandler) Delete(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "DeleteGrade", started)
	if !ok {
		return
	}
	if err := h.grades.Delete(c.Request.Context(), userID, c.Param("id")); err != nil {
		respondServiceError(c, "DeleteGrade", userID, started, err)
		return
	}
	logAcademicOp(c, "DeleteGrade", http.StatusNoContent, userID, started, "")
	c.AbortWithStatus(http.StatusNoContent)
}

// CreateChild maneja POST /grades/:id/children → 201 {id}.
func (h *GradesHandler) CreateChild(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "CreateGradeChild", started)
	if !ok {
		return
	}
	in, valid := parseGradeInput(c, false)
	if !valid {
		badBody(c, "CreateGradeChild", userID, started)
		return
	}
	g, err := h.grades.CreateChild(c.Request.Context(), userID, c.Param("id"), in)
	if err != nil {
		respondServiceError(c, "CreateGradeChild", userID, started, err)
		return
	}
	logAcademicOp(c, "CreateGradeChild", http.StatusCreated, userID, started, "")
	c.JSON(http.StatusCreated, gin.H{"id": g.ID})
}

// Average maneja GET /subjects/:id/average → 200 {average, status}.
func (h *GradesHandler) Average(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "GetAverage", started)
	if !ok {
		return
	}
	res, err := h.avg.Average(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		respondServiceError(c, "GetAverage", userID, started, err)
		return
	}
	logAcademicOp(c, "GetAverage", http.StatusOK, userID, started, "")
	c.JSON(http.StatusOK, res)
}
