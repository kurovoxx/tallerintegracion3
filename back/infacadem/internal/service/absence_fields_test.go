package service

import (
	"context"
	"testing"
)

// Campos nuevos 3_1_9: source/type en absences (CRUD + PATCH set/limpia).

func TestAbsence_Fields_CreateUpdateClear(t *testing.T) {
	ctx := context.Background()
	abs := newFakeAbsences(map[string]string{})
	svc := NewAbsenceService(abs)
	subID := newUUID()
	abs.subs[subID] = userA

	created, err := svc.Create(ctx, userA, subID, "2026-03-10", false, strPtr(" self "), strPtr("Teoría"))
	if err != nil {
		t.Fatal(err)
	}
	if created.Source == nil || *created.Source != "self" {
		t.Fatalf("source trim: %+v", created)
	}
	if created.Type == nil || *created.Type != "Teoría" {
		t.Fatalf("type: %+v", created)
	}

	// Vacíos = NULL.
	empty, err := svc.Create(ctx, userA, subID, "2026-03-11", false, strPtr("  "), strPtr(""))
	if err != nil {
		t.Fatal(err)
	}
	if empty.Source != nil || empty.Type != nil {
		t.Fatalf("vacíos NULL: %+v", empty)
	}

	// PATCH set.
	upd, err := svc.Update(ctx, userA, created.ID, AbsenceUpdate{Source: strPtr("teacher"), Type: strPtr("Lab")})
	if err != nil {
		t.Fatal(err)
	}
	if upd.Source == nil || *upd.Source != "teacher" || upd.Type == nil || *upd.Type != "Lab" {
		t.Fatalf("patch set: %+v", upd)
	}
	if !upd.Justified && created.Justified {
		t.Fatalf("patch conserva justified: %+v", upd)
	}

	// PATCH "" = limpiar (NULL), combinado con justified.
	j := true
	upd, err = svc.Update(ctx, userA, created.ID, AbsenceUpdate{Justified: &j, Source: strPtr(""), Type: strPtr("  ")})
	if err != nil {
		t.Fatal(err)
	}
	if !upd.Justified || upd.Source != nil || upd.Type != nil {
		t.Fatalf("patch clear: %+v", upd)
	}

	// Sin campos = 400.
	if _, err := svc.Update(ctx, userA, created.ID, AbsenceUpdate{}); err == nil {
		t.Fatal("vacío 400")
	} else {
		mustCode(t, err, "bad_request")
	}
	if _, err := svc.Update(ctx, "", created.ID, AbsenceUpdate{Source: strPtr("x")}); err == nil {
		t.Fatal("sin user 401")
	} else {
		mustCode(t, err, "unauthorized")
	}
	if _, err := svc.Update(ctx, userA, "x", AbsenceUpdate{Source: strPtr("x")}); err == nil {
		t.Fatal("uuid 400")
	} else {
		mustCode(t, err, "invalid_uuid")
	}

	// Wrapper SetJustified intacto.
	off := false
	upd, err = svc.SetJustified(ctx, userA, created.ID, &off)
	if err != nil || upd.Justified {
		t.Fatalf("wrapper: %+v %v", upd, err)
	}
}
