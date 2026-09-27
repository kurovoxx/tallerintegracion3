package http

import (
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/utils"
)

const (
	// copyRateLimitMax es el máximo de peticiones de clonación permitidas por
	// usuario dentro de la ventana de tiempo.
	copyRateLimitMax = 10
	// copyRateLimitGlobalMax es el máximo de peticiones de clonación permitidas
	// en total entre todos los usuarios dentro de la ventana de tiempo: evita
	// que un ataque distribuido con muchos usuarios sature el upstream de Drive.
	copyRateLimitGlobalMax = 100
	// copyRateLimitWindow es la ventana temporal deslizante del rate limit.
	copyRateLimitWindow = time.Minute
	// copyRateLimitBucketMax es el máximo de buckets por usuario admitidos en
	// memoria antes de podar los inactivos: acota el mapa ante un ataque de
	// spoofing de userIDs (tantos buckets como IDs distintos) para evitar OOM.
	copyRateLimitBucketMax = 500
)

// copyRateLimiter limita en memoria la tasa de clonaciones con una ventana
// deslizante de 1 minuto, aplicando dos topes simultáneos: un máximo por
// usuario (copyRateLimitMax) y un máximo global entre todos los usuarios
// (copyRateLimitGlobalMax). Es seguro para uso concurrente (sync.Mutex) y purga
// en cada evaluación los timestamps vencidos, de modo que no acumula memoria por
// usuario ni globalmente.
type copyRateLimiter struct {
	mu          sync.Mutex
	buckets     map[string][]time.Time
	globalTimes []time.Time
}

func newCopyRateLimiter() *copyRateLimiter {
	return &copyRateLimiter{buckets: make(map[string][]time.Time)}
}

// allow registra el intento del usuario y devuelve (permitido, límiteGlobal).
// Si se supera el tope global o el tope del usuario dentro de la ventana de
// 1 minuto, el intento se rechaza (no se registra). El segundo valor indica si
// la denegación fue por el tope global, para que el handler responda 429 con un
// mensaje claro según corresponda.
func (l *copyRateLimiter) allow(userID string) (bool, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	// Lock exclusivo total desde la primera línea (antes de leer el reloj):
	// todas las verificaciones (global y por usuario), las purgas y los append
	// ocurren de forma indivisible, sin ventanas de carrera entre el snapshot
	// temporal y el estado de los slices.
	now := time.Now()
	cutoff := now.Add(-copyRateLimitWindow)
	// Purga y evaluación del tope global (suma de todos los usuarios): se
	// construye un slice nuevo (copia defensiva) en lugar de reutilizar el
	// backing array con [:0], de modo que ningún slice compartido quede vivo
	// fuera del mutex ni se pise memoria de una evaluación concurrente. Si no
	// queda ningún timestamp activo, se libera el slice (nil) para devolver el
	// backing array al GC.
	active := make([]time.Time, 0, len(l.globalTimes))
	for _, ts := range l.globalTimes {
		if ts.After(cutoff) {
			active = append(active, ts)
		}
	}
	if len(active) == 0 {
		l.globalTimes = nil
	} else {
		l.globalTimes = active
	}
	// Purga y evaluación del tope por usuario (misma copia defensiva). Un
	// bucket sin timestamps vigentes se elimina del mapa en lugar de reasignar
	// un slice vacío: evita que userIDs efímeros/spoofeados acumulen entradas.
	uactive := make([]time.Time, 0, len(l.buckets[userID]))
	for _, ts := range l.buckets[userID] {
		if ts.After(cutoff) {
			uactive = append(uactive, ts)
		}
	}
	if len(uactive) == 0 {
		delete(l.buckets, userID)
	} else {
		l.buckets[userID] = uactive
	}
	// Poda anti-OOM: si el mapa creció por encima del tope (spoofing de
	// userIDs), se eliminan todos los buckets sin timestamps vigentes.
	if len(l.buckets) > copyRateLimitBucketMax {
		for k, times := range l.buckets {
			hasActive := false
			for _, ts := range times {
				if ts.After(cutoff) {
					hasActive = true
					break
				}
			}
			if !hasActive {
				delete(l.buckets, k)
			}
		}
	}
	if len(l.globalTimes) >= copyRateLimitGlobalMax {
		return false, true
	}
	if len(uactive) >= copyRateLimitMax {
		return false, false
	}
	l.buckets[userID] = append(uactive, now)
	l.globalTimes = append(l.globalTimes, now)
	return true, false
}

type NoteHandler struct {
	svc         *service.NoteService
	copyLimiter *copyRateLimiter
}

func NewNoteHandler(svc *service.NoteService) *NoteHandler {
	return &NoteHandler{svc: svc, copyLimiter: newCopyRateLimiter()}
}

// POST /notes
type createRequest struct {
	Title      string  `json:"title" binding:"required,max=300"`
	SubjectID  *string `json:"subject_id" binding:"omitempty,uuid"`
	Visibility string  `json:"visibility" binding:"required,oneof=public private"`
	Content    *string `json:"content"` // opcional para compatibilidad con spec TAREA
}

// validateNoteID valida formato UUID del path param antes de llegar al store.
// Retorna false y responde 404 si el formato es inválido (evita depender del
// casteo de Postgres y unifica el contrato: recurso mal formado = no encontrado).
func validateNoteID(c *gin.Context, id string) bool {
	if !utils.ValidateUUID(id) {
		utils.RespondError(c, http.StatusNotFound, utils.ErrNotFound, "nota no encontrada")
		return false
	}
	return true
}

func validateUUIDParam(c *gin.Context, id, resource string) bool {
	if !utils.ValidateUUID(id) {
		utils.RespondError(c, http.StatusNotFound, utils.ErrNotFound, resource+" no encontrado")
		return false
	}
	return true
}

// isPayloadTooLarge detecta el error de http.MaxBytesReader de forma robusta
// mediante errors.As(*http.MaxBytesError) en lugar de comparar substrings del
// mensaje de límite excedido, que es frágil ante cambios de redacción
// o wrapping por parte de Gin/multipart.
func isPayloadTooLarge(err error) bool {
	var maxBytesErr *http.MaxBytesError
	return errors.As(err, &maxBytesErr)
}

func (h *NoteHandler) Create(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	var req createRequest
	// Límite anti-DoS: JSON de notas máximo 1MB (content grande va a Drive, no al JSON).
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1048576)
	if err := c.ShouldBindJSON(&req); err != nil {
		if isPayloadTooLarge(err) {
			utils.RespondError(c, http.StatusRequestEntityTooLarge, utils.ErrFileTooLarge, utils.MessageForCode(utils.ErrFileTooLarge))
			return
		}
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "Request inválido")
		return
	}
	idemKey := c.GetHeader("X-Idempotency-Key")
	// Firma canónica alineada con el servicio:
	// (ctx, userID, title, subjectID, visibility, content, idempotencyKey).
	note, err := h.svc.Create(c.Request.Context(), userID, req.Title, req.SubjectID, req.Visibility, req.Content, idemKey)
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
	if !validateNoteID(c, id) {
		return
	}
	note, err := h.svc.Get(c.Request.Context(), userID, id)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	// respuesta según contrato: 200 {title, content (descargado de Drive), likes_count, ...}
	resp := gin.H{
		"id":          note.ID,
		"user_id":     note.UserID,
		"title":       note.Title,
		"visibility":  note.Visibility,
		"likes_count": note.LikesCount,
		"created_at":  note.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":  note.UpdatedAt.UTC().Format(time.RFC3339),
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
			"created_at":  n.CreatedAt.UTC().Format(time.RFC3339),
			"updated_at":  n.UpdatedAt.UTC().Format(time.RFC3339),
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
	Title      *string `json:"title" binding:"omitempty,min=1,max=300"`
	Visibility *string `json:"visibility" binding:"omitempty,oneof=public private"`
	Content    *string `json:"content"` // opcional para sync Drive
	// Version es la precondición de versionado optimista: si se envía, el
	// cambio solo se aplica cuando la nota sigue en esa versión (409 si no).
	Version *int64 `json:"version" binding:"omitempty,min=1"`
}

func (h *NoteHandler) Patch(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	id := c.Param("id")
	if !validateNoteID(c, id) {
		return
	}
	var req patchRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1048576)
	if err := c.ShouldBindJSON(&req); err != nil {
		if isPayloadTooLarge(err) {
			utils.RespondError(c, http.StatusRequestEntityTooLarge, utils.ErrFileTooLarge, utils.MessageForCode(utils.ErrFileTooLarge))
			return
		}
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "Request inválido")
		return
	}
	if req.Title == nil && req.Visibility == nil && req.Content == nil {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "nada que actualizar")
		return
	}
	// Idempotencia opcional: si el cliente envía X-Idempotency-Key, el servicio
	// responde a los replays de red con el resultado cacheado (TTL 10 min) sin
	// re-aplicar el cambio en PG/Drive.
	idemKey := c.GetHeader("X-Idempotency-Key")
	expectedVersion := int64(0)
	if req.Version != nil {
		expectedVersion = *req.Version
	}
	updated, err := h.svc.UpdateWithExpectedVersion(c.Request.Context(), userID, id, req.Title, req.Visibility, req.Content, expectedVersion, idemKey)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":         updated.ID,
		"title":      updated.Title,
		"visibility": updated.Visibility,
		"version":    updated.Version,
		"updated_at": updated.UpdatedAt.UTC().Format(time.RFC3339),
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
	if !validateNoteID(c, id) {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), userID, id); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// respondMimeValidationError traduce los errores tipados del sniffing estricto
// de adjuntos a la respuesta HTTP canónica: 415 cuando el contenido real no
// pertenece a la whitelist (image/jpeg, image/png, application/pdf) y 400
// cuando el MIME declarado por el cliente no coincide con el contenido.
func respondMimeValidationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, drive.ErrUnsupportedMimeType):
		utils.RespondError(c, http.StatusUnsupportedMediaType, utils.ErrUnsupportedMediaType, "Tipo de archivo no permitido: solo image/jpeg, image/png o application/pdf")
	case errors.Is(err, drive.ErrMimeTypeMismatch):
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "El contenido del archivo no coincide con el tipo declarado")
	default:
		utils.RespondError(c, http.StatusUnsupportedMediaType, utils.ErrUnsupportedMediaType, utils.MessageForCode(utils.ErrUnsupportedMediaType))
	}
}

// POST /notes/{id}/attachments
func (h *NoteHandler) UploadAttachment(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	noteID := c.Param("id")
	if !validateNoteID(c, noteID) {
		return
	}

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
		// Límite anti-DoS para JSON de adjuntos externos (1MB).
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1048576)
		if err := c.ShouldBindJSON(&body); err != nil {
			if isPayloadTooLarge(err) {
				utils.RespondError(c, http.StatusRequestEntityTooLarge, utils.ErrFileTooLarge, utils.MessageForCode(utils.ErrFileTooLarge))
			} else {
				utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "file requerido (external_file_id)")
			}
			return
		}
		if body.ExternalFileID == nil {
			utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "external_file_id requerido")
			return
		}
		{
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
			// El registro externo no aporta binario que sniffar: se exige que el
			// tipo declarado (o resuelto por extensión) pertenezca a la misma
			// whitelist estricta, para que no sea un bypass del sniffing.
			if !drive.IsAllowedAttachmentMimeType(drive.DetectMimeType(fn, ft, nil)) {
				utils.RespondError(c, http.StatusUnsupportedMediaType, utils.ErrUnsupportedMediaType, "Tipo de archivo no permitido: solo image/jpeg, image/png o application/pdf")
				return
			}
			att, err := h.svc.AddAttachmentExternal(c.Request.Context(), userID, noteID, *body.ExternalFileID, fn, ft, isInline, body.FileSize)
			if err != nil {
				handleServiceError(c, err)
				return
			}
			c.JSON(http.StatusCreated, gin.H{"attachment_id": att.ID, "is_inline": att.IsInline, "file_url": att.FileURL, "external_file_id": att.ExternalFileID})
			return
		}
	}

	// Multipart: límite anti-DoS 11MB (10MB archivo + overhead multipart).
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 11*1024*1024)

	// Multipart file
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		if isPayloadTooLarge(err) {
			utils.RespondError(c, http.StatusRequestEntityTooLarge, utils.ErrFileTooLarge, utils.MessageForCode(utils.ErrFileTooLarge))
			return
		}
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
	if len(data) == 0 {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "archivo vacío")
		return
	}
	// también respetar header
	isInline := c.Query("is_inline") == "true" || c.PostForm("is_inline") == "true"
	fileType := header.Header.Get("Content-Type")
	if fileType == "" {
		fileType = "application/octet-stream"
	}
	// Sniffing estricto: el tipo real (primeros 512 bytes) manda. Se rechaza
	// 415 si no está en la whitelist y 400 si el MIME declarado lo contradice.
	sniffedType, mimeErr := drive.ValidateAttachmentMime(fileType, data)
	if mimeErr != nil {
		respondMimeValidationError(c, mimeErr)
		return
	}
	att, err := h.svc.AddAttachment(c.Request.Context(), userID, noteID, header.Filename, sniffedType, data, isInline)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"attachment_id": att.ID, "is_inline": att.IsInline, "file_url": att.FileURL, "external_file_id": att.ExternalFileID})
}

// POST /notes/upload
// Sube un archivo directo al Drive del usuario autenticado (OAuth) sin
// vincularlo todavía a una nota. Devuelve el Drive File ID y su enlace web;
// el cliente puede luego registrarlo con POST /notes/{id}/attachments enviando
// {external_file_id}. Acepta multipart/form-data con el campo binario 'file'.
func (h *NoteHandler) UploadFile(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	// Límite anti-DoS 11MB (10MB archivo + overhead multipart).
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 11*1024*1024)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		if isPayloadTooLarge(err) {
			utils.RespondError(c, http.StatusRequestEntityTooLarge, utils.ErrFileTooLarge, utils.MessageForCode(utils.ErrFileTooLarge))
			return
		}
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "file requerido (multipart field 'file')")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		utils.RespondError(c, http.StatusInternalServerError, utils.ErrInternal, "error leyendo archivo")
		return
	}
	if header.Size > 10*1024*1024 || int64(len(data)) > 10*1024*1024 {
		utils.RespondError(c, http.StatusRequestEntityTooLarge, utils.ErrFileTooLarge, utils.MessageForCode(utils.ErrFileTooLarge))
		return
	}
	if len(data) == 0 {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "archivo vacío")
		return
	}
	fileType := header.Header.Get("Content-Type")
	if strings.TrimSpace(fileType) == "" {
		fileType = "application/octet-stream"
	}
	// Sniffing estricto: el tipo real (primeros 512 bytes) manda. Se rechaza
	// 415 si no está en la whitelist y 400 si el MIME declarado lo contradice.
	sniffedType, mimeErr := drive.ValidateAttachmentMime(fileType, data)
	if mimeErr != nil {
		respondMimeValidationError(c, mimeErr)
		return
	}
	up, err := h.svc.UploadToDrive(c.Request.Context(), userID, header.Filename, sniffedType, data)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"external_file_id": up.ExternalFileID,
		"file_url":         up.FileURL,
		"file_name":        up.FileName,
		"file_type":        up.FileType,
		"file_size_bytes":  up.FileSizeBytes,
		"is_inline":        false,
	})
}

// DELETE /notes/{id}/attachments/{attachmentId}
func (h *NoteHandler) DeleteAttachment(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	noteID := c.Param("id")
	if !validateNoteID(c, noteID) {
		return
	}
	attID := c.Param("attachmentId")
	if attID == "" {
		attID = c.Param("attachment_id")
	}
	if !validateUUIDParam(c, attID, "adjunto") {
		return
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
	if !validateNoteID(c, noteID) {
		return
	}
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
	if !validateNoteID(c, noteID) {
		return
	}
	// Rate limit anti-DoS/abuso de cuota híbrido: máximo 10 clonaciones por
	// minuto por usuario y máximo 100 clonaciones por minuto entre todos los
	// usuarios. Al superar cualquiera de los dos se responde 429 inmediatamente
	// sin tocar el servicio, con mensaje claro según el límite vulnerado.
	allowed, hitGlobal := h.copyLimiter.allow(userID)
	if !allowed {
		if hitGlobal {
			utils.RespondError(c, http.StatusTooManyRequests, utils.ErrRateLimited, "Límite global de clonación excedido, intente más tarde")
		} else {
			utils.RespondError(c, http.StatusTooManyRequests, utils.ErrRateLimited, "Límite de clonación excedido, intente más tarde")
		}
		return
	}
	idemKey := c.GetHeader("X-Idempotency-Key")
	newNote, err := h.svc.Copy(c.Request.Context(), userID, noteID, idemKey)
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
	if !validateNoteID(c, noteID) {
		return
	}
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
	if !validateNoteID(c, noteID) {
		return
	}
	if err := h.svc.Unlike(c.Request.Context(), userID, noteID); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// POST /notes/{id}/share
type shareRequest struct {
	GroupID    string `json:"group_id" binding:"required,uuid"`
	AccessMode string `json:"access_mode" binding:"required,oneof=link restricted"`
}

func (h *NoteHandler) Share(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	noteID := c.Param("id")
	if !validateNoteID(c, noteID) {
		return
	}
	var req shareRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1048576)
	if err := c.ShouldBindJSON(&req); err != nil {
		if isPayloadTooLarge(err) {
			utils.RespondError(c, http.StatusRequestEntityTooLarge, utils.ErrFileTooLarge, utils.MessageForCode(utils.ErrFileTooLarge))
			return
		}
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
	if !validateUUIDParam(c, sharedID, "compartición") {
		return
	}
	if err := h.svc.Unshare(c.Request.Context(), userID, sharedID); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// POST /notes/unshare-all
type unshareAllRequest struct {
	UserID  string `json:"user_id" binding:"required,uuid"`
	GroupID string `json:"group_id" binding:"required,uuid"`
}

func (h *NoteHandler) UnshareAll(c *gin.Context) {
	// FIX IDOR: endpoint protegido con JWT de usuario. Solo el propio usuario
	// o un admin del grupo puede desvincular a user_id de group_id. Sin esta
	// verificación cualquier usuario autenticado podía desvincular a terceros
	// (IDOR de escritura). Si el llamado es interno (microservicio Social) debe
	// usar un token de servicio distinto, no el JWT de usuario final.
	callerID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	var req unshareAllRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1048576)
	if err := c.ShouldBindJSON(&req); err != nil {
		if isPayloadTooLarge(err) {
			utils.RespondError(c, http.StatusRequestEntityTooLarge, utils.ErrFileTooLarge, utils.MessageForCode(utils.ErrFileTooLarge))
			return
		}
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "user_id y group_id requeridos")
		return
	}
	if callerID != req.UserID && !h.svc.IsGroupAdmin(c.Request.Context(), callerID, req.GroupID) {
		utils.RespondError(c, http.StatusForbidden, utils.ErrForbidden, "no autorizado")
		return
	}
	if err := h.svc.UnshareAll(c.Request.Context(), req.UserID, req.GroupID); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// GET /notes/{id}/access
//
// Responde 200 con el contrato de acceso: la autorización de aplicación se
// resuelve localmente (owner | public | link | restricted) y las banderas de
// Drive describen el estado remoto sin abortar la petición:
//   - drive_sync_status: convergencia de notes.drive_managed_permissions
//     (failed > pending > synced), calculada con GetAccessInfo;
//   - drive_connection_required: true sólo cuando el autor de una nota no pública
//     (access_mode=owner) no tiene conexión OAuth vigente para el archivo
//     (drive.OAuthError). La falta de conexión se informa como metadato, no como
//     403, para que el cliente pueda ofrecer reconectar Drive.
//   - drive_access_verified: true sólo cuando Drive confirma el acceso del autor
//     (VerifyFileAccess sin error). Para link, public y restricted ambas banderas
//     son false: el archivo se sirve con la autorización de la aplicación, no con
//     el token del solicitante, así que no se consulta Drive.
func (h *NoteHandler) GetAccess(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	noteID := c.Param("id")
	if !validateNoteID(c, noteID) {
		return
	}
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
	if !validateUUIDParam(c, groupID, "grupo") {
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
			"created_at": n.CreatedAt.UTC().Format(time.RFC3339), "updated_at": n.UpdatedAt.UTC().Format(time.RFC3339),
		}
		out = append(out, item)
	}
	c.JSON(http.StatusOK, gin.H{"notes": out, "next_cursor": next})
}

// handleServiceError traduce errores de dominio a HTTP canónico usando
// exclusivamente errores tipados (errors.Is / errors.As) y centinelas
// estructurados. No se inspeccionan mensajes con strings.Contains: así se
// evita mapear por casualidad y se garantiza que ninguna traza SQL ni detalle
// interno de Postgres/Drive llegue al cliente en el body de la respuesta.
func handleServiceError(c *gin.Context, err error) {
	// 1. OAuth tipado: *drive.OAuthError => 403 genérico sin filtrar detalles.
	if drive.IsOAuthError(err) {
		log.Printf("notes: acceso denegado Drive (OAuthError tipado): %v", err)
		utils.RespondError(c, http.StatusForbidden, utils.ErrForbidden, "Conecte o renueve su Google Drive")
		return
	}

	// 2. Errores de dominio con centinelas estructurados.
	var se *service.ServiceError
	if errors.As(err, &se) {
		switch {
		case errors.Is(err, service.ErrDriveUnavailable):
			log.Printf("notes: upstream de almacenamiento no disponible: %v", err)
			utils.RespondError(c, http.StatusBadGateway, utils.ErrInternal, "Servicio de almacenamiento no disponible")
		case errors.Is(err, service.ErrSourceAccessDenied):
			// Zero-knowledge absoluto: el fallo de acceso al archivo origen es
			// indistinguible de una nota inexistente (404 not_found, mismo
			// mensaje que notFoundNote del servicio). Nunca se filtra que el
			// recurso existe pero su origen no es legible.
			log.Printf("notes: acceso al archivo origen denegado (zero-knowledge): %v", err)
			utils.RespondError(c, http.StatusNotFound, utils.ErrNotFound, "Nota no encontrada")
		case errors.Is(err, service.ErrInternalDatabase), errors.Is(err, service.ErrInternalServer):
			// Detalle solo al log del servidor; el cliente recibe 500 genérico.
			log.Printf("notes: error interno: %v", err)
			utils.RespondError(c, http.StatusInternalServerError, utils.ErrInternal, "Error interno")
		default:
			utils.RespondError(c, utils.StatusForCode(se.Code), se.Code, se.Message)
		}
		return
	}

	// 3. Defensa en profundidad: *drive.DriveError crudo.
	var de *drive.DriveError
	if errors.As(err, &de) {
		switch {
		case de.Code == http.StatusForbidden || de.Code == http.StatusNotFound:
			log.Printf("notes: recurso Drive no disponible (code %d): %v", de.Code, err)
			utils.RespondError(c, http.StatusNotFound, utils.ErrNoteUnavailable, "Nota no disponible en almacenamiento remoto")
		case de.Code == http.StatusRequestEntityTooLarge:
			utils.RespondError(c, http.StatusRequestEntityTooLarge, utils.ErrFileTooLarge, utils.MessageForCode(utils.ErrFileTooLarge))
		default:
			log.Printf("notes: error de Drive no tipado por el servicio: %v", err)
			utils.RespondError(c, http.StatusBadGateway, utils.ErrInternal, "Servicio de almacenamiento no disponible")
		}
		return
	}

	// 4. Fallback seguro: nunca exponer el error crudo.
	log.Printf("notes: error interno no tipado: %v", err)
	utils.RespondError(c, http.StatusInternalServerError, utils.ErrInternal, "Error interno")
}
