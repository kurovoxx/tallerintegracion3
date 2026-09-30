package http

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
	"net/http"
)

func sheetJSON(s sqlc.SocialSprintSheet) gin.H {
	var start, end *string
	if s.PeriodStart.Valid {
		v := s.PeriodStart.Time.Format("2006-01-02")
		start = &v
	}
	if s.PeriodEnd.Valid {
		v := s.PeriodEnd.Time.Format("2006-01-02")
		end = &v
	}
	return gin.H{"id": s.ID, "name": s.Name, "period_start": start, "period_end": end}
}
func sheetError(c *gin.Context, err error) {
	code, status := "internal", http.StatusInternalServerError
	switch {
	case errors.Is(err, service.ErrForbidden):
		code, status = "forbidden", 403
	case errors.Is(err, service.ErrSheetNotFound), errors.Is(err, service.ErrGroupNotFound):
		code, status = "not_found", 404
	case errors.Is(err, service.ErrInvalidSheet), errors.Is(err, service.ErrInvalidSheetID), errors.Is(err, service.ErrInvalidGroupID), errors.Is(err, service.ErrInvalidUserID):
		code, status = "invalid_body", 400
	}
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": "No se pudo completar la operación del sprint."}})
}
func (h *SprintHandler) ListSheets(c *gin.Context) {
	uid, ok := middleware.GetUserID(c)
	if !ok {
		c.Status(401)
		return
	}
	sheets, err := h.svc.ListSprintSheets(c.Request.Context(), c.Param("id"), uid)
	if err != nil {
		sheetError(c, err)
		return
	}
	data := make([]gin.H, 0, len(sheets))
	for _, s := range sheets {
		data = append(data, sheetJSON(s))
	}
	c.JSON(200, gin.H{"data": data})
}
func (h *SprintHandler) CreateSheet(c *gin.Context) { h.writeSheet(c, false) }
func (h *SprintHandler) UpdateSheet(c *gin.Context) { h.writeSheet(c, true) }
func (h *SprintHandler) writeSheet(c *gin.Context, update bool) {
	uid, ok := middleware.GetUserID(c)
	if !ok {
		c.Status(401)
		return
	}
	var patch service.SheetPatch
	if err := c.ShouldBindJSON(&patch); err != nil {
		sheetError(c, service.ErrInvalidSheet)
		return
	}
	var sheet sqlc.SocialSprintSheet
	var err error
	status := 201
	if update {
		status = 200
		sheet, err = h.svc.UpdateSprintSheet(c.Request.Context(), c.Param("id"), uid, c.Param("sheetId"), patch)
	} else {
		sheet, err = h.svc.CreateSprintSheet(c.Request.Context(), c.Param("id"), uid, patch)
	}
	if err != nil {
		sheetError(c, err)
		return
	}
	c.JSON(status, gin.H{"data": sheetJSON(sheet)})
}
