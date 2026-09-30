package drive

import (
	"context"
	"io"
	"mime"
)

const MaxAttachmentBytes = 10 * 1024 * 1024

// DownloadAttachment preserves bytes and uses the note owner's OAuth context.
func (r *RealDriveClient) DownloadAttachment(ctx context.Context, ownerID, fileID string) ([]byte, string, error) {
	srv, err := r.serviceFor(ctx, ownerID)
	if err != nil {
		return nil, "", err
	}
	response, err := srv.Files.Get(fileID).Context(ctx).Download()
	if err != nil {
		return nil, "", mapGoogleError("DownloadAttachment", err)
	}
	defer response.Body.Close()
	if response.ContentLength > MaxAttachmentBytes {
		return nil, "", &DriveError{Code: 413, Message: "adjunto demasiado grande"}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, MaxAttachmentBytes+1))
	if err != nil {
		return nil, "", &DriveError{Code: 500, Message: "no se pudo leer el adjunto"}
	}
	if len(data) > MaxAttachmentBytes {
		return nil, "", &DriveError{Code: 413, Message: "adjunto demasiado grande"}
	}
	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || contentType == "application/octet-stream" {
		contentType = DetectMimeType("", "", data)
	}
	return data, contentType, nil
}

func (m *MockClient) DownloadAttachment(ctx context.Context, ownerID, fileID string) ([]byte, string, error) {
	if err := m.checkOAuth(ownerID); err != nil {
		return nil, "", err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if err := m.GetErr[fileID]; err != nil {
		return nil, "", err
	}
	file := m.files[fileID]
	if file == nil {
		return nil, "", &DriveError{Code: 404, Message: "adjunto no disponible"}
	}
	if file.OwnerID != ownerID {
		return nil, "", &DriveError{Code: 403, Message: "adjunto no disponible"}
	}
	if len(file.Data) > MaxAttachmentBytes {
		return nil, "", &DriveError{Code: 413, Message: "adjunto demasiado grande"}
	}
	return append([]byte(nil), file.Data...), file.MimeType, nil
}
