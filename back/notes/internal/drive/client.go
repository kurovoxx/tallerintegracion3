package drive

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// DriveError representa error de la API de Drive con código HTTP semántico.
type DriveError struct {
	Code    int    // 403, 404, 413, 500
	Message string
}

func (e *DriveError) Error() string { return fmt.Sprintf("drive %d: %s", e.Code, e.Message) }

func IsNotFound(err error) bool {
	var de *DriveError
	if errors.As(err, &de) {
		return de.Code == 404
	}
	return false
}
func IsForbidden(err error) bool {
	var de *DriveError
	if errors.As(err, &de) {
		return de.Code == 403
	}
	return false
}

// Client abstrae Google Drive del usuario autenticado.
// En producción delega a google.golang.org/api/drive/v3 con token OAuth desde identity.oauth_connections.
// En tests se usa MockClient.
type Client interface {
	// CreateFile crea archivo .md en carpeta designada del autor, retorna driveFileID.
	CreateFile(ctx context.Context, userID string, title string, content string) (string, error)
	// GetFileContent descarga contenido Markdown desde Drive usando driveFileID.
	GetFileContent(ctx context.Context, userID string, driveFileID string) (string, error)
	// UpdateFile actualiza contenido o renombra.
	UpdateFile(ctx context.Context, userID string, driveFileID string, newContent *string, newTitle *string) error
	// DeleteFile elimina archivo en Drive.
	DeleteFile(ctx context.Context, userID string, driveFileID string) error
	// UploadAttachment sube binario a carpeta del apunte, retorna externalFileID y fileURL.
	UploadAttachment(ctx context.Context, userID string, noteID string, fileName string, fileType string, data []byte, isInline bool) (string, string, error)
	// DeleteAttachment elimina adjunto en Drive.
	DeleteAttachment(ctx context.Context, userID string, externalFileID string) error
	// CopyFile clona archivo: descarga de src y crea nuevo en Drive de dst.
	CopyFile(ctx context.Context, srcUserID string, srcFileID string, dstUserID string, newTitle string) (string, error)
	// Share helpers (opcional): grant/revoke permisos en Drive vía API.
	GrantPermission(ctx context.Context, ownerUserID string, fileID string, granteeEmail string, role string) error
	RevokePermission(ctx context.Context, ownerUserID string, fileID string, granteeEmail string) error
	RevokeAllPermissions(ctx context.Context, ownerUserID string, fileID string) error
}

// MockClient implementación en memoria para tests y modo StorageMode=mock.
type MockClient struct {
	mu    sync.RWMutex
	files map[string]*mockFile // fileID -> file
	// hooks para inyección de errores en tests
	CreateErr error
	GetErr    map[string]error // fileID -> error
	UpdateErr error
	DeleteErr error
	CopyErr   error
}

type mockFile struct {
	ID      string
	OwnerID string
	Title   string
	Content string
	Folder  string // noteID o "root"
}

func NewMockClient() *MockClient {
	return &MockClient{
		files:  make(map[string]*mockFile),
		GetErr: make(map[string]error),
	}
}

func (m *MockClient) CreateFile(ctx context.Context, userID string, title string, content string) (string, error) {
	if m.CreateErr != nil {
		return "", m.CreateErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	id := "drive_" + uuid.NewString()
	m.files[id] = &mockFile{ID: id, OwnerID: userID, Title: title, Content: content}
	return id, nil
}

func (m *MockClient) GetFileContent(ctx context.Context, userID string, driveFileID string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if err, ok := m.GetErr[driveFileID]; ok && err != nil {
		return "", err
	}
	f, ok := m.files[driveFileID]
	if !ok {
		return "", &DriveError{Code: 404, Message: "Nota no disponible en almacenamiento remoto"}
	}
	// Simular permiso revocado: si owner != requester y no es público, podría ser 403.
	// Para simplificar, solo permitimos lectura si el archivo existe; control de acceso lo hace el Service.
	_ = userID
	return f.Content, nil
}

func (m *MockClient) UpdateFile(ctx context.Context, userID string, driveFileID string, newContent *string, newTitle *string) error {
	if m.UpdateErr != nil {
		return m.UpdateErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.files[driveFileID]
	if !ok {
		return &DriveError{Code: 404, Message: "Nota no disponible en almacenamiento remoto"}
	}
	if f.OwnerID != userID {
		return &DriveError{Code: 403, Message: "Nota no disponible en almacenamiento remoto"}
	}
	if newContent != nil {
		f.Content = *newContent
	}
	if newTitle != nil {
		f.Title = *newTitle
	}
	return nil
}

func (m *MockClient) DeleteFile(ctx context.Context, userID string, driveFileID string) error {
	if m.DeleteErr != nil {
		return m.DeleteErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.files[driveFileID]
	if !ok {
		// idempotente: si ya no existe, no error (pero Drive devolvería 404)
		return nil
	}
	if f.OwnerID != userID {
		return &DriveError{Code: 403, Message: "Nota no disponible en almacenamiento remoto"}
	}
	delete(m.files, driveFileID)
	return nil
}

func (m *MockClient) UploadAttachment(ctx context.Context, userID string, noteID string, fileName string, fileType string, data []byte, isInline bool) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(data) > 10*1024*1024 {
		return "", "", &DriveError{Code: 413, Message: "Archivo muy grande"}
	}
	id := "att_" + uuid.NewString()
	url := fmt.Sprintf("https://drive.google.com/file/d/%s/view", id)
	m.files[id] = &mockFile{ID: id, OwnerID: userID, Title: fileName, Content: string(data), Folder: noteID}
	return id, url, nil
}

func (m *MockClient) DeleteAttachment(ctx context.Context, userID string, externalFileID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.files[externalFileID]
	if !ok {
		return nil
	}
	if f.OwnerID != userID {
		return &DriveError{Code: 403, Message: "Nota no disponible en almacenamiento remoto"}
	}
	delete(m.files, externalFileID)
	return nil
}

func (m *MockClient) CopyFile(ctx context.Context, srcUserID string, srcFileID string, dstUserID string, newTitle string) (string, error) {
	if m.CopyErr != nil {
		return "", m.CopyErr
	}
	m.mu.RLock()
	src, ok := m.files[srcFileID]
	m.mu.RUnlock()
	if !ok {
		return "", &DriveError{Code: 404, Message: "Nota no disponible en almacenamiento remoto"}
	}
	if err, ok := m.GetErr[srcFileID]; ok && err != nil {
		return "", err
	}
	// Clonar contenido
	m.mu.Lock()
	defer m.mu.Unlock()
	newID := "drive_" + uuid.NewString()
	m.files[newID] = &mockFile{ID: newID, OwnerID: dstUserID, Title: newTitle, Content: src.Content}
	_ = srcUserID
	return newID, nil
}

func (m *MockClient) GrantPermission(ctx context.Context, ownerUserID string, fileID string, granteeEmail string, role string) error {
	// mock no-op
	return nil
}
func (m *MockClient) RevokePermission(ctx context.Context, ownerUserID string, fileID string, granteeEmail string) error {
	return nil
}
func (m *MockClient) RevokeAllPermissions(ctx context.Context, ownerUserID string, fileID string) error {
	return nil
}

// Helpers para tests: inyectar errores específicos por fileID
func (m *MockClient) InjectGetError(fileID string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GetErr[fileID] = err
}
func (m *MockClient) ClearGetError(fileID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.GetErr, fileID)
}

// Utility: list files for debugging
func (m *MockClient) FileCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.files)
}
func (m *MockClient) HasFile(fileID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.files[fileID]
	return ok
}

// Ensure interface compliance
var _ Client = (*MockClient)(nil)

// RealDriveClient placeholder: en producción usaría Google Drive API.
// Por ahora delega a Mock para no requerir credenciales reales.
type RealDriveClient struct {
	mock *MockClient
}

func NewRealDriveClient() *RealDriveClient {
	return &RealDriveClient{mock: NewMockClient()}
}
func (r *RealDriveClient) CreateFile(ctx context.Context, userID string, title string, content string) (string, error) {
	// TODO: implementar con driveService.Files.Create(...).Do()
	// Por ahora fallback a mock + log
	if strings.TrimSpace(title) == "" {
		return "", &DriveError{Code: 400, Message: "título vacío"}
	}
	return r.mock.CreateFile(ctx, userID, title, content)
}
func (r *RealDriveClient) GetFileContent(ctx context.Context, userID string, driveFileID string) (string, error) {
	return r.mock.GetFileContent(ctx, userID, driveFileID)
}
func (r *RealDriveClient) UpdateFile(ctx context.Context, userID string, driveFileID string, newContent *string, newTitle *string) error {
	return r.mock.UpdateFile(ctx, userID, driveFileID, newContent, newTitle)
}
func (r *RealDriveClient) DeleteFile(ctx context.Context, userID string, driveFileID string) error {
	return r.mock.DeleteFile(ctx, userID, driveFileID)
}
func (r *RealDriveClient) UploadAttachment(ctx context.Context, userID string, noteID string, fileName string, fileType string, data []byte, isInline bool) (string, string, error) {
	return r.mock.UploadAttachment(ctx, userID, noteID, fileName, fileType, data, isInline)
}
func (r *RealDriveClient) DeleteAttachment(ctx context.Context, userID string, externalFileID string) error {
	return r.mock.DeleteAttachment(ctx, userID, externalFileID)
}
func (r *RealDriveClient) CopyFile(ctx context.Context, srcUserID string, srcFileID string, dstUserID string, newTitle string) (string, error) {
	return r.mock.CopyFile(ctx, srcUserID, srcFileID, dstUserID, newTitle)
}
func (r *RealDriveClient) GrantPermission(ctx context.Context, ownerUserID string, fileID string, granteeEmail string, role string) error {
	return r.mock.GrantPermission(ctx, ownerUserID, fileID, granteeEmail, role)
}
func (r *RealDriveClient) RevokePermission(ctx context.Context, ownerUserID string, fileID string, granteeEmail string) error {
	return r.mock.RevokePermission(ctx, ownerUserID, fileID, granteeEmail)
}
func (r *RealDriveClient) RevokeAllPermissions(ctx context.Context, ownerUserID string, fileID string) error {
	return r.mock.RevokeAllPermissions(ctx, ownerUserID, fileID)
}
