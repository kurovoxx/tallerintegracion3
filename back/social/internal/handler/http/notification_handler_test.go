package http

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

// Repo falso: respeta orden entregado y read idempotente (ya leída → false).
type fakeNotifRepo struct {
	items []model.MeetingNotificationItem
}

func (f *fakeNotifRepo) ListNotificationsByUser(_ context.Context, _ string) ([]model.MeetingNotificationItem, error) {
	return f.items, nil
}

func (f *fakeNotifRepo) MarkNotificationRead(_ context.Context, id, _ string) (bool, error) {
	for i, it := range f.items {
		if it.ID == id {
			if it.ReadAt != nil {
				return false, nil
			}
			now := time.Now()
			f.items[i].ReadAt = &now
			return true, nil
		}
	}
	return false, nil
}

type notifEnv struct {
	router *gin.Engine
}

func newNotifEnv(items []model.MeetingNotificationItem, mStore *service.MemoryMeetingStore) *notifEnv {
	gin.SetMode(gin.TestMode)
	notifH := NewNotificationHandler(service.NewNotificationService(&fakeNotifRepo{items: items}))
	meetingH := NewMeetingHandler(service.NewMeetingService(mStore, service.NoopMeetingCreatedNotifier{}))
	r := gin.New()
	protected := r.Group("")
	protected.Use(middleware.NewAuthMiddleware(stubValidator{}).RequireAuth())
	protected.POST("/groups/:id/meetings", meetingH.CreateMeeting)
	protected.GET("/groups/:id/meetings", meetingH.ListMeetings)
	protected.GET("/notifications/me", notifH.ListMine)
	protected.POST("/notifications/:id/read", notifH.MarkRead)
	return &notifEnv{router: r}
}

func notifItem(id, meetingID string, read bool) model.MeetingNotificationItem {
	it := model.MeetingNotificationItem{
		ID: id, MeetingID: meetingID, GroupID: uuid.NewString(),
		Title: "Daily", ScheduledAt: time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC),
		CreatedAt: time.Now(),
	}
	if read {
		t := time.Now()
		it.ReadAt = &t
	}
	return it
}

func TestNotifications_ListMine_Unit(t *testing.T) {
	n1, n2 := uuid.NewString(), uuid.NewString()
	e := newNotifEnv([]model.MeetingNotificationItem{notifItem(n1, uuid.NewString(), false), notifItem(n2, uuid.NewString(), true)}, service.NewMemoryMeetingStore())
	user := uuid.NewString()

	w := doMartinReq(e.router, http.MethodGet, "/notifications/me", user, nil)
	expect(t, w, http.StatusOK)
	m := decode(t, w)
	list, _ := m["data"].([]any)
	if len(list) != 2 {
		t.Fatalf("se esperaban 2, got %s", w.Body.String())
	}
	first := list[0].(map[string]any)
	if first["id"] != n1 || first["read"] != false {
		t.Fatalf("no leída primero con forma completa, got %v", first)
	}
	for _, k := range []string{"meeting_id", "group_id", "title", "scheduled_at", "created_at"} {
		if _, ok := first[k]; !ok {
			t.Fatalf("falta campo %s en %v", k, first)
		}
	}

	expect(t, doMartinReq(e.router, http.MethodGet, "/notifications/me", "", nil), http.StatusUnauthorized)
}

func TestNotifications_MarkRead_Unit(t *testing.T) {
	n1 := uuid.NewString()
	e := newNotifEnv([]model.MeetingNotificationItem{notifItem(n1, uuid.NewString(), false)}, service.NewMemoryMeetingStore())
	user := uuid.NewString()

	w := doMartinReq(e.router, http.MethodPost, "/notifications/"+n1+"/read", user, nil)
	expect(t, w, http.StatusOK)

	// Ya leída → 404 (sin filtrar); inexistente → 404.
	expect(t, doMartinReq(e.router, http.MethodPost, "/notifications/"+n1+"/read", user, nil), http.StatusNotFound)
	expect(t, doMartinReq(e.router, http.MethodPost, "/notifications/"+uuid.NewString()+"/read", user, nil), http.StatusNotFound)
	expect(t, doMartinReq(e.router, http.MethodPost, "/notifications/"+n1+"/read", "", nil), http.StatusUnauthorized)
}

func TestMeetings_List_Unit(t *testing.T) {
	mStore := service.NewMemoryMeetingStore()
	e := newNotifEnv(nil, mStore)
	admin, member, stranger := uuid.NewString(), uuid.NewString(), uuid.NewString()
	gid := uuid.NewString()
	mStore.AddGroup(gid, admin, member)

	w := doMartinReq(e.router, http.MethodPost, "/groups/"+gid+"/meetings", admin, map[string]any{
		"title": "Futura", "scheduled_at": "2026-11-20T15:00:00Z",
	})
	expect(t, w, http.StatusCreated)
	w = doMartinReq(e.router, http.MethodPost, "/groups/"+gid+"/meetings", admin, map[string]any{
		"title": "Pasada", "scheduled_at": "2020-01-01T15:00:00Z",
	})
	expect(t, w, http.StatusCreated)

	// Solo la futura (scheduled_at >= now), con forma completa.
	w = doMartinReq(e.router, http.MethodGet, "/groups/"+gid+"/meetings", member, nil)
	expect(t, w, http.StatusOK)
	list, _ := decode(t, w)["data"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["title"] != "Futura" {
		t.Fatalf("solo futura esperada, got %s", w.Body.String())
	}

	expect(t, doMartinReq(e.router, http.MethodGet, "/groups/"+gid+"/meetings", stranger, nil), http.StatusForbidden)
	expect(t, doMartinReq(e.router, http.MethodGet, "/groups/"+gid+"/meetings", "", nil), http.StatusUnauthorized)
	expect(t, doMartinReq(e.router, http.MethodGet, "/groups/"+uuid.NewString()+"/meetings", member, nil), http.StatusNotFound)
}
