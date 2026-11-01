package http

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Campos nuevos 3_1_8 por HTTP: subjects aceptan/devuelven
// code/professor/room (null si ausentes, PATCH set/limpia) y schedules
// aceptan/devuelven modality.

func TestHandler_SubjectFields_Roundtrip(t *testing.T) {
	f := newHTTPFixture()
	tok := mintToken(t, testUserA, testSecret, time.Hour)

	w := f.do(t, "POST", "/curricula", `{"name":"M"}`, tok)
	if w.Code != 201 {
		t.Fatalf("malla 201: %d %s", w.Code, w.Body.String())
	}
	malla := decodeID(t, w)

	w = f.do(t, "POST", "/curricula/"+malla+"/subjects",
		`{"name":"Álgebra","code":"MAT-101","professor":"M. Soto","room":"CJP11-204","credits":5}`, tok)
	if w.Code != 201 {
		t.Fatalf("subject 201: %d %s", w.Code, w.Body.String())
	}
	id := decodeID(t, w)

	w = f.do(t, "GET", "/subjects/"+id, "", tok)
	if w.Code != 200 {
		t.Fatalf("get 200: %d %s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["code"] != "MAT-101" || got["professor"] != "M. Soto" || got["room"] != "CJP11-204" {
		t.Fatalf("campos: %s", w.Body.String())
	}

	// PATCH parcial los edita.
	w = f.do(t, "PATCH", "/subjects/"+id, `{"code":"MAT-102","room":""}`, tok)
	if w.Code != 200 {
		t.Fatalf("patch 200: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"code":"MAT-102"`) {
		t.Fatalf("patch set: %s", w.Body.String())
	}

	// "" limpia (null), resto intacto.
	w = f.do(t, "GET", "/subjects/"+id, "", tok)
	var got2 map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got2)
	if got2["code"] != "MAT-102" || got2["room"] != nil {
		t.Fatalf("patch clear: %s", w.Body.String())
	}
	if got2["professor"] != "M. Soto" {
		t.Fatalf("patch conserva: %s", w.Body.String())
	}

	// Sin campos → null explícito, sin romper.
	w = f.do(t, "POST", "/curricula/"+malla+"/subjects", `{"name":"Plana"}`, tok)
	id2 := decodeID(t, w)
	w = f.do(t, "GET", "/subjects/"+id2, "", tok)
	var got3 map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got3)
	for _, k := range []string{"code", "professor", "room"} {
		if v, ok := got3[k]; !ok || v != nil {
			t.Fatalf("%s null: %s", k, w.Body.String())
		}
	}

	// List los incluye.
	w = f.do(t, "GET", "/curricula/"+malla+"/subjects", "", tok)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"code":"MAT-102"`) {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
}

func TestHandler_ScheduleModality_Roundtrip(t *testing.T) {
	f := newHTTPFixture()
	tok := mintToken(t, testUserA, testSecret, time.Hour)

	w := f.do(t, "POST", "/curricula", `{"name":"M"}`, tok)
	malla := decodeID(t, w)
	w = f.do(t, "POST", "/curricula/"+malla+"/subjects", `{"name":"S"}`, tok)
	mat := decodeID(t, w)

	w = f.do(t, "POST", "/subjects/"+mat+"/schedules",
		`{"day_of_week":1,"start_time":"10:00","end_time":"11:30","modality":"Cátedra"}`, tok)
	if w.Code != 201 {
		t.Fatalf("schedule 201: %d %s", w.Code, w.Body.String())
	}
	w = f.do(t, "POST", "/subjects/"+mat+"/schedules",
		`{"day_of_week":3,"start_time":"10:00","end_time":"11:30"}`, tok)
	if w.Code != 201 {
		t.Fatalf("schedule sin modality 201: %d %s", w.Code, w.Body.String())
	}

	w = f.do(t, "GET", "/subjects/"+mat+"/schedules", "", tok)
	if w.Code != 200 {
		t.Fatalf("list 200: %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"modality":"Cátedra"`) {
		t.Fatalf("modality en list: %s", body)
	}
	if !strings.Contains(body, `"modality":null`) {
		t.Fatalf("modality null en list: %s", body)
	}
}
