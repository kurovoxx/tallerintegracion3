-- name: CreateNote :one
INSERT INTO notes.notes (user_id, subject_id, title, external_file_id, visibility, forked_from_note_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, user_id, subject_id, title, external_file_id, visibility, likes_count, forked_from_note_id, created_at, updated_at;

-- name: GetNoteByID :one
SELECT id, user_id, subject_id, title, external_file_id, visibility, likes_count, forked_from_note_id, created_at, updated_at
FROM notes.notes WHERE id = $1;

-- name: ListNotesByUser :many
SELECT id, user_id, subject_id, title, external_file_id, visibility, likes_count, forked_from_note_id, created_at, updated_at
FROM notes.notes WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2;

-- name: UpdateNote :one
UPDATE notes.notes SET title = COALESCE($2, title), visibility = COALESCE($3, visibility), updated_at = now()
WHERE id = $1
RETURNING id, user_id, subject_id, title, external_file_id, visibility, likes_count, forked_from_note_id, created_at, updated_at;

-- name: DeleteNote :exec
DELETE FROM notes.notes WHERE id = $1;

-- name: IncrementLikes :exec
UPDATE notes.notes SET likes_count = likes_count + $1, updated_at = now() WHERE id = $2;

-- name: CreateAttachment :one
INSERT INTO notes.note_attachments (note_id, external_file_id, file_url, file_type, file_name, file_size_bytes, is_inline)
VALUES ($1,$2,$3,$4,$5,$6,$7)
RETURNING id, note_id, external_file_id, file_url, file_type, file_name, file_size_bytes, is_inline, created_at;

-- name: GetAttachment :one
SELECT id, note_id, external_file_id, file_url, file_type, file_name, file_size_bytes, is_inline, created_at
FROM notes.note_attachments WHERE id=$1;

-- name: ListAttachmentsByNote :many
SELECT id, note_id, external_file_id, file_url, file_type, file_name, file_size_bytes, is_inline, created_at
FROM notes.note_attachments WHERE note_id=$1 ORDER BY created_at DESC;

-- name: DeleteAttachment :exec
DELETE FROM notes.note_attachments WHERE id=$1;

-- name: SaveNote :one
INSERT INTO notes.saved_notes (user_id, note_id) VALUES ($1,$2) RETURNING id, user_id, note_id, saved_at;

-- name: DeleteSavedNote :exec
DELETE FROM notes.saved_notes WHERE user_id=$1 AND note_id=$2;

-- name: ExistsSavedNote :one
SELECT EXISTS(SELECT 1 FROM notes.saved_notes WHERE user_id=$1 AND note_id=$2);

-- name: CreateLike :one
INSERT INTO notes.note_likes (note_id, user_id) VALUES ($1,$2) RETURNING id, note_id, user_id, created_at;

-- name: DeleteLike :exec
DELETE FROM notes.note_likes WHERE note_id=$1 AND user_id=$2;

-- name: ExistsLike :one
SELECT EXISTS(SELECT 1 FROM notes.note_likes WHERE note_id=$1 AND user_id=$2);

-- name: CreateSharedNote :one
INSERT INTO notes.shared_notes (note_id, group_id, is_admin_note, access_mode, author_followers_snapshot)
VALUES ($1,$2,$3,$4,$5)
RETURNING id, note_id, group_id, is_admin_note, access_mode, author_followers_snapshot, shared_at;

-- name: GetSharedNote :one
SELECT id, note_id, group_id, is_admin_note, access_mode, author_followers_snapshot, shared_at FROM notes.shared_notes WHERE id=$1;

-- name: ListSharedByNote :many
SELECT id, note_id, group_id, is_admin_note, access_mode, author_followers_snapshot, shared_at FROM notes.shared_notes WHERE note_id=$1;

-- name: ListSharedByGroup :many
SELECT id, note_id, group_id, is_admin_note, access_mode, author_followers_snapshot, shared_at
FROM notes.shared_notes WHERE group_id=$1 ORDER BY is_admin_note DESC, shared_at DESC LIMIT $2;

-- name: DeleteSharedNote :exec
DELETE FROM notes.shared_notes WHERE id=$1;

-- name: DeleteSharedByNoteAndGroup :exec
DELETE FROM notes.shared_notes WHERE note_id=$1 AND group_id=$2;

-- name: DeleteSharedAllByNote :exec
DELETE FROM notes.shared_notes WHERE note_id=$1;

-- name: DeleteSharedByUserAndGroup :exec
DELETE FROM notes.shared_notes WHERE group_id=$1 AND note_id IN (SELECT id FROM notes.notes WHERE user_id=$2);

-- name: HasAnyShare :one
SELECT EXISTS(SELECT 1 FROM notes.shared_notes WHERE note_id=$1);
