package http

import (
	"time"

	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/service"
)

// DTOs de respuesta compartidos: garantizan que los endpoints agregados
// (overview/detail) devuelvan exactamente la misma forma que las rutas
// individuales. Listas vacías como [] (los conversores inicializan slices).

type gradeResponse struct {
	ID                    string  `json:"id"`
	SubjectID             string  `json:"subject_id"`
	ParentGradeID         *string `json:"parent_grade_id"`
	IsFinalExam           bool    `json:"is_final_exam"`
	Status                string  `json:"status"`
	Name                  string  `json:"name"`
	Score                 *int    `json:"score"`
	Weight                float64 `json:"weight"`
	ScheduledDate         *string `json:"scheduled_date"`
	Topics                *string `json:"topics"`
	NotifyDiscord         bool    `json:"notify_discord"`
	GoogleCalendarEventID *string `json:"google_calendar_event_id"`
	CreatedAt             string  `json:"created_at"`
}

type gradeNodeResponse struct {
	Grade    gradeResponse   `json:"grade"`
	Children []gradeResponse `json:"children"`
}

type scheduleResponse struct {
	ID        string  `json:"id"`
	SubjectID string  `json:"subject_id"`
	DayOfWeek int     `json:"day_of_week"`
	StartTime string  `json:"start_time"`
	EndTime   string  `json:"end_time"`
	Modality  *string `json:"modality"`
}

type absenceResponse struct {
	ID        string `json:"id"`
	SubjectID string `json:"subject_id"`
	Date      string `json:"date"`
	Justified bool   `json:"justified"`
	CreatedAt string `json:"created_at"`
}

func formatDate(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02")
	return &s
}

func toGradeResponse(g model.Grade) gradeResponse {
	return gradeResponse{
		ID: g.ID, SubjectID: g.SubjectID, ParentGradeID: g.ParentGradeID,
		IsFinalExam: g.IsFinalExam, Status: g.Status, Name: g.Name,
		Score: g.Score, Weight: g.Weight, ScheduledDate: formatDate(g.ScheduledDate),
		Topics: g.Topics, NotifyDiscord: g.NotifyDiscord,
		GoogleCalendarEventID: g.GoogleCalendarEventID,
		CreatedAt:             g.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func toGradeNodeResponse(n service.GradeNode) gradeNodeResponse {
	children := make([]gradeResponse, 0, len(n.Children))
	for _, ch := range n.Children {
		children = append(children, toGradeResponse(ch))
	}
	return gradeNodeResponse{Grade: toGradeResponse(n.Grade), Children: children}
}

func toScheduleResponse(s model.Schedule) scheduleResponse {
	return scheduleResponse{
		ID:        s.ID,
		SubjectID: s.SubjectID,
		DayOfWeek: s.DayOfWeek,
		StartTime: s.StartTime.Format("15:04:05"),
		EndTime:   s.EndTime.Format("15:04:05"),
		Modality:  s.Modality,
	}
}

func toAbsenceResponse(a model.Absence) absenceResponse {
	return absenceResponse{
		ID: a.ID, SubjectID: a.SubjectID,
		Date: a.Date.Format("2006-01-02"), Justified: a.Justified,
		CreatedAt: a.CreatedAt.UTC().Format(time.RFC3339),
	}
}
