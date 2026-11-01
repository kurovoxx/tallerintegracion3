package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/utils"
)

// ContentsHandler maneja contenidos (unidades) de una materia propia.
type ContentsHandler struct {
	svc *service.ContentService
}

func NewContentsHandler(svc *service.ContentService) *ContentsHandler {
	return &ContentsHandler{svc: svc}
}

type contentRequest struct {
	Title    *string `json:"title"`
	Body     *string `json:"body"`
	Position *int    `json:"position"`
	Done     *bool   `json:"done"`
}

func toContentInput(req contentRequest) service.ContentInput {
	return service.ContentInput{
		Title: req.Title, Body: req.Body, Position: req.Position, Done: req.Done,
	}
}

// Create maneja POST /subjects/:id/contents → 201 {id}.
// 404 si la materia no existe o no es propia.
func (h *ContentsHandler) Create(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "CreateContent", started)
	if !ok {
		return
	}
	var req contentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "Request inválido")
		logAcademicOp(c, "CreateContent", http.StatusBadRequest, userID, started, "code="+utils.ErrBadRequest)
		return
	}
	out, err := h.svc.Create(c.Request.Context(), userID, c.Param("id"), toContentInput(req))
	if err != nil {
		respondServiceError(c, "CreateContent", userID, started, err)
		return
	}
	logAcademicOp(c, "CreateContent", http.StatusCreated, userID, started, "")
	c.JSON(http.StatusCreated, gin.H{"id": out.ID})
}

// ListBySubject maneja GET /subjects/:id/contents → 200 {items} por posición.
func (h *ContentsHandler) ListBySubject(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "ListContents", started)
	if !ok {
		return
	}
	out, err := h.svc.List(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		respondServiceError(c, "ListContents", userID, started, err)
		return
	}
	logAcademicOp(c, "ListContents", http.StatusOK, userID, started, "")
	c.JSON(http.StatusOK, gin.H{"items": out})
}

// Get maneja GET /contents/:id → 200.
func (h *ContentsHandler) Get(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "GetContent", started)
	if !ok {
		return
	}
	out, err := h.svc.Get(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		respondServiceError(c, "GetContent", userID, started, err)
		return
	}
	logAcademicOp(c, "GetContent", http.StatusOK, userID, started, "")
	c.JSON(http.StatusOK, out)
}

// Update maneja PATCH /contents/:id → 200.
func (h *ContentsHandler) Update(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "UpdateContent", started)
	if !ok {
		return
	}
	var req contentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, utils.ErrBadRequest, "Request inválido")
		logAcademicOp(c, "UpdateContent", http.StatusBadRequest, userID, started, "code="+utils.ErrBadRequest)
		return
	}
	out, err := h.svc.Update(c.Request.Context(), userID, c.Param("id"), toContentInput(req))
	if err != nil {
		respondServiceError(c, "UpdateContent", userID, started, err)
		return
	}
	logAcademicOp(c, "UpdateContent", http.StatusOK, userID, started, "")
	c.JSON(http.StatusOK, out)
}

// Delete maneja DELETE /contents/:id → 204.
func (h *ContentsHandler) Delete(c *gin.Context) {
	started := time.Now()
	userID, ok := requireUserID(c, "DeleteContent", started)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), userID, c.Param("id")); err != nil {
		respondServiceError(c, "DeleteContent", userID, started, err)
		return
	}
	logAcademicOp(c, "DeleteContent", http.StatusNoContent, userID, started, "")
	c.AbortWithStatus(http.StatusNoContent)
}
