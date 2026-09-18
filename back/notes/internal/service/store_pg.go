package service

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/repository"
)

// Adaptadores PG que implementan Store interfaces usando repository + pool.

type PGNoteStore struct {
	repo *repository.NoteRepository
	pool *pgxpool.Pool
}

func NewPGNoteStore(pool *pgxpool.Pool) *PGNoteStore {
	return &PGNoteStore{repo: repository.NewNoteRepository(pool), pool: pool}
}
func (p *PGNoteStore) Create(ctx context.Context, userID string, subjectID *string, title string, externalFileID *string, visibility string, forkedFrom *string) (*model.Note, error) {
	return p.repo.Create(ctx, nil, userID, subjectID, title, externalFileID, visibility, forkedFrom)
}
func (p *PGNoteStore) GetByID(ctx context.Context, id string) (*model.Note, error) {
	return p.repo.GetByID(ctx, nil, id)
}
func (p *PGNoteStore) ListByUser(ctx context.Context, userID, cursor string, limit int) ([]*model.Note, string, error) {
	return p.repo.ListByUser(ctx, nil, userID, cursor, limit)
}
func (p *PGNoteStore) Update(ctx context.Context, id string, title *string, visibility *string) (*model.Note, error) {
	return p.repo.Update(ctx, nil, id, title, visibility)
}
func (p *PGNoteStore) Delete(ctx context.Context, id string) error {
	return p.repo.Delete(ctx, nil, id)
}
func (p *PGNoteStore) IncrementLikes(ctx context.Context, noteID string, delta int) error {
	return p.repo.IncrementLikes(ctx, nil, noteID, delta)
}
func (p *PGNoteStore) UpdateExternalFileID(ctx context.Context, noteID, fileID string) error {
	return p.repo.UpdateExternalFileID(ctx, nil, noteID, fileID)
}

type PGAttachmentStore struct {
	repo *repository.AttachmentRepository
}

func NewPGAttachmentStore(pool *pgxpool.Pool) *PGAttachmentStore {
	return &PGAttachmentStore{repo: repository.NewAttachmentRepository(pool)}
}
func (p *PGAttachmentStore) Create(ctx context.Context, noteID, externalFileID, fileURL, fileType string, fileName *string, fileSize *int, isInline bool) (*model.Attachment, error) {
	return p.repo.Create(ctx, nil, noteID, externalFileID, fileURL, fileType, fileName, fileSize, isInline)
}
func (p *PGAttachmentStore) GetByID(ctx context.Context, id string) (*model.Attachment, error) {
	return p.repo.GetByID(ctx, nil, id)
}
func (p *PGAttachmentStore) ListByNote(ctx context.Context, noteID string) ([]*model.Attachment, error) {
	return p.repo.ListByNote(ctx, nil, noteID)
}
func (p *PGAttachmentStore) Delete(ctx context.Context, id string) error {
	return p.repo.Delete(ctx, nil, id)
}

type PGSavedStore struct {
	repo *repository.SavedRepository
}

func NewPGSavedStore(pool *pgxpool.Pool) *PGSavedStore {
	return &PGSavedStore{repo: repository.NewSavedRepository(pool)}
}
func (p *PGSavedStore) Save(ctx context.Context, userID, noteID string) (*model.SavedNote, error) {
	return p.repo.Save(ctx, nil, userID, noteID)
}
func (p *PGSavedStore) Exists(ctx context.Context, userID, noteID string) (bool, error) {
	return p.repo.Exists(ctx, nil, userID, noteID)
}
func (p *PGSavedStore) Delete(ctx context.Context, userID, noteID string) error {
	return p.repo.Delete(ctx, nil, userID, noteID)
}

type PGLikeStore struct {
	repo *repository.LikeRepository
}

func NewPGLikeStore(pool *pgxpool.Pool) *PGLikeStore {
	return &PGLikeStore{repo: repository.NewLikeRepository(pool)}
}
func (p *PGLikeStore) Create(ctx context.Context, noteID, userID string) (*model.NoteLike, error) {
	return p.repo.Create(ctx, nil, noteID, userID)
}
func (p *PGLikeStore) Delete(ctx context.Context, noteID, userID string) (bool, error) {
	return p.repo.Delete(ctx, nil, noteID, userID)
}
func (p *PGLikeStore) Exists(ctx context.Context, noteID, userID string) (bool, error) {
	return p.repo.Exists(ctx, nil, noteID, userID)
}
func (p *PGLikeStore) LikeAtomic(ctx context.Context, noteID, userID string) error {
	return p.repo.LikeAtomic(ctx, noteID, userID)
}
func (p *PGLikeStore) UnlikeAtomic(ctx context.Context, noteID, userID string) error {
	return p.repo.UnlikeAtomic(ctx, noteID, userID)
}

type PGSharedStore struct {
	repo *repository.SharedRepository
}

func NewPGSharedStore(pool *pgxpool.Pool) *PGSharedStore {
	return &PGSharedStore{repo: repository.NewSharedRepository(pool)}
}
func (p *PGSharedStore) Create(ctx context.Context, noteID, groupID string, isAdminNote bool, accessMode string, followersSnapshot int) (*model.SharedNote, error) {
	return p.repo.Create(ctx, nil, noteID, groupID, isAdminNote, accessMode, followersSnapshot)
}
func (p *PGSharedStore) GetByID(ctx context.Context, id string) (*model.SharedNote, error) {
	return p.repo.GetByID(ctx, nil, id)
}
func (p *PGSharedStore) ListByNote(ctx context.Context, noteID string) ([]*model.SharedNote, error) {
	return p.repo.ListByNote(ctx, nil, noteID)
}
func (p *PGSharedStore) ListByGroup(ctx context.Context, groupID string, limit int, cursor string) ([]*model.SharedNote, string, error) {
	return p.repo.ListByGroup(ctx, nil, groupID, limit, cursor)
}
func (p *PGSharedStore) Delete(ctx context.Context, id string) error {
	return p.repo.Delete(ctx, nil, id)
}
func (p *PGSharedStore) DeleteAllByNote(ctx context.Context, noteID string) error {
	return p.repo.DeleteAllByNote(ctx, nil, noteID)
}
func (p *PGSharedStore) DeleteByUserAndGroup(ctx context.Context, userID, groupID string) error {
	return p.repo.DeleteByUserAndGroup(ctx, nil, userID, groupID)
}
func (p *PGSharedStore) HasAccess(ctx context.Context, noteID, groupID string) (bool, error) {
	return p.repo.HasAccess(ctx, nil, noteID, groupID)
}
func (p *PGSharedStore) HasAnyShare(ctx context.Context, noteID string) (bool, error) {
	return p.repo.HasAnyShare(ctx, nil, noteID)
}
