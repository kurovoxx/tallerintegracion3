package http

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestListHoursContractAndMembership(t *testing.T) {
	e := newTasksEnv()
	admin, member, stranger := uuid.NewString(), uuid.NewString(), uuid.NewString()
	groupID := uuid.NewString()
	e.sStore.AddGroup(groupID, admin, member)
	w := e.do(http.MethodPost, "/groups/"+groupID+"/sprint-sheet", admin, map[string]any{
		"title": "Horas", "assigned_to": member,
	})
	expect(t, w, http.StatusCreated)
	taskID := decode(t, w)["data"].(map[string]any)["id"].(string)
	path := "/sprint-sheet/" + taskID + "/hours"

	w = e.do(http.MethodGet, path, member, nil)
	expect(t, w, http.StatusOK)
	body := decode(t, w)
	if entries, ok := body["data"].([]any); !ok || len(entries) != 0 || body["total_hours"] != float64(0) {
		t.Fatalf("horas vacías deben devolver {data:[],total_hours:0}: %s", w.Body)
	}

	for _, entry := range []struct {
		date   string
		hours  float64
		status int
	}{
		{"2026-10-18", 2.5, http.StatusCreated},
		{"2026-10-18", 2, http.StatusOK},
		{"2026-10-19", 1.25, http.StatusCreated},
	} {
		expect(t, e.do(http.MethodPost, path, member, map[string]any{
			"log_date": entry.date, "hours": entry.hours,
		}), entry.status)
	}
	w = e.do(http.MethodGet, path, admin, nil)
	expect(t, w, http.StatusOK)
	body = decode(t, w)
	entries, ok := body["data"].([]any)
	if !ok || len(entries) != 2 || body["total_hours"] != 3.25 {
		t.Fatalf("la actualización debe reemplazar horas y recalcular el total: %s", w.Body)
	}
	wantHours := map[string]float64{"2026-10-18": 2, "2026-10-19": 1.25}
	for _, item := range entries {
		entry := item.(map[string]any)
		date, _ := entry["log_date"].(string)
		if entry["task_id"] != taskID || entry["id"] == "" || entry["hours"] != wantHours[date] {
			t.Fatalf("entrada de horas inválida: %v", entry)
		}
		delete(wantHours, date)
	}
	if len(wantHours) != 0 {
		t.Fatalf("faltan fechas de horas: %v", wantHours)
	}

	for _, tc := range []struct {
		name, path, user string
		status           int
	}{
		{"sin JWT", path, "", http.StatusUnauthorized},
		{"no miembro", path, stranger, http.StatusForbidden},
		{"UUID inválido", "/sprint-sheet/basura/hours", member, http.StatusBadRequest},
		{"tarea inexistente", "/sprint-sheet/" + uuid.NewString() + "/hours", member, http.StatusNotFound},
		{"from inválido", path + "?from=ayer", member, http.StatusBadRequest},
		{"to inválido", path + "?to=2026-02-30", member, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expect(t, e.do(http.MethodGet, tc.path, tc.user, nil), tc.status)
		})
	}
}
