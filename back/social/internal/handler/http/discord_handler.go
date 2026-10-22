package http

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

// Constantes de Discord OAuth2 para conexión automática (tarea 3_5_18).
// Estas se inyectan desde el entorno en main.go.
var (
	discordClientID     string
	discordClientSecret string
	discordRedirectURI  string
)

// SetDiscordOAuthConfig configura las credenciales de Discord OAuth2.
// Se llama desde main.go al iniciar el servidor.
func SetDiscordOAuthConfig(clientID, clientSecret, redirectURI string) {
	discordClientID = clientID
	discordClientSecret = clientSecret
	discordRedirectURI = redirectURI
}

// discordState almacena temporalmente el estado de autenticación OAuth2
// para prevenir ataques CSRF. Mapea state -> {groupID, userID}.
var discordState = make(map[string]struct {
	GroupID  string
	UserID   string
	Created  time.Time
})
var discordStateMu sync.RWMutex

// init limpia estados expirados cada 5 minutos.
func init() {
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			discordStateMu.Lock()
			for k, v := range discordState {
				if time.Since(v.Created) > 10*time.Minute {
					delete(discordState, k)
				}
			}
			discordStateMu.Unlock()
		}
	}()
}

// generateState crea un estado aleatorio para OAuth2.
func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// stateFor guarda el estado asociado a un grupo y usuario.
func stateFor(groupID, userID string) (string, error) {
	state, err := generateState()
	if err != nil {
		return "", err
	}
	discordStateMu.Lock()
	discordState[state] = struct {
		GroupID string
		UserID  string
		Created time.Time
	}{groupID, userID, time.Now()}
	discordStateMu.Unlock()
	return state, nil
}

// popState recupera y elimina un estado.
func popState(state string) (groupID, userID string, ok bool) {
	discordStateMu.Lock()
	defer discordStateMu.Unlock()
	v, ok := discordState[state]
	if ok {
		delete(discordState, state)
	}
	return v.GroupID, v.UserID, ok
}

type DiscordHandler struct {
	svc *service.DiscordService
}

func NewDiscordHandler(svc *service.DiscordService) *DiscordHandler {
	return &DiscordHandler{svc: svc}
}

// RegisterRoutes registra las rutas de Discord en un router ya protegido por
// el middleware de autenticación. main.go y los tests comparten esta lista.
func (h *DiscordHandler) RegisterRoutes(r gin.IRoutes) {
	r.GET("/groups/:id/discord-config", h.GetConfig)
	r.PUT("/groups/:id/discord-config", h.PutConfig)
}

// RegisterAuthRoutes registra las rutas de callback de Discord OAuth2.
// Estas rutas NO requieren JWT de usuario (son llamadas por Discord).
func (h *DiscordHandler) RegisterAuthRoutes(r gin.IRoutes) {
	r.GET("/auth/discord/callback", h.DiscordCallback)
}

// ConnectDiscord maneja GET /groups/:id/discord/connect.
// Redirige a Discord para autorizar la conexión OAuth2.
// Requiere JWT de usuario.
func (h *DiscordHandler) ConnectDiscord(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("id"))
	if groupID == "" {
		fail(c, "ConnectDiscord", http.StatusBadRequest, "invalid_group_id", "group ID is required")
		return
	}
	if _, err := uuid.Parse(groupID); err != nil {
		fail(c, "ConnectDiscord", http.StatusBadRequest, "invalid_group_id", "invalid group ID format")
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok {
		fail(c, "ConnectDiscord", http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	state, err := stateFor(groupID, userID)
	if err != nil {
		fail(c, "ConnectDiscord", http.StatusInternalServerError, "internal", "could not generate state")
		return
	}

	params := url.Values{}
	params.Set("client_id", discordClientID)
	params.Set("redirect_uri", discordRedirectURI)
	params.Set("response_type", "code")
	params.Set("scope", "guilds webhook.incoming")
	params.Set("state", state)

	authURL := "https://discord.com/api/oauth2/authorize?" + params.Encode()
	c.JSON(http.StatusOK, gin.H{
		"redirect_url": authURL,
		"message":      "Redirect to Discord for authorization",
	})
}

// DiscordCallback maneja GET /auth/discord/callback.
// Es llamado por Discord tras la autorización del usuario.
// No requiere JWT de usuario (es llamado por el navegador tras el OAuth de Discord).
func (h *DiscordHandler) DiscordCallback(c *gin.Context) {
	code := strings.TrimSpace(c.Query("code"))
	state := strings.TrimSpace(c.Query("state"))
	errorParam := strings.TrimSpace(c.Query("error"))

	if errorParam != "" {
		c.Redirect(http.StatusTemporaryRedirect, "/groups/error?error="+url.QueryEscape(errorParam))
		return
	}
	if code == "" || state == "" {
		c.Redirect(http.StatusTemporaryRedirect, "/groups/error?error=missing_code_or_state")
		return
	}

	groupID, userID, ok := popState(state)
	if !ok {
		c.Redirect(http.StatusTemporaryRedirect, "/groups/error?error=invalid_or_expired_state")
		return
	}

	ctx := c.Request.Context()
	if _, err := h.svc.ConnectDiscord(ctx, groupID, userID, code); err != nil {
		log.Printf("DiscordCallback error: %v", err)
		c.Redirect(http.StatusTemporaryRedirect, "/groups/error?error="+url.QueryEscape(err.Error()))
		return
	}

	c.Redirect(http.StatusTemporaryRedirect, "/groups/"+groupID+"/discord-config?connected=1")
}

// GetConfig maneja GET /groups/{id}/discord-config.
// Solo miembros. 200 {server_name, invite_url, webhook_url?};
// 404 discord_not_configured si nunca se configuró.
func (h *DiscordHandler) GetConfig(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("id"))
	if groupID == "" {
		fail(c, "GetConfig", http.StatusBadRequest, "invalid_group_id", "group ID is required")
		return
	}
	if _, err := uuid.Parse(groupID); err != nil {
		fail(c, "GetConfig", http.StatusBadRequest, "invalid_group_id", "invalid group ID format")
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok {
		fail(c, "GetConfig", http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	cfg, err := h.svc.GetConfig(c.Request.Context(), groupID, userID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrGroupNotFound):
			fail(c, "GetConfig", http.StatusNotFound, "group_not_found", "group not found")
		case errors.Is(err, service.ErrForbidden):
			fail(c, "GetConfig", http.StatusForbidden, "forbidden", "you must be a member of the group")
		case errors.Is(err, service.ErrDiscordNotConfigured):
			fail(c, "GetConfig", http.StatusNotFound, "discord_not_configured", "discord not configured for this group")
		case errors.Is(err, service.ErrInvalidGroupID):
			fail(c, "GetConfig", http.StatusBadRequest, "invalid_group_id", err.Error())
		case errors.Is(err, service.ErrInvalidUserID):
			fail(c, "GetConfig", http.StatusBadRequest, "invalid_user_id", err.Error())
		default:
			log.Printf("GetDiscordConfig internal error: %v", err)
			fail(c, "GetConfig", http.StatusInternalServerError, "internal", "could not load discord config")
		}
		return
	}

	logOp(c, "PutDiscordConfig", http.StatusOK, "group_id="+groupID)
	c.JSON(http.StatusOK, gin.H{
		"server_name": pgTextToString(cfg.ServerName),
		"invite_url":  pgTextToString(cfg.InviteUrl),
		"webhook_url": pgTextToNil(cfg.WebhookUrl),
	})
}

// PUT /groups/{id}/discord-config — contrato: agentApiContract.md sección 5
// Body: {server_name, invite_url, webhook_url?} → 200. Solo admin.
type PutDiscordConfigRequest struct {
	ServerName string `json:"server_name" binding:"required,max=200"`
	InviteURL  string `json:"invite_url" binding:"required,max=500"`
	WebhookURL string `json:"webhook_url" binding:"max=500"`
}

func (h *DiscordHandler) PutConfig(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("id"))
	if groupID == "" {
		fail(c, "PutConfig", http.StatusBadRequest, "invalid_group_id", "group ID is required")
		return
	}
	if _, err := uuid.Parse(groupID); err != nil {
		fail(c, "PutConfig", http.StatusBadRequest, "invalid_group_id", "invalid group ID format")
		return
	}

	var req PutDiscordConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "PutConfig", http.StatusBadRequest, "invalid_body", err.Error())
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok {
		fail(c, "PutConfig", http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	cfg, err := h.svc.PutConfig(
		c.Request.Context(),
		groupID,
		userID,
		req.ServerName,
		req.InviteURL,
		req.WebhookURL,
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrGroupNotFound):
			fail(c, "PutConfig", http.StatusNotFound, "group_not_found", "group not found")
		case errors.Is(err, service.ErrForbidden):
			fail(c, "PutConfig", http.StatusForbidden, "forbidden", "only admins can configure discord")
		case errors.Is(err, service.ErrInvalidGroupID):
			fail(c, "PutConfig", http.StatusBadRequest, "invalid_group_id", err.Error())
		case errors.Is(err, service.ErrInvalidUserID):
			fail(c, "PutConfig", http.StatusBadRequest, "invalid_user_id", err.Error())
		case errors.Is(err, service.ErrServerNameEmpty),
			errors.Is(err, service.ErrInviteURLEmpty),
			errors.Is(err, service.ErrInvalidWebhookURL):
			fail(c, "PutConfig", http.StatusBadRequest, "invalid_body", err.Error())
		default:
			log.Printf("PutDiscordConfig internal error: %v", err)
			fail(c, "PutConfig", http.StatusInternalServerError, "internal", "could not save discord config")
		}
		return
	}

	logOp(c, "GetDiscordConfig", http.StatusOK, "group_id="+groupID)
	c.JSON(http.StatusOK, gin.H{
		"message":     "discord config saved successfully",
		"server_name": pgTextToString(cfg.ServerName),
		"invite_url":  pgTextToString(cfg.InviteUrl),
		"webhook_url": pgTextToNil(cfg.WebhookUrl),
	})
}

func pgTextToString(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}

func pgTextToNil(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	s := t.String
	return &s
}
