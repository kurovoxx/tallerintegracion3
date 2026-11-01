package http

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Campos nuevos 3_1_9 por HTTP: absences aceptan/devuelven source/type
// (null si ausentes, PATCH set/limpia).

func TestHandler_AbsenceFields_Roundtrip(t *testing.T) {
	f := newAttendanceFixture(t)
	tok := mintToken(t, testUserA, testSecret, time.Hour)
	mat := f.subjectA

	w := f.do(t, "POST", "/subjects/"+mat+"/absences",
		`{"date":"2026-03-10","source":"self","type":"Teoría"}`, tok)
	id := absenceID(t, w)

	w = f.do(t, "GET", "/absences/"+id, "", tok)
	if w.Code != 200 {
		t.Fatalf("get 200: %d %s", w.Code, w.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["source"] != "self" || got["type"] != "Teoría" {
		t.Fatalf("campos: %s", w.Body.String())
	}
	if got["date"] != "2026-03-10" {
		t.Fatalf("fecha: %s", w.Body.String())
	}

	// PATCH set + clear.
	w = f.do(t, "PATCH", "/absences/"+id, `{"justified":true,"source":"teacher","type":""}`, tok)
	if w.Code != 200 {
		t.Fatalf("patch 200: %d %s", w.Code, w.Body.String())
	}
	var got2 map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got2)
	if got2["justified"] != true || got2["source"] != "teacher" || got2["type"] != nil {
		t.Fatalf("patch: %s", w.Body.String())
	}

	// Sin campos → null explícito.
	w = f.do(t, "POST", "/subjects/"+mat+"/absences", `{"date":"2026-03-11"}`, tok)
	id2 := absenceID(t, w)
	w = f.do(t, "GET", "/absences/"+id2, "", tok)
	var got3 map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got3)
	if v, ok := got3["source"]; !ok || v != nil {
		t.Fatalf("source null: %s", w.Body.String())
	}
	if v, ok := got3["type"]; !ok || v != nil {
		t.Fatalf("type null: %s", w.Body.String())
	}

	// List los incluye.
	w = f.do(t, "GET", "/subjects/"+mat+"/absences", "", tok)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"source":"teacher"`) {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
}
