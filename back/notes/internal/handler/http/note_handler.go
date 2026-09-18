package http

import (
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/utils"
)

type NoteHandler struct {
	svc *service.NoteService
}

func NewNoteHandler(svc *service.NoteService) *NoteHandler {
	return &NoteHandler{svc: svc}
}

// POST /notes
type createRequest struct {
	Title      string  `json:"title" binding:"required"`
	SubjectID  *string `json:"subject_id"`
	Visibility string  `json:"visibility" binding:"required"`
	Content    *string `json:"content"` // opcional para compatibilidad con spec TAREA
	Tags       *string `json:"tags"`
}

func (h *NoteHandler) Create(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "Request inválido: "+err.Error())
		return
	}
	log.Printf("[HTTP DEBUG] POST /notes recibido. Payload: %+v, RequesterID: %s", req, userID)
	note, err := h.svc.Create(c.Request.Context(), userID, req.Title, req.SubjectID, req.Visibility, req.Content, req.Tags)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"note_id": note.ID})
}

// GET /notes/{id}
func (h *NoteHandler) Get(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	id := c.Param("id")
	note, err := h.svc.Get(c.Request.Context(), userID, id)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	// respuesta según contrato: 200 {title, content (descargado de Drive), likes_count, ...}
	resp := gin.H{
		"id":           note.ID,
		"user_id":      note.UserID,
		"title":        note.Title,
		"visibility":   note.Visibility,
		"likes_count":  note.LikesCount,
		"created_at":   note.CreatedAt,
		"updated_at":   note.UpdatedAt,
	}
	if note.SubjectID != nil {
		resp["subject_id"] = *note.SubjectID
	}
	if note.ExternalFileID != nil {
		resp["external_file_id"] = *note.ExternalFileID
		// drive_url opcional
		resp["drive_url"] = "https://drive.google.com/file/d/" + *note.ExternalFileID + "/view"
	}
	if note.ForkedFromNoteID != nil {
		resp["forked_from_note_id"] = *note.ForkedFromNoteID
	}
	if note.Content != nil {
		resp["content"] = *note.Content
	}
	c.JSON(http.StatusOK, resp)
}

// GET /notes/me
func (h *NoteHandler) ListMy(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	cursor := c.Query("cursor")
	limitStr := c.Query("limit")
	limit := 20
	if limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil {
			limit = v
		}
	}
	notes, next, err := h.svc.ListMy(c.Request.Context(), userID, cursor, limit)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	// mapear a array de objetos minimal
	out := make([]gin.H, 0, len(notes))
	for _, n := range notes {
		item := gin.H{
			"id":          n.ID,
			"user_id":     n.UserID,
			"title":       n.Title,
			"visibility":  n.Visibility,
			"likes_count": n.LikesCount,
			"created_at":  n.CreatedAt,
			"updated_at":  n.UpdatedAt,
		}
		if n.SubjectID != nil {
			item["subject_id"] = *n.SubjectID
		}
		if n.ExternalFileID != nil {
			item["external_file_id"] = *n.ExternalFileID
		}
		out = append(out, item)
	}
	c.JSON(http.StatusOK, gin.H{"notes": out, "next_cursor": next})
}

// PATCH /notes/{id}
type patchRequest struct {
	Title      *string `json:"title"`
	Visibility *string `json:"visibility"`
	Content    *string `json:"content"` // opcional para sync Drive
}

func (h *NoteHandler) Patch(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	id := c.Param("id")
	var req patchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "Request inválido")
		return
	}
	if req.Title == nil && req.Visibility == nil && req.Content == nil {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "nada que actualizar")
		return
	}
	updated, err := h.svc.Update(c.Request.Context(), userID, id, req.Title, req.Visibility, req.Content)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":         updated.ID,
		"title":      updated.Title,
		"visibility": updated.Visibility,
		"updated_at": updated.UpdatedAt,
	})
}

// DELETE /notes/{id}
func (h *NoteHandler) Delete(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	id := c.Param("id")
	if err := h.svc.Delete(c.Request.Context(), userID, id); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// POST /notes/{id}/attachments
func (h *NoteHandler) UploadAttachment(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	noteID := c.Param("id")

	// Soportar multipart {file} o JSON {external_file_id} (cliente ya subió directo a Drive)
	contentType := c.GetHeader("Content-Type")
	if strings.Contains(contentType, "application/json") {
		var body struct {
			ExternalFileID *string `json:"external_file_id"`
			FileName       *string `json:"file_name"`
			FileType       *string `json:"file_type"`
			FileSize       *int    `json:"file_size_bytes"`
			IsInline       *bool   `json:"is_inline"`
		}
		// Leer body sin consumir multipart: si es JSON, parsear
		if err := c.ShouldBindJSON(&body); err == nil && body.ExternalFileID != nil {
			isInline := false
			if body.IsInline != nil {
				isInline = *body.IsInline
			}
			ft := "application/octet-stream"
			if body.FileType != nil {
				ft = *body.FileType
			}
			fn := ""
			if body.FileName != nil {
				fn = *body.FileName
			}
			att, err := h.svc.AddAttachmentExternal(c.Request.Context(), userID, noteID, *body.ExternalFileID, fn, ft, isInline, body.FileSize)
			if err != nil {
				handleServiceError(c, err)
				return
			}
			c.JSON(http.StatusCreated, gin.H{"attachment_id": att.ID, "is_inline": att.IsInline, "file_url": att.FileURL})
			return
		}
	}

	// Multipart file
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		// intentar leer body raw como file?
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "file requerido (multipart field 'file')")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		utils.RespondError(c, http.StatusInternalServerError, utils.ErrInternal, "error leyendo archivo")
		return
	}
	// límite 10MB según spec 413
	if header.Size > 10*1024*1024 || int64(len(data)) > 10*1024*1024 {
		utils.RespondError(c, http.StatusRequestEntityTooLarge, utils.ErrFileTooLarge, utils.MessageForCode(utils.ErrFileTooLarge))
		return
	}
	// también respetar header
	isInline := c.Query("is_inline") == "true" || c.PostForm("is_inline") == "true"
	fileType := header.Header.Get("Content-Type")
	if fileType == "" {
		fileType = "application/octet-stream"
	}
	att, err := h.svc.AddAttachment(c.Request.Context(), userID, noteID, header.Filename, fileType, data, isInline)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"attachment_id": att.ID, "is_inline": att.IsInline, "file_url": att.FileURL})
}

// DELETE /notes/{id}/attachments/{attachmentId}
func (h *NoteHandler) DeleteAttachment(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	noteID := c.Param("id")
	attID := c.Param("attachmentId")
	if attID == "" {
		attID = c.Param("attachment_id")
	}
	if err := h.svc.RemoveAttachment(c.Request.Context(), userID, noteID, attID); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// POST /notes/{id}/save
func (h *NoteHandler) Save(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	noteID := c.Param("id")
	_, err := h.svc.Save(c.Request.Context(), userID, noteID)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusCreated)
}

// POST /notes/{id}/copy
func (h *NoteHandler) Copy(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	noteID := c.Param("id")
	newNote, err := h.svc.Copy(c.Request.Context(), userID, noteID)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"note_id": newNote.ID, "forked_from_note_id": noteID})
}

// POST /notes/{id}/like
func (h *NoteHandler) Like(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	noteID := c.Param("id")
	if err := h.svc.Like(c.Request.Context(), userID, noteID); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusCreated)
}

// DELETE /notes/{id}/like
func (h *NoteHandler) Unlike(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	noteID := c.Param("id")
	if err := h.svc.Unlike(c.Request.Context(), userID, noteID); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// POST /notes/{id}/share
type shareRequest struct {
	GroupID    string `json:"group_id" binding:"required"`
	AccessMode string `json:"access_mode" binding:"required"`
}

func (h *NoteHandler) Share(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	noteID := c.Param("id")
	var req shareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "group_id y access_mode requeridos")
		return
	}
	shared, err := h.svc.Share(c.Request.Context(), userID, noteID, req.GroupID, req.AccessMode)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"shared_note_id": shared.ID})
}

// DELETE /notes/shared/{sharedNoteId}
func (h *NoteHandler) Unshare(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	sharedID := c.Param("sharedNoteId")
	if sharedID == "" {
		sharedID = c.Param("id")
	}
	if err := h.svc.Unshare(c.Request.Context(), userID, sharedID); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// POST /notes/unshare-all
type unshareAllRequest struct {
	UserID  string `json:"user_id" binding:"required"`
	GroupID string `json:"group_id" binding:"required"`
}

func (h *NoteHandler) UnshareAll(c *gin.Context) {
	// interno, llamado por Social; requiere auth pero no valida pertenencia estricta si es interno
	var req unshareAllRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "user_id y group_id requeridos")
		return
	}
	if err := h.svc.UnshareAll(c.Request.Context(), req.UserID, req.GroupID); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// GET /notes/{id}/access
func (h *NoteHandler) GetAccess(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	noteID := c.Param("id")
	access, err := h.svc.GetAccess(c.Request.Context(), userID, noteID)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, access)
}

// GET /groups/{id}/notes
func (h *NoteHandler) ListGroupNotes(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	groupID := c.Param("id")
	cursor := c.Query("cursor")
	limitStr := c.Query("limit")
	limit := 20
	if limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil {
			limit = v
		}
	}
	notes, next, err := h.svc.ListGroupNotes(c.Request.Context(), userID, groupID, cursor, limit)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	out := make([]gin.H, 0, len(notes))
	for _, n := range notes {
		item := gin.H{
			"id": n.ID, "user_id": n.UserID, "title": n.Title,
			"visibility": n.Visibility, "likes_count": n.LikesCount,
			"created_at": n.CreatedAt, "updated_at": n.UpdatedAt,
		}
		out = append(out, item)
	}
	c.JSON(http.StatusOK, gin.H{"notes": out, "next_cursor": next})
}

func handleServiceError(c *gin.Context, err error) {
	if se, ok := err.(*service.ServiceError); ok {
		status := utils.StatusForCode(se.Code)
		// Mapeos específicos contrato HTTP
		switch se.Code {
		case utils.ErrInvalidTitle, utils.ErrInvalidVisibility, utils.ErrInvalidSubjectID, utils.ErrInvalidAccessMode, utils.ErrBadRequest:
			status = http.StatusBadRequest
		case utils.ErrUnauthorized, utils.ErrInvalidToken, utils.ErrTokenExpired:
			status = http.StatusUnauthorized
		case utils.ErrForbidden:
			status = http.StatusForbidden
		case utils.ErrNotFound:
			status = http.StatusNotFound
		case utils.ErrNoteUnavailable:
			// spec: 404 o 410 no disponible (403/404 de Drive) -> usamos 404 con mensaje "Nota no disponible..."
			status = http.StatusNotFound
		case utils.ErrAlreadySaved, utils.ErrAlreadyLiked, utils.ErrConflict:
			status = http.StatusConflict
		case utils.ErrFileTooLarge:
			status = http.StatusRequestEntityTooLarge
		}
		utils.RespondError(c, status, se.Code, se.Message)
		return
	}
	utils.RespondError(c, http.StatusInternalServerError, utils.ErrInternal, err.Error())
}
