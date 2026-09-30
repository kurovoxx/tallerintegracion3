package service

import (
	"context"
	"errors"
	"testing"
)

type fakeUserDirectory struct {
	data map[string]UserPublic
	err  error
}

func (f *fakeUserDirectory) LookupUsers(ctx context.Context, ids []string) (map[string]UserPublic, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := map[string]UserPublic{}
	for _, id := range ids {
		if p, ok := f.data[id]; ok {
			out[id] = p
		}
	}
	return out, nil
}

func TestListMembersEnriched_ConNombres(t *testing.T) {
	svc, store := newTestSvc()
	g := newGroup(t, svc, uAdmin)
	joinAs(t, svc, store, g.ID, uMember)
	name := "Miembro Uno"
	svc.SetUserDirectory(&fakeUserDirectory{data: map[string]UserPublic{
		uMember: {UserID: uMember, Email: "member@example.com", DisplayName: &name},
	}})

	ms, err := svc.ListMembersEnriched(ctx, g.ID, uAdmin)
	if err != nil {
		t.Fatalf("enriquecido: %v", err)
	}
	if len(ms) != 2 {
		t.Fatalf("esperado 2 miembros, got %d", len(ms))
	}
	found := false
	for _, m := range ms {
		if m.UserID == uMember {
			found = true
			if m.DisplayName == nil || *m.DisplayName != "Miembro Uno" {
				t.Fatalf("display_name no enriquecido: %+v", m)
			}
			if m.Email == nil || *m.Email != "member@example.com" {
				t.Fatalf("email no enriquecido: %+v", m)
			}
		}
	}
	if !found {
		t.Fatalf("miembro no encontrado en %+v", ms)
	}
}

func TestListMembersEnriched_DirectorioCaido_Degrada(t *testing.T) {
	svc, _ := newTestSvc()
	g := newGroup(t, svc, uAdmin)
	svc.SetUserDirectory(&fakeUserDirectory{err: errors.New("auth caído")})

	ms, err := svc.ListMembersEnriched(ctx, g.ID, uAdmin)
	if err != nil {
		t.Fatalf("fallo de directorio no debe romper: %v", err)
	}
	if len(ms) != 1 || ms[0].DisplayName != nil {
		t.Fatalf("debe degradar a sin nombres: %+v", ms)
	}
}

func TestListMembersEnriched_SinDirectorio_Compatible(t *testing.T) {
	svc, _ := newTestSvc()
	g := newGroup(t, svc, uAdmin)

	ms, err := svc.ListMembersEnriched(ctx, g.ID, uAdmin)
	if err != nil {
		t.Fatalf("sin directorio: %v", err)
	}
	if len(ms) != 1 || ms[0].UserID != uAdmin {
		t.Fatalf("respuesta base intacta: %+v", ms)
	}
}
