package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/repository"
)

// In-memory stores para tests unitarios sin DB.

type MemoryNoteStore struct {
	mu              sync.RWMutex
	notes           map[string]*model.Note
	driveOperations map[string]*model.DriveOperation
	// idempotencyKeys replica notes.idempotency_keys para tests sin PG:
	// clave userID:operation:idempotencyKey.
	idempotencyKeys map[string]*model.IdempotencyKey
}

func NewMemoryNoteStore() *MemoryNoteStore {
	return &MemoryNoteStore{notes: make(map[string]*model.Note)}
}

// Create inserta una nota en memoria. noteID permite preasignar el id (crash
// recovery lógico); vacío => se genera uno nuevo. Un id duplicado replica la
// violación de PK de notes.notes.
func (m *MemoryNoteStore) Create(ctx context.Context, noteID string, userID string, subjectID *string, title string, externalFileID *string, visibility string, forkedFrom *string, syncStatus string) (*model.Note, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if syncStatus == "" {
		syncStatus = "pending_drive"
	}
	if strings.TrimSpace(noteID) == "" {
		noteID = uuid.NewString()
	}
	if _, exists := m.notes[noteID]; exists {
		return nil, fmt.Errorf("create note: duplicate_note_id: %s", noteID)
	}
	n := &model.Note{
		ID: noteID, UserID: userID, SubjectID: subjectID, Title: title,
		ExternalFileID: externalFileID, Visibility: visibility, LikesCount: 0,
		ForkedFromNoteID: forkedFrom, SyncStatus: syncStatus, Version: 1,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	m.notes[noteID] = n
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

// Update replica el versionado optimista de PG: si la versión esperada no
// coincide con la actual devuelve repository.ErrConflict (la nota existe y otro
// writer ganó); si la nota no existe devuelve (nil, nil). En caso de éxito
// incrementa Version en la misma operación.
func (m *MemoryNoteStore) Update(ctx context.Context, id string, title *string, visibility *string, expectedVersion int64) (*model.Note, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, ok := m.notes[id]
	if !ok {
		return nil, nil
	}
	if n.Version != expectedVersion {
		return nil, repository.ErrConflict
	}
	if title != nil {
		n.Title = *title
	}
	if visibility != nil {
		n.Visibility = *visibility
	}
	n.Version++
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
		return repository.ErrNotFound
	}
	n.ExternalFileID = &fileID
	n.SyncStatus = "synced"
	n.UpdatedAt = time.Now().UTC()
	return nil
}

// idemMemoryKey namespacea la clave de idempotencia en el store en memoria
// igual que la UNIQUE (user_id, operation, idempotency_key) de PG.
func idemMemoryKey(userID, operation, idempotencyKey string) string {
	return userID + ":" + operation + ":" + idempotencyKey
}

func cloneIdempotencyKey(k *model.IdempotencyKey) *model.IdempotencyKey {
	if k == nil {
		return nil
	}
	cop := *k
	if k.ResourceID != nil {
		id := *k.ResourceID
		cop.ResourceID = &id
	}
	return &cop
}

// GetOrClaimIdempotencyKey replica el claim atómico de PG: inserta la clave si
// no existe o si el registro previo venció (claim in_progress colgado por un
// crash o replay completed caducado); si existe vigente, lo devuelve sin
// reclamarlo. Un registro 'recoverable' con el mismo request_hash se reclama
// de inmediato (sin esperar a expires_at); con hash distinto nunca se reclama
// (el servicio responde 409 Conflict). preassignedResourceID pre-asocia el
// recurso al claim nuevo y, al reclamar un registro vencido o recoverable, se
// preserva estrictamente el resource_id previo (paridad con el COALESCE de PG)
// para recuperar el id del intento que murió.
func (m *MemoryNoteStore) GetOrClaimIdempotencyKey(ctx context.Context, userID, operation, idempotencyKey, requestHash string, preassignedResourceID *string, expiresAt time.Time) (*model.IdempotencyKey, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(operation) == "" || strings.TrimSpace(idempotencyKey) == "" {
		return nil, false, fmt.Errorf("claim idempotency key: operation and key are required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.idempotencyKeys == nil {
		m.idempotencyKeys = make(map[string]*model.IdempotencyKey)
	}
	now := time.Now().UTC()
	k := idemMemoryKey(userID, operation, idempotencyKey)
	existing := m.idempotencyKeys[k]
	if existing != nil && existing.Status == model.IdempotencyStatusRecoverable {
		if existing.RequestHash != requestHash {
			return cloneIdempotencyKey(existing), false, nil
		}
		rec := &model.IdempotencyKey{
			UserID:         userID,
			Operation:      operation,
			IdempotencyKey: idempotencyKey,
			RequestHash:    requestHash,
			Status:         model.IdempotencyStatusInProgress,
			ExpiresAt:      expiresAt,
		}
		if existing.ResourceID != nil && strings.TrimSpace(*existing.ResourceID) != "" {
			preserved := *existing.ResourceID
			rec.ResourceID = &preserved
		} else if preassignedResourceID != nil && strings.TrimSpace(*preassignedResourceID) != "" {
			preassigned := *preassignedResourceID
			rec.ResourceID = &preassigned
		}
		m.idempotencyKeys[k] = rec
		return cloneIdempotencyKey(rec), true, nil
	}
	if existing != nil && existing.ExpiresAt.After(now) {
		return cloneIdempotencyKey(existing), false, nil
	}
	rec := &model.IdempotencyKey{
		UserID:         userID,
		Operation:      operation,
		IdempotencyKey: idempotencyKey,
		RequestHash:    requestHash,
		Status:         model.IdempotencyStatusInProgress,
		ExpiresAt:      expiresAt,
	}
	if existing != nil && existing.ResourceID != nil && strings.TrimSpace(*existing.ResourceID) != "" {
		preserved := *existing.ResourceID
		rec.ResourceID = &preserved
	} else if preassignedResourceID != nil && strings.TrimSpace(*preassignedResourceID) != "" {
		preassigned := *preassignedResourceID
		rec.ResourceID = &preassigned
	}
	m.idempotencyKeys[k] = rec
	return cloneIdempotencyKey(rec), true, nil
}

// CompleteIdempotencyKey publica el recurso asociado y abre la ventana de
// replay (inMemoryIdempotencyReplayTTL), igual que el intervalo SQL de 24h.
func (m *MemoryNoteStore) CompleteIdempotencyKey(ctx context.Context, userID, operation, idempotencyKey, resourceID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rec := m.idempotencyKeys[idemMemoryKey(userID, operation, idempotencyKey)]
	if rec == nil {
		return repository.ErrNotFound
	}
	id := resourceID
	rec.ResourceID = &id
	rec.Status = model.IdempotencyStatusCompleted
	rec.ExpiresAt = time.Now().UTC().Add(inMemoryIdempotencyReplayTTL)
	return nil
}

// DeleteIdempotencyKey libera la clave reclamada tras un fallo para permitir
// reintentos inmediatos con la misma X-Idempotency-Key.
func (m *MemoryNoteStore) DeleteIdempotencyKey(ctx context.Context, userID, operation, idempotencyKey string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.idempotencyKeys, idemMemoryKey(userID, operation, idempotencyKey))
	return nil
}

// MarkIdempotencyRecoverable marca la clave como 'recoverable' tras un fallo
// posterior a la inserción local del recurso: asocia el resourceID ya
// persistido, extiende la retención (paridad con las 24h de PG) y permite que
// un reintento inmediato con el mismo request_hash lo retome sin duplicar.
func (m *MemoryNoteStore) MarkIdempotencyRecoverable(ctx context.Context, userID, operation, idempotencyKey, resourceID string, _ string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rec := m.idempotencyKeys[idemMemoryKey(userID, operation, idempotencyKey)]
	if rec == nil {
		return repository.ErrNotFound
	}
	id := resourceID
	rec.ResourceID = &id
	rec.Status = model.IdempotencyStatusRecoverable
	rec.ExpiresAt = time.Now().UTC().Add(inMemoryIdempotencyReplayTTL)
	return nil
}

// Attachment memory
type MemoryAttachmentStore struct {
	mu    sync.RWMutex
	atts  map[string]*model.Attachment
	notes *MemoryNoteStore
}

func NewMemoryAttachmentStore() *MemoryAttachmentStore {
	return &MemoryAttachmentStore{atts: make(map[string]*model.Attachment)}
}

func (m *MemoryAttachmentStore) SetNoteStore(notes *MemoryNoteStore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notes = notes
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
	mu    sync.Mutex
	likes map[string]*model.NoteLike // key noteID:userID
	notes *MemoryNoteStore           // referencia para LikeAtomic (incremento atómico)
}

func NewMemoryLikeStore() *MemoryLikeStore {
	return &MemoryLikeStore{likes: make(map[string]*model.NoteLike)}
}

// NewMemoryLikeStoreWithNotes crea like store ligado a notes para operaciones atómicas.
func NewMemoryLikeStoreWithNotes(notes *MemoryNoteStore) *MemoryLikeStore {
	return &MemoryLikeStore{likes: make(map[string]*model.NoteLike), notes: notes}
}

// SetNoteStore inyecta referencia a notes para LikeAtomic/UnlikeAtomic.
func (m *MemoryLikeStore) SetNoteStore(notes *MemoryNoteStore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notes = notes
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
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.likes[keyLike(noteID, userID)]
	return ok, nil
}

// LikeAtomic inserta like y incrementa likes_count de forma atómica en memoria (mutex exclusivo).
func (m *MemoryLikeStore) LikeAtomic(ctx context.Context, noteID, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := keyLike(noteID, userID)
	if _, ok := m.likes[k]; ok {
		return fmt.Errorf("already_liked: duplicate")
	}
	l := &model.NoteLike{ID: uuid.NewString(), NoteID: noteID, UserID: userID, CreatedAt: time.Now().UTC()}
	m.likes[k] = l
	if m.notes != nil {
		m.notes.mu.Lock()
		if n, ok := m.notes.notes[noteID]; ok {
			n.LikesCount++
			n.UpdatedAt = time.Now().UTC()
		}
		m.notes.mu.Unlock()
	}
	return nil
}

// UnlikeAtomic elimina like y decrementa likes_count de forma atómica (GREATEST 0).
func (m *MemoryLikeStore) UnlikeAtomic(ctx context.Context, noteID, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := keyLike(noteID, userID)
	if _, ok := m.likes[k]; !ok {
		return nil // idempotente
	}
	delete(m.likes, k)
	if m.notes != nil {
		m.notes.mu.Lock()
		if n, ok := m.notes.notes[noteID]; ok {
			if n.LikesCount > 0 {
				n.LikesCount--
			}
			n.UpdatedAt = time.Now().UTC()
		}
		m.notes.mu.Unlock()
	}
	return nil
}

// Shared memory
type MemorySharedStore struct {
	mu      sync.RWMutex
	shared  map[string]*model.SharedNote
	managed map[string]*model.DriveManagedPermission // key noteID:principalType:principalKey
	// noteLocks es el equivalente en memoria del lock distribuido por nota
	// (pg_advisory_xact_lock) que serializa los cambios de ACL en PG: permite a
	// los tests unitarios ejercitar la misma exclusión Share/Unshare/Update.
	noteLocksMu sync.Mutex
	noteLocks   map[string]*sync.Mutex
}

func NewMemorySharedStore() *MemorySharedStore {
	return &MemorySharedStore{shared: make(map[string]*model.SharedNote), managed: make(map[string]*model.DriveManagedPermission)}
}

// noteLock devuelve (creándolo si hace falta) el mutex exclusivo de la nota.
func (m *MemorySharedStore) noteLock(noteID string) *sync.Mutex {
	m.noteLocksMu.Lock()
	defer m.noteLocksMu.Unlock()
	if m.noteLocks == nil {
		m.noteLocks = make(map[string]*sync.Mutex)
	}
	l, ok := m.noteLocks[noteID]
	if !ok {
		l = &sync.Mutex{}
		m.noteLocks[noteID] = l
	}
	return l
}

// WithNoteLock ejecuta fn sosteniendo el lock exclusivo de la nota. Réplica en
// memoria del lock distribuido PG para tests unitarios: serializa Share,
// Unshare, Update y el reconciliador sobre la misma nota.
func (m *MemorySharedStore) WithNoteLock(ctx context.Context, noteID string, fn func(context.Context) error) error {
	if fn == nil {
		return fmt.Errorf("note lock: callback is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	l := m.noteLock(noteID)
	l.Lock()
	defer l.Unlock()
	return fn(ctx)
}

func keyManaged(noteID, principalType, principalKey string) string {
	return noteID + ":" + principalType + ":" + principalKey
}

func cloneManagedPermission(p *model.DriveManagedPermission) *model.DriveManagedPermission {
	if p == nil {
		return nil
	}
	cop := *p
	if p.DrivePermissionID != nil {
		id := *p.DrivePermissionID
		cop.DrivePermissionID = &id
	}
	return &cop
}

func (m *MemorySharedStore) Create(ctx context.Context, noteID, groupID string, isAdminNote bool, accessMode string, followersSnapshot int) (*model.SharedNote, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := uuid.NewString()
	s := &model.SharedNote{
		ID: id, NoteID: noteID, GroupID: groupID, IsAdminNote: isAdminNote,
		AccessMode: accessMode, AuthorFollowersSnapshot: followersSnapshot,
		PermissionSyncStatus: model.PermissionSyncPending, SharedAt: time.Now().UTC(),
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

func (m *MemorySharedStore) UpdatePermissionSyncStatus(ctx context.Context, id, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.shared[id]
	if !ok {
		return repository.ErrNotFound
	}
	s.PermissionSyncStatus = status
	return nil
}

func (m *MemorySharedStore) UpdateNotePermissionSyncStatus(ctx context.Context, noteID, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.shared {
		if s.NoteID == noteID {
			s.PermissionSyncStatus = status
		}
	}
	return nil
}

// ListNotesWithPendingPermissionSync replica la consulta PG: notas con algún
// share o permiso administrado fuera de in_sync.
func (m *MemorySharedStore) ListNotesWithPendingPermissionSync(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	seen := map[string]bool{}
	var ids []string
	add := func(noteID string) {
		if seen[noteID] {
			return
		}
		seen[noteID] = true
		ids = append(ids, noteID)
	}
	for _, s := range m.shared {
		if s.PermissionSyncStatus != model.PermissionSyncInSync {
			add(s.NoteID)
		}
	}
	for _, p := range m.managed {
		if p.SyncStatus != model.PermissionSyncInSync {
			add(p.NoteID)
		}
	}
	sort.Strings(ids)
	if len(ids) > limit {
		ids = ids[:limit]
	}
	return ids, nil
}

func (m *MemorySharedStore) ListManagedPermissions(ctx context.Context, noteID string) ([]*model.DriveManagedPermission, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*model.DriveManagedPermission
	for _, p := range m.managed {
		if p.NoteID == noteID {
			out = append(out, cloneManagedPermission(p))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PrincipalType != out[j].PrincipalType {
			return out[i].PrincipalType < out[j].PrincipalType
		}
		return out[i].PrincipalKey < out[j].PrincipalKey
	})
	return out, nil
}

func (m *MemorySharedStore) UpsertManagedPermission(ctx context.Context, permission *model.DriveManagedPermission) (*model.DriveManagedPermission, error) {
	if permission == nil {
		return nil, fmt.Errorf("managed permission is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.managed == nil {
		m.managed = make(map[string]*model.DriveManagedPermission)
	}
	if permission.Role == "" {
		permission.Role = "reader"
	}
	k := keyManaged(permission.NoteID, permission.PrincipalType, permission.PrincipalKey)
	now := time.Now().UTC()
	if existing := m.managed[k]; existing != nil {
		existing.ExternalFileID = permission.ExternalFileID
		if permission.DrivePermissionID != nil {
			existing.DrivePermissionID = permission.DrivePermissionID
		}
		existing.Role = permission.Role
		existing.SyncStatus = permission.SyncStatus
		existing.UpdatedAt = now
		return cloneManagedPermission(existing), nil
	}
	rec := *permission
	rec.ID = uuid.NewString()
	rec.CreatedAt, rec.UpdatedAt = now, now
	m.managed[k] = &rec
	return cloneManagedPermission(&rec), nil
}

func (m *MemorySharedStore) DeleteManagedPermission(ctx context.Context, noteID, principalType, principalKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.managed, keyManaged(noteID, principalType, principalKey))
	return nil
}

func (m *MemorySharedStore) DeleteManagedPermissionsByNote(ctx context.Context, noteID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, p := range m.managed {
		if p.NoteID == noteID {
			delete(m.managed, k)
		}
	}
	return nil
}

func (m *MemorySharedStore) MarkManagedPermissionsPending(ctx context.Context, noteID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.managed {
		if p.NoteID == noteID && p.SyncStatus != model.PermissionSyncPending {
			p.SyncStatus = model.PermissionSyncPending
			p.UpdatedAt = time.Now().UTC()
		}
	}
	return nil
}

// SetSharedAt asigna shared_at para el noteID dado (helper para tests de ordenamiento).
func (m *MemorySharedStore) SetSharedAt(noteID string, t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, sh := range m.shared {
		if sh.NoteID == noteID {
			sh.SharedAt = t
		}
	}
}

// SetIsAdminNote asigna is_admin_note para el noteID dado (helper para tests).
func (m *MemorySharedStore) SetIsAdminNote(noteID string, isAdmin bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, sh := range m.shared {
		if sh.NoteID == noteID {
			sh.IsAdminNote = isAdmin
		}
	}
}

var _ = strings.Contains

// MemorySocialResolver para tests
type MemorySocialResolver struct {
	mu        sync.RWMutex
	members   map[string]bool // key userID:groupID
	admins    map[string]bool
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

// MemoryMemberDirectory mock de GroupMemberDirectory para tests.
type MemoryMemberDirectory struct {
	mu     sync.RWMutex
	emails map[string][]string // groupID -> correos crudos
	err    error
}

func NewMemoryMemberDirectory() *MemoryMemberDirectory {
	return &MemoryMemberDirectory{emails: make(map[string][]string)}
}

// SetEmails fija los correos crudos de un grupo (pueden incluir duplicados,
// vacíos o mayúsculas para ejercitar la normalización).
func (m *MemoryMemberDirectory) SetEmails(groupID string, emails []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emails[groupID] = emails
}

// SetError inyecta un fallo al obtener miembros (nil lo limpia).
func (m *MemoryMemberDirectory) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

func (m *MemoryMemberDirectory) ListMemberEmails(ctx context.Context, groupID string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.err != nil {
		return nil, m.err
	}
	return m.emails[groupID], nil
}

func (m *MemoryNoteStore) memoryNoteStore() *MemoryNoteStore { return m }

func (m *MemoryNoteStore) EnqueueDriveOperation(ctx context.Context, op, noteID, attID, fileID, ownerUserID string, payload map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.enqueueDriveOperation(ctx, op, noteID, attID, fileID, ownerUserID, payload)
}

// Caller holds mu so a local mutation and its outbox entry are atomic.
func (m *MemoryNoteStore) enqueueDriveOperation(ctx context.Context, op, noteID, attID, fileID, ownerUserID string, payload map[string]any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if ownerUserID == "" {
		return fmt.Errorf("missing drive operation owner")
	}
	if payload == nil {
		payload = map[string]any{}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	optional := func(v string) *string {
		if v == "" {
			return nil
		}
		return &v
	}
	now := time.Now().UTC()
	job := &model.DriveOperation{ID: uuid.NewString(), Operation: op, NoteID: optional(noteID), AttachmentID: optional(attID), ExternalFileID: optional(fileID), OwnerUserID: ownerUserID, Payload: body, Status: "pending", NextAttemptAt: now, CreatedAt: &now, UpdatedAt: &now}
	if m.driveOperations == nil {
		m.driveOperations = make(map[string]*model.DriveOperation)
	}
	m.driveOperations[job.ID] = job
	return nil
}

func (m *MemoryNoteStore) ClaimDriveOperations(ctx context.Context, limit int, lockDuration time.Duration) ([]*model.DriveOperation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 || lockDuration <= 0 {
		return nil, fmt.Errorf("invalid claim limit or lock duration")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	var jobs []*model.DriveOperation
	for _, job := range m.driveOperations {
		if (job.Status == "pending" || job.Status == "failed" || job.Status == "processing") && !job.NextAttemptAt.After(now) {
			jobs = append(jobs, job)
		}
	}
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].NextAttemptAt.Equal(jobs[j].NextAttemptAt) {
			return jobs[i].ID < jobs[j].ID
		}
		return jobs[i].NextAttemptAt.Before(jobs[j].NextAttemptAt)
	})
	if len(jobs) > limit {
		jobs = jobs[:limit]
	}
	for i, job := range jobs {
		job.Status = "processing"
		job.Attempts++
		job.NextAttemptAt = now.Add(lockDuration)
		job.UpdatedAt = &now
		cop := *job
		cop.Payload = append(json.RawMessage(nil), job.Payload...)
		clone := func(p *string) *string {
			if p == nil {
				return nil
			}
			v := *p
			return &v
		}
		cop.NoteID, cop.AttachmentID, cop.ExternalFileID, cop.LastError = clone(job.NoteID), clone(job.AttachmentID), clone(job.ExternalFileID), clone(job.LastError)
		cloneTime := func(p *time.Time) *time.Time {
			if p == nil {
				return nil
			}
			v := *p
			return &v
		}
		cop.CreatedAt, cop.UpdatedAt, cop.CompletedAt = cloneTime(job.CreatedAt), cloneTime(job.UpdatedAt), cloneTime(job.CompletedAt)
		jobs[i] = &cop
	}
	return jobs, nil
}

func (m *MemoryNoteStore) CompleteDriveOperation(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	job := m.driveOperations[id]
	if job == nil || job.Status != "processing" {
		return repository.ErrNotFound
	}
	now := time.Now().UTC()
	job.Status, job.CompletedAt, job.UpdatedAt, job.LastError = "completed", &now, &now, nil
	return nil
}

func (m *MemoryNoteStore) FailDriveOperation(ctx context.Context, id, errStr string, nextAttempt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	job := m.driveOperations[id]
	if job == nil || job.Status != "processing" {
		return repository.ErrNotFound
	}
	now := time.Now().UTC()
	job.Status, job.LastError, job.NextAttemptAt, job.UpdatedAt = "failed", &errStr, nextAttempt, &now
	return nil
}

func (m *MemoryNoteStore) DeleteWithDriveCleanup(ctx context.Context, noteID, requesterID string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	note := m.notes[noteID]
	if note == nil {
		return "", repository.ErrNotFound
	}
	if note.UserID != requesterID {
		return "", repository.ErrForbidden
	}
	fileID := ""
	if note.ExternalFileID != nil {
		fileID = *note.ExternalFileID
	}
	if fileID != "" {
		if err := m.enqueueDriveOperation(ctx, "delete_file", noteID, "", fileID, note.UserID, nil); err != nil {
			return "", err
		}
	}
	delete(m.notes, noteID)
	return fileID, nil
}

func (m *MemoryAttachmentStore) DeleteAttachmentWithDriveCleanup(ctx context.Context, noteID, attachmentID, requesterID string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	m.mu.RLock()
	notes := m.notes
	m.mu.RUnlock()
	if notes == nil {
		return "", fmt.Errorf("attachment store requires note store")
	}
	notes.mu.Lock()
	defer notes.mu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	note := notes.notes[noteID]
	if note == nil {
		return "", repository.ErrNotFound
	}
	if note.UserID != requesterID {
		return "", repository.ErrForbidden
	}
	att := m.atts[attachmentID]
	if att == nil || att.NoteID != noteID {
		return "", repository.ErrNotFound
	}
	if att.ExternalFileID != "" {
		if err := notes.enqueueDriveOperation(ctx, "delete_attachment", noteID, attachmentID, att.ExternalFileID, note.UserID, nil); err != nil {
			return "", err
		}
	}
	delete(m.atts, attachmentID)
	return att.ExternalFileID, nil
}

func (m *MemoryNoteStore) UpdateSyncStatus(ctx context.Context, noteID, syncStatus string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, ok := m.notes[noteID]
	if !ok {
		return fmt.Errorf("not_found")
	}
	n.SyncStatus = syncStatus
	n.UpdatedAt = time.Now().UTC()
	return nil
}

// GetPendingSyncNotes replica la consulta del repositorio PG: devuelve las
// notas pendientes de reconciliación (pending_drive y failed_sync). El filtro
// de estado/antigüedad aplica en la capa de servicio.
func (m *MemoryNoteStore) GetPendingSyncNotes(ctx context.Context) ([]*model.Note, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*model.Note
	for _, n := range m.notes {
		if n.SyncStatus == "pending_drive" || n.SyncStatus == "failed_sync" {
			cop := *n
			out = append(out, &cop)
		}
	}
	return out, nil
}
