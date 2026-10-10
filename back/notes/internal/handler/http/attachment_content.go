package http

import (
	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/utils"
	"mime"
	"net/http"
)

func (h *NoteHandler) AttachmentContent(c *gin.Context) {
	defer traceNotesOperation(c, "/notes/:id/attachments/:attachment_id/content")()
	userID, ok := middleware.GetUserID(c)
	if !ok {
		respondNotesError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	data, contentType, att, err := h.svc.GetAttachmentContent(c.Request.Context(), userID, c.Param("id"), c.Param("attachmentId"))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	disposition := "attachment"
	if contentType == "image/png" || contentType == "image/jpeg" || contentType == "image/gif" || contentType == "image/webp" || contentType == "application/pdf" {
		disposition = "inline"
	}
	name := "adjunto"
	if att.FileName != nil && *att.FileName != "" {
		name = *att.FileName
	}
	c.Header("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": name}))
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, contentType, data)
}
