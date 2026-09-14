package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
)

// In-memory stores para tests unitarios sin DB.

type MemoryNoteStore struct {
	mu    sync.RWMutex
	notes map[string]*model.Note
}

func NewMemoryNoteStore() *MemoryNoteStore {
	return &MemoryNoteStore{notes: make(map[string]*model.Note)}
}
func (m *MemoryNoteStore) Create(ctx context.Context, userID string, subjectID *string, title string, externalFileID *string, visibility string, forkedFrom *string) (*model.Note, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := uuid.NewString()
	n := &model.Note{
		ID: id, UserID: userID, SubjectID: subjectID, Title: title,
		ExternalFileID: externalFileID, Visibility: visibility, LikesCount: 0,
		ForkedFromNoteID: forkedFrom, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	m.notes[id] = n
	cop := *n
	return &cop, nil
}
func (m *MemoryNoteStore) GetByID(ctx context.Context, id string) (*model.Note, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n, ok := m.notes[id]
	if !ok {
		return nil, nil
	}
	cop := *n
	return &cop, nil
}
func (m *MemoryNoteStore) ListByUser(ctx context.Context, userID, cursor string, limit int) ([]*model.Note, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*model.Note
	for _, n := range m.notes {
		if n.UserID == userID {
			cop := *n
			list = append(list, &cop)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].CreatedAt.Equal(list[j].CreatedAt) {
			return list[i].ID > list[j].ID
		}
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	// cursor pagination por ID
	if cursor != "" {
		idx := -1
		for i, n := range list {
			if n.ID == cursor {
				idx = i
				break
			}
		}
		if idx >= 0 {
			if idx+1 < len(list) {
				list = list[idx+1:]
			} else {
				list = []*model.Note{}
			}
		}
	}
	var next string
	if len(list) > limit {
		next = list[limit-1].ID
		list = list[:limit]
	}
	return list, next, nil
}
func (m *MemoryNoteStore) Update(ctx context.Context, id string, title *string, visibility *string) (*model.Note, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, ok := m.notes[id]
	if !ok {
		return nil, nil
	}
	if title != nil {
		n.Title = *title
	}
	if visibility != nil {
		n.Visibility = *visibility
	}
	n.UpdatedAt = time.Now().UTC()
	cop := *n
	return &cop, nil
}
func (m *MemoryNoteStore) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.notes[id]; !ok {
		return fmt.Errorf("not_found")
	}
	delete(m.notes, id)
	return nil
}
func (m *MemoryNoteStore) IncrementLikes(ctx context.Context, noteID string, delta int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, ok := m.notes[noteID]
	if !ok {
		return fmt.Errorf("not_found")
	}
	n.LikesCount += delta
	if n.LikesCount < 0 {
		n.LikesCount = 0
	}
	n.UpdatedAt = time.Now().UTC()
	return nil
}
func (m *MemoryNoteStore) UpdateExternalFileID(ctx context.Context, noteID, fileID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, ok := m.notes[noteID]
	if !ok {
		return fmt.Errorf("not_found")
	}
	n.ExternalFileID = &fileID
	n.UpdatedAt = time.Now().UTC()
	return nil
}

// Attachment memory
type MemoryAttachmentStore struct {
	mu   sync.RWMutex
	atts map[string]*model.Attachment
}

func NewMemoryAttachmentStore() *MemoryAttachmentStore {
	return &MemoryAttachmentStore{atts: make(map[string]*model.Attachment)}
}
func (m *MemoryAttachmentStore) Create(ctx context.Context, noteID, externalFileID, fileURL, fileType string, fileName *string, fileSize *int, isInline bool) (*model.Attachment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := uuid.NewString()
	a := &model.Attachment{
		ID: id, NoteID: noteID, ExternalFileID: externalFileID, FileURL: fileURL, FileType: fileType,
		FileName: fileName, FileSizeBytes: fileSize, IsInline: isInline, CreatedAt: time.Now().UTC(),
	}
	m.atts[id] = a
	cop := *a
	return &cop, nil
}
func (m *MemoryAttachmentStore) GetByID(ctx context.Context, id string) (*model.Attachment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.atts[id]
	if !ok {
		return nil, nil
	}
	cop := *a
	return &cop, nil
}
func (m *MemoryAttachmentStore) ListByNote(ctx context.Context, noteID string) ([]*model.Attachment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*model.Attachment
	for _, a := range m.atts {
		if a.NoteID == noteID {
			cop := *a
			out = append(out, &cop)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
func (m *MemoryAttachmentStore) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.atts[id]; !ok {
		return fmt.Errorf("not_found")
	}
	delete(m.atts, id)
	return nil
}

// Saved memory
type MemorySavedStore struct {
	mu    sync.RWMutex
	saved map[string]*model.SavedNote // key userID:noteID
	byID  map[string]*model.SavedNote
}

func NewMemorySavedStore() *MemorySavedStore {
	return &MemorySavedStore{saved: make(map[string]*model.SavedNote), byID: make(map[string]*model.SavedNote)}
}
func keySaved(u, n string) string { return u + ":" + n }
func (m *MemorySavedStore) Save(ctx context.Context, userID, noteID string) (*model.SavedNote, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := keySaved(userID, noteID)
	if _, ok := m.saved[k]; ok {
		return nil, fmt.Errorf("already_saved: duplicate")
	}
	id := uuid.NewString()
	s := &model.SavedNote{ID: id, UserID: userID, NoteID: noteID, SavedAt: time.Now().UTC()}
	m.saved[k] = s
	m.byID[id] = s
	cop := *s
	return &cop, nil
}
func (m *MemorySavedStore) Exists(ctx context.Context, userID, noteID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.saved[keySaved(userID, noteID)]
	return ok, nil
}
func (m *MemorySavedStore) Delete(ctx context.Context, userID, noteID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := keySaved(userID, noteID)
	if s, ok := m.saved[k]; ok {
		delete(m.byID, s.ID)
	}
	delete(m.saved, k)
	return nil
}

// Like memory
type MemoryLikeStore struct {
	mu    sync.RWMutex
	likes map[string]*model.NoteLike // key noteID:userID
}

func NewMemoryLikeStore() *MemoryLikeStore {
	return &MemoryLikeStore{likes: make(map[string]*model.NoteLike)}
}
func keyLike(n, u string) string { return n + ":" + u }
func (m *MemoryLikeStore) Create(ctx context.Context, noteID, userID string) (*model.NoteLike, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := keyLike(noteID, userID)
	if _, ok := m.likes[k]; ok {
		return nil, fmt.Errorf("already_liked: duplicate")
	}
	l := &model.NoteLike{ID: uuid.NewString(), NoteID: noteID, UserID: userID, CreatedAt: time.Now().UTC()}
	m.likes[k] = l
	cop := *l
	return &cop, nil
}
func (m *MemoryLikeStore) Delete(ctx context.Context, noteID, userID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := keyLike(noteID, userID)
	if _, ok := m.likes[k]; !ok {
		return false, nil
	}
	delete(m.likes, k)
	return true, nil
}
func (m *MemoryLikeStore) Exists(ctx context.Context, noteID, userID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.likes[keyLike(noteID, userID)]
	return ok, nil
}

// Shared memory
type MemorySharedStore struct {
	mu     sync.RWMutex
	shared map[string]*model.SharedNote
}

func NewMemorySharedStore() *MemorySharedStore {
	return &MemorySharedStore{shared: make(map[string]*model.SharedNote)}
}
func (m *MemorySharedStore) Create(ctx context.Context, noteID, groupID string, isAdminNote bool, accessMode string, followersSnapshot int) (*model.SharedNote, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := uuid.NewString()
	s := &model.SharedNote{
		ID: id, NoteID: noteID, GroupID: groupID, IsAdminNote: isAdminNote,
		AccessMode: accessMode, AuthorFollowersSnapshot: followersSnapshot, SharedAt: time.Now().UTC(),
	}
	m.shared[id] = s
	cop := *s
	return &cop, nil
}
func (m *MemorySharedStore) GetByID(ctx context.Context, id string) (*model.SharedNote, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.shared[id]
	if !ok {
		return nil, nil
	}
	cop := *s
	return &cop, nil
}
func (m *MemorySharedStore) ListByNote(ctx context.Context, noteID string) ([]*model.SharedNote, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*model.SharedNote
	for _, s := range m.shared {
		if s.NoteID == noteID {
			cop := *s
			out = append(out, &cop)
		}
	}
	return out, nil
}
func (m *MemorySharedStore) ListByGroup(ctx context.Context, groupID string, limit int, cursor string) ([]*model.SharedNote, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*model.SharedNote
	for _, s := range m.shared {
		if s.GroupID == groupID {
			cop := *s
			list = append(list, &cop)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].IsAdminNote != list[j].IsAdminNote {
			return list[i].IsAdminNote && !list[j].IsAdminNote
		}
		return list[i].SharedAt.After(list[j].SharedAt)
	})
	if cursor != "" {
		idx := -1
		for i, s := range list {
			if s.ID == cursor {
				idx = i
				break
			}
		}
		if idx >= 0 {
			if idx+1 < len(list) {
				list = list[idx+1:]
			} else {
				list = []*model.SharedNote{}
			}
		}
	}
	var next string
	if len(list) > limit {
		next = list[limit-1].ID
		list = list[:limit]
	}
	return list, next, nil
}
func (m *MemorySharedStore) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.shared[id]; !ok {
		return fmt.Errorf("not_found")
	}
	delete(m.shared, id)
	return nil
}
func (m *MemorySharedStore) DeleteAllByNote(ctx context.Context, noteID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range m.shared {
		if v.NoteID == noteID {
			delete(m.shared, k)
		}
	}
	return nil
}
func (m *MemorySharedStore) DeleteByUserAndGroup(ctx context.Context, userID, groupID string) error {
	// Necesitamos conocer notas del usuario; este store no tiene acceso a notes store.
	// Para memoria, solo borramos shares donde groupID coincide y asumimos note pertenece a user si test lo configura.
	// Simplificación: borrar todos con groupID (tests ajustan)
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range m.shared {
		if v.GroupID == groupID && strings.HasPrefix(k, "") {
			// No podemos filtrar por user sin referencia; dejamos que test use DeleteAllByNote o mock externo.
			// Para hacerlo correcto, necesitamos inyectar noteStore; alternativa: borrar todos del grupo
			_ = userID
			delete(m.shared, k)
			_ = v
		}
	}
	return nil
}
func (m *MemorySharedStore) HasAccess(ctx context.Context, noteID, groupID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.shared {
		if s.NoteID == noteID && s.GroupID == groupID {
			return true, nil
		}
	}
	return false, nil
}
func (m *MemorySharedStore) HasAnyShare(ctx context.Context, noteID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.shared {
		if s.NoteID == noteID {
			return true, nil
		}
	}
	return false, nil
}

var _ = strings.Contains

// MemorySocialResolver para tests
type MemorySocialResolver struct {
	mu       sync.RWMutex
	members  map[string]bool // key userID:groupID
	admins   map[string]bool
	followers map[string]int
}

func NewMemorySocialResolver() *MemorySocialResolver {
	return &MemorySocialResolver{
		members:   make(map[string]bool),
		admins:    make(map[string]bool),
		followers: make(map[string]int),
	}
}
func (r *MemorySocialResolver) AddMember(userID, groupID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.members[userID+":"+groupID] = true
}
func (r *MemorySocialResolver) AddAdmin(userID, groupID string) {
	r.AddMember(userID, groupID)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.admins[userID+":"+groupID] = true
}
func (r *MemorySocialResolver) SetFollowers(userID string, cnt int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.followers[userID] = cnt
}
func (r *MemorySocialResolver) IsMember(ctx context.Context, userID, groupID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.members[userID+":"+groupID], nil
}
func (r *MemorySocialResolver) IsAdmin(ctx context.Context, userID, groupID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.admins[userID+":"+groupID], nil
}
func (r *MemorySocialResolver) GetFollowersCount(ctx context.Context, userID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.followers[userID], nil
}
