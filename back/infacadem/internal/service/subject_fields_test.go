package service

import (
	"context"
	"testing"
)

// Campos nuevos 3_1_8: code/professor/room en subjects (CRUD + PATCH
// set/limpia) y modality en schedules (alta + NULL).

func TestSubject_Fields_CreateUpdateClear(t *testing.T) {
	ctx := context.Background()
	_, subSvc, _, cur, _, _ := newFixture()
	malla, err := NewCurriculumService(cur).Create(ctx, userA, "M")
	if err != nil {
		t.Fatal(err)
	}

	created, err := subSvc.Create(ctx, userA, malla.ID, SubjectInput{
		Name: strPtr("Álgebra"), Code: strPtr("  MAT-101 "),
		Professor: strPtr("M. Soto"), Room: strPtr("CJP11-204"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Code == nil || *created.Code != "MAT-101" {
		t.Fatalf("code trim: %+v", created)
	}
	if created.Professor == nil || *created.Professor != "M. Soto" {
		t.Fatalf("professor: %+v", created)
	}
	if created.Room == nil || *created.Room != "CJP11-204" {
		t.Fatalf("room: %+v", created)
	}

	// Vacíos en alta = NULL, no error.
	empty, err := subSvc.Create(ctx, userA, malla.ID, SubjectInput{
		Name: strPtr("Sin datos"), Code: strPtr("   "),
		Professor: strPtr(""), Room: strPtr(""),
	})
	if err != nil {
		t.Fatal(err)
	}
	if empty.Code != nil || empty.Professor != nil || empty.Room != nil {
		t.Fatalf("vacíos NULL: %+v", empty)
	}

	// PATCH set.
	upd, err := subSvc.Update(ctx, userA, created.ID, SubjectInput{
		Code: strPtr("MAT-102"), Room: strPtr("Lab-3"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if upd.Code == nil || *upd.Code != "MAT-102" || upd.Room == nil || *upd.Room != "Lab-3" {
		t.Fatalf("patch set: %+v", upd)
	}
	if upd.Professor == nil || *upd.Professor != "M. Soto" {
		t.Fatalf("patch conserva resto: %+v", upd)
	}

	// PATCH "" = limpiar (NULL).
	upd, err = subSvc.Update(ctx, userA, created.ID, SubjectInput{Code: strPtr("  ")})
	if err != nil {
		t.Fatal(err)
	}
	if upd.Code != nil {
		t.Fatalf("patch clear: %+v", upd)
	}

	// Solo-clear no es "sin campos".
	upd, err = subSvc.Update(ctx, userA, created.ID, SubjectInput{Professor: strPtr(""), Room: strPtr("")})
	if err != nil {
		t.Fatal(err)
	}
	if upd.Professor != nil || upd.Room != nil {
		t.Fatalf("patch clear múltiple: %+v", upd)
	}
}

func TestSchedule_Modality_Create(t *testing.T) {
	ctx := context.Background()
	_, subSvc, schSvc, cur, _, _ := newFixture()
	malla, _ := NewCurriculumService(cur).Create(ctx, userA, "M")
	mat, _ := subSvc.Create(ctx, userA, malla.ID, SubjectInput{Name: strPtr("S")})

	mod := "  Cátedra "
	s, err := schSvc.Create(ctx, userA, mat.ID, 1, "08:00", "09:30", &mod)
	if err != nil {
		t.Fatal(err)
	}
	if s.Modality == nil || *s.Modality != "Cátedra" {
		t.Fatalf("modality trim: %+v", s)
	}

	// Vacía/nil = NULL.
	for _, m := range []*string{nil, strPtr(""), strPtr("   ")} {
		s, err := schSvc.Create(ctx, userA, mat.ID, 2, "10:00", "11:30", m)
		if err != nil {
			t.Fatal(err)
		}
		if s.Modality != nil {
			t.Fatalf("modality NULL: %+v", s)
		}
	}

	// List la devuelve.
	list, err := schSvc.List(ctx, userA, mat.ID)
	if err != nil || len(list) != 4 {
		t.Fatalf("list: %v %d", err, len(list))
	}
	found := false
	for _, h := range list {
		if h.Modality != nil && *h.Modality == "Cátedra" {
			found = true
		}
	}
	if !found {
		t.Fatalf("modality en list: %+v", list)
	}
}
