package service

import (
	"context"
	"strings"

	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/utils"
)

// SubjectStore es el contrato que el service espera del repository.
type SubjectStore interface {
	Create(ctx context.Context, userID, curriculumID string, in repository.SubjectCreate) (*model.Subject, error)
	GetByID(ctx context.Context, id, userID string) (*model.Subject, error)
	ListByCurriculum(ctx context.Context, curriculumID, userID string) ([]model.Subject, bool, error)
	Update(ctx context.Context, id, userID string, p repository.SubjectPatch) (*model.Subject, error)
	Delete(ctx context.Context, id, userID string) (bool, error)
}

// SubjectInput agrupa el alta/edición de materias (nil = omitido).
// Semester/Credits/TotalClasses/Attendance no tienen validación extra:
// se persisten tal cual (NULL si nil).
// Code/Professor/Room son varchar libres: nil = omitido, "" = limpiar (NULL).
type SubjectInput struct {
	Name                 *string
	Code                 *string
	Professor            *string
	Room                 *string
	Semester             *int
	Credits              *int
	TotalClasses         *int
	MinAttendancePct     *float64
	AverageDecimalPlaces *int
	Status               *string
}

// SubjectService contiene las reglas de subjects (propiedad vía su malla).
type SubjectService struct {
	repo SubjectStore
}

func NewSubjectService(repo SubjectStore) *SubjectService {
	return &SubjectService{repo: repo}
}

func cleanDecimal(v *int) (int, error) {
	if v == nil {
		return 2, nil // default espejo del DDL (DEFAULT 2)
	}
	if *v < 0 || *v > 2 {
		return 0, NewServiceError(utils.ErrInvalidDecimal)
	}
	return *v, nil
}

func cleanStatus(v *string) (string, error) {
	if v == nil {
		return model.SubjectStatusInProgress, nil // default espejo del DDL
	}
	s := strings.TrimSpace(*v)
	if !model.IsValidSubjectStatus(s) {
		return "", NewServiceError(utils.ErrInvalidStatus)
	}
	return s, nil
}

// cleanSubjectText normaliza un varchar opcional (code/professor/room).
// Retorna (valor, limpiar): nil = omitido (sin cambios); "" tras trim =
// limpiar (NULL); resto = valor con trim aplicado.
func cleanSubjectText(v *string) (val *string, clear bool) {
	if v == nil {
		return nil, false
	}
	t := strings.TrimSpace(*v)
	if t == "" {
		return nil, true
	}
	return &t, false
}

// Create crea una materia en una malla propia; 404 si la malla no existe o es ajena.
func (s *SubjectService) Create(ctx context.Context, userID, curriculumID string, in SubjectInput) (*model.Subject, error) {
	if err := requireUser(userID); err != nil {
		return nil, err
	}
	curriculumID, err := ValidateUUID(curriculumID)
	if err != nil {
		return nil, err
	}
	if in.Name == nil {
		return nil, NewServiceError(utils.ErrInvalidName)
	}
	name, err := ValidateName(*in.Name)
	if err != nil {
		return nil, err
	}
	dec, err := cleanDecimal(in.AverageDecimalPlaces)
	if err != nil {
		return nil, err
	}
	status, err := cleanStatus(in.Status)
	if err != nil {
		return nil, err
	}
	code, _ := cleanSubjectText(in.Code)
	professor, _ := cleanSubjectText(in.Professor)
	room, _ := cleanSubjectText(in.Room)
	out, err := s.repo.Create(ctx, strings.TrimSpace(userID), curriculumID, repository.SubjectCreate{
		Name:                 name,
		Code:                 code,
		Professor:            professor,
		Room:                 room,
		Semester:             in.Semester,
		Credits:              in.Credits,
		TotalClasses:         in.TotalClasses,
		MinAttendancePct:     in.MinAttendancePct,
		AverageDecimalPlaces: dec,
		Status:               status,
	})
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, NewServiceError(utils.ErrNotFound)
	}
	return out, nil
}

// Get retorna una materia propia; 404 si no existe o su malla es ajena.
func (s *SubjectService) Get(ctx context.Context, userID, id string) (*model.Subject, error) {
	if err := requireUser(userID); err != nil {
		return nil, err
	}
	id, err := ValidateUUID(id)
	if err != nil {
		return nil, err
	}
	out, err := s.repo.GetByID(ctx, id, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, NewServiceError(utils.ErrNotFound)
	}
	return out, nil
}

// List lista las materias de una malla propia; 404 si la malla no existe o es ajena.
func (s *SubjectService) List(ctx context.Context, userID, curriculumID string) ([]model.Subject, error) {
	if err := requireUser(userID); err != nil {
		return nil, err
	}
	curriculumID, err := ValidateUUID(curriculumID)
	if err != nil {
		return nil, err
	}
	out, owned, err := s.repo.ListByCurriculum(ctx, curriculumID, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	if !owned {
		return nil, NewServiceError(utils.ErrNotFound)
	}
	if out == nil {
		out = []model.Subject{}
	}
	return out, nil
}

// Update aplica PATCH parcial; 400 si no hay campos o alguno es inválido,
// 404 si no existe o su malla es ajena.
func (s *SubjectService) Update(ctx context.Context, userID, id string, in SubjectInput) (*model.Subject, error) {
	if err := requireUser(userID); err != nil {
		return nil, err
	}
	id, err := ValidateUUID(id)
	if err != nil {
		return nil, err
	}
	var p repository.SubjectPatch
	if in.Name != nil {
		name, err := ValidateName(*in.Name)
		if err != nil {
			return nil, err
		}
		p.Name = &name
	}
	if code, clear := cleanSubjectText(in.Code); clear {
		p.ClearCode = true
	} else if code != nil {
		p.Code = code
	}
	if professor, clear := cleanSubjectText(in.Professor); clear {
		p.ClearProfessor = true
	} else if professor != nil {
		p.Professor = professor
	}
	if room, clear := cleanSubjectText(in.Room); clear {
		p.ClearRoom = true
	} else if room != nil {
		p.Room = room
	}
	if in.Semester != nil {
		p.Semester = in.Semester
	}
	if in.Credits != nil {
		p.Credits = in.Credits
	}
	if in.TotalClasses != nil {
		p.TotalClasses = in.TotalClasses
	}
	if in.MinAttendancePct != nil {
		p.MinAttendancePct = in.MinAttendancePct
	}
	if in.AverageDecimalPlaces != nil {
		dec, err := cleanDecimal(in.AverageDecimalPlaces)
		if err != nil {
			return nil, err
		}
		p.AverageDecimalPlaces = &dec
	}
	if in.Status != nil {
		status, err := cleanStatus(in.Status)
		if err != nil {
			return nil, err
		}
		p.Status = &status
	}
	if p.Name == nil && p.Semester == nil && p.Credits == nil && p.TotalClasses == nil &&
		p.MinAttendancePct == nil && p.AverageDecimalPlaces == nil && p.Status == nil &&
		p.Code == nil && p.Professor == nil && p.Room == nil &&
		!p.ClearCode && !p.ClearProfessor && !p.ClearRoom {
		return nil, NewServiceError(utils.ErrBadRequest)
	}
	out, err := s.repo.Update(ctx, id, strings.TrimSpace(userID), p)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, NewServiceError(utils.ErrNotFound)
	}
	return out, nil
}

// Delete borra una materia propia (schedules caen por CASCADE).
// 404 si no existe o su malla es ajena.
func (s *SubjectService) Delete(ctx context.Context, userID, id string) error {
	if err := requireUser(userID); err != nil {
		return err
	}
	id, err := ValidateUUID(id)
	if err != nil {
		return err
	}
	deleted, err := s.repo.Delete(ctx, id, strings.TrimSpace(userID))
	if err != nil {
		return err
	}
	if !deleted {
		return NewServiceError(utils.ErrNotFound)
	}
	return nil
}
