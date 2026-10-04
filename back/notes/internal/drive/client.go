package drive

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// DriveError representa error de la API de Drive con código HTTP semántico.
type DriveError struct {
	Code    int // 403, 404, 413, 500
	Message string
}

func (e *DriveError) Error() string { return fmt.Sprintf("drive %d: %s", e.Code, e.Message) }

// OAuthError representa falta de conexión OAuth vigente (semántica 403).
// Se usa en lugar de fmt.Errorf con texto interno ("no oauth connection found...")
// para que el handler pueda mapear a 403 genérico ("Conecte o renueve su
// Google Drive") sin filtrar detalles internos al frontend. El Message es
// deliberadamente genérico y seguro para logs; nunca incluye DSNs, tokens ni
// trazas de BD.
type OAuthError struct {
	Message string
}

func (e *OAuthError) Error() string { return "oauth 403: " + e.Message }

// IsOAuthError reporta si err es (o envuelve) un *OAuthError.
func IsOAuthError(err error) bool {
	var oe *OAuthError
	return errors.As(err, &oe)
}

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
	DownloadAttachment(ctx context.Context, ownerID, fileID string) ([]byte, string, error)
	// CreateFile crea archivo .md en carpeta designada del autor, retorna driveFileID.
	// El archivo se indexa con appProperties (notes_note_id, notes_owner_user_id)
	// para poder recuperarlo si el proceso crashea antes de persistir el fileID.
	CreateFile(ctx context.Context, userID string, noteID string, title string, content string) (string, error)
	// GetFileContent descarga contenido Markdown desde Drive usando driveFileID.
	GetFileContent(ctx context.Context, userID string, driveFileID string) (string, error)
	// UpdateFile actualiza contenido o renombra.
	UpdateFile(ctx context.Context, userID string, driveFileID string, newContent *string, newTitle *string) error
	// DeleteFile elimina archivo en Drive.
	DeleteFile(ctx context.Context, userID string, driveFileID string) error
	// UploadAttachment sube binario a carpeta del apunte, retorna externalFileID y fileURL.
	// Preserva el MIME declarado cuando es específico (image/jpeg, image/png,
	// application/pdf, text/markdown); si viene vacío o application/octet-stream
	// lo resuelve por extensión y sniffing del contenido.
	UploadAttachment(ctx context.Context, userID string, noteID string, fileName string, fileType string, data []byte, isInline bool) (string, string, error)
	// DeleteAttachment elimina adjunto en Drive.
	DeleteAttachment(ctx context.Context, userID string, externalFileID string) error
	// CopyFile clona archivo: descarga de src y crea nuevo en Drive de dst. El
	// clon se indexa con appProperties de la nueva nota (newNoteID) y su dueño.
	CopyFile(ctx context.Context, srcUserID string, srcFileID string, dstUserID string, newNoteID string, newTitle string) (string, error)
	// FindFileByNoteID busca el archivo .md asociado a una nota por su
	// appProperties notes_note_id (dueño = ownerUserID) y devuelve su fileID;
	// "" sin error cuando no existe. Permite recuperar huérfanos tras un crash.
	FindFileByNoteID(ctx context.Context, ownerUserID string, noteID string) (string, error)
	// Share helpers (opcional): grant/revoke permisos en Drive vía API.
	GrantPermission(ctx context.Context, ownerUserID string, fileID string, granteeEmail string, role string) error
	GrantLinkPermission(ctx context.Context, ownerUserID string, fileID string) (string, error)
	// An empty permissionID resolves and revokes the current anyone permission.
	// This permits cleanup after a restart without persisting a Google permission ID.
	RevokePermissionByID(ctx context.Context, ownerUserID string, fileID string, permissionID string) error
	RevokePermission(ctx context.Context, ownerUserID string, fileID string, granteeEmail string) error
	RevokeAllPermissions(ctx context.Context, ownerUserID string, fileID string) error
	// VerifyFileAccess comprueba que el usuario puede acceder al archivo.
	VerifyFileAccess(ctx context.Context, userID string, driveFileID string) error
}

// GrantCall registra un intento de GrantPermission (observable en tests).
type GrantCall struct {
	FileID string
	Email  string
	Role   string
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
	// GrantCalls registra cada GrantPermission; GrantErr inyecta fallo por email.
	GrantCalls     []GrantCall
	GrantErr       map[string]error // email normalizado o crudo -> error
	LinkGrantCalls []string
	LinkGrantErr   error
	RevokeCalls    []GrantCall
	RevokeIDCalls  []GrantCall
	RevokeErr      error
	permissions    map[string]map[string]string // fileID -> permissionID -> email (empty for anyone)
	// NoOAuth simula usuarios sin conexión OAuth (comportamiento RealDriveClient:
	// serviceFor falla con "no oauth connection found"). Cuando un userID está
	// marcado, las operaciones que requieren su token fallan igual que en prod.
	// CopyFile solo exige OAuth del destino (dst), replicando el Real desacoplado
	// que no requiere token del autor original para clonar apuntes públicos.
	NoOAuth map[string]bool
}

type mockFile struct {
	Data          []byte
	ID            string
	OwnerID       string
	Title         string
	Content       string
	MimeType      string
	Folder        string // noteID o "root"
	AppProperties map[string]string
}

// noteFileProperties indexa el archivo .md con su nota y dueño. notes_note_id
// es la clave de recuperación tras un crash (FindFileByNoteID); se omite si la
// nota aún no tiene ID para no indexar archivos anónimos.
func noteFileProperties(ownerUserID, noteID string) map[string]string {
	props := map[string]string{"notes_owner_user_id": ownerUserID}
	if strings.TrimSpace(noteID) != "" {
		props["notes_note_id"] = noteID
	}
	return props
}

// attachmentFileProperties indexa el binario adjunto con su dueño y, cuando se
// conoce, con la nota destino (notes_note_id). Así el adjunto queda asociado a
// la misma carpeta lógica de la app que el .md y puede auditarse/limpiarse por
// nota sin depender solo de la fila de Postgres.
func attachmentFileProperties(ownerUserID, noteID string) map[string]string {
	props := noteFileProperties(ownerUserID, noteID)
	props["notes_attachment"] = "1"
	return props
}

// ensureAttachmentFileName garantiza una extensión coherente con el MIME
// canónico (imagen.png, documento.pdf, apunte.md) para que Google Drive
// previsualice e identifique el binario aunque el cliente no haya enviado
// extensión en el nombre.
func ensureAttachmentFileName(name, mimeType string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "adjunto"
	}
	if strings.Contains(filepath.Base(name), ".") {
		return name
	}
	switch normalizeMimeType(mimeType) {
	case MimePNG:
		return name + ".png"
	case MimeJPEG:
		return name + ".jpg"
	case MimePDF:
		return name + ".pdf"
	case MimeMarkdown:
		return name + ".md"
	default:
		return name
	}
}

func cloneProperties(props map[string]string) map[string]string {
	if props == nil {
		return nil
	}
	cop := make(map[string]string, len(props))
	for k, v := range props {
		cop[k] = v
	}
	return cop
}

func NewMockClient() *MockClient {
	return &MockClient{
		files:       make(map[string]*mockFile),
		GetErr:      make(map[string]error),
		permissions: make(map[string]map[string]string),
	}
}

// checkOAuth replica serviceFor: falla si el usuario no tiene OAuth vinculado.
func (m *MockClient) checkOAuth(userID string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.NoOAuth != nil && m.NoOAuth[userID] {
		return &OAuthError{Message: "Conecte o renueve su Google Drive"}
	}
	return nil
}

// Disconnect marca a un usuario sin OAuth (simula fila ausente/revocada en
// identity.oauth_connections). Reconnect revierte el efecto.
func (m *MockClient) Disconnect(userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.NoOAuth == nil {
		m.NoOAuth = make(map[string]bool)
	}
	m.NoOAuth[userID] = true
}
func (m *MockClient) Reconnect(userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.NoOAuth != nil {
		delete(m.NoOAuth, userID)
	}
}

func (m *MockClient) CreateFile(ctx context.Context, userID string, noteID string, title string, content string) (string, error) {
	if err := m.checkOAuth(userID); err != nil {
		return "", err
	}
	if m.CreateErr != nil {
		return "", m.CreateErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	id := "drive_" + uuid.NewString()
	m.files[id] = &mockFile{ID: id, OwnerID: userID, Title: title, Content: content, MimeType: MimeMarkdown, AppProperties: noteFileProperties(userID, noteID)}
	return id, nil
}

func (m *MockClient) GetFileContent(ctx context.Context, userID string, driveFileID string) (string, error) {
	if err := m.checkOAuth(userID); err != nil {
		return "", err
	}
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
	if err := m.checkOAuth(userID); err != nil {
		return err
	}
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
	if err := m.checkOAuth(userID); err != nil {
		// Compensación y borrados en cascada no deben fallar por OAuth ausente:
		// si el archivo no requiere token válido (ya huérfano o borrado), la
		// limpieza debe ser best-effort. Solo se propaga el error OAuth cuando
		// el archivo aún existe y pertenece al usuario.
		m.mu.RLock()
		f, ok := m.files[driveFileID]
		m.mu.RUnlock()
		if !ok {
			return nil
		}
		if f.OwnerID == userID {
			// Permitir borrado compensatorio aun sin OAuth para evitar huérfanos.
			m.mu.Lock()
			delete(m.files, driveFileID)
			m.mu.Unlock()
			return nil
		}
		return err
	}
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
	if err := m.checkOAuth(userID); err != nil {
		return "", "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(data) > 10*1024*1024 {
		return "", "", &DriveError{Code: 413, Message: "Archivo muy grande"}
	}
	if strings.TrimSpace(fileName) == "" {
		fileName = "attachment"
	}
	// Paridad con RealDriveClient: preserva el MIME declarado si es específico
	// y lo resuelve por extensión/sniffing cuando viene vacío o genérico, y
	// garantiza una extensión coherente con el tipo para el nombre en Drive.
	fileType = DetectMimeType(fileName, fileType, data)
	fileName = ensureAttachmentFileName(fileName, fileType)
	id := "att_" + uuid.NewString()
	url := fmt.Sprintf("https://drive.google.com/file/d/%s/view", id)
	m.files[id] = &mockFile{
		ID:            id,
		OwnerID:       userID,
		Title:         fileName,
		Data:          append([]byte(nil), data...),
		MimeType:      fileType,
		Folder:        noteID,
		AppProperties: attachmentFileProperties(userID, noteID),
	}
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

func (m *MockClient) CopyFile(ctx context.Context, srcUserID string, srcFileID string, dstUserID string, newNoteID string, newTitle string) (string, error) {
	// Paridad con RealDriveClient desacoplado: solo se exige OAuth del destino.
	// El token del autor original NO es requerido (permite clonar apuntes
	// públicos aunque el autor haya revocado Drive).
	if err := m.checkOAuth(dstUserID); err != nil {
		return "", err
	}
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
	m.files[newID] = &mockFile{ID: newID, OwnerID: dstUserID, Title: newTitle, Content: src.Content, MimeType: MimeMarkdown, AppProperties: noteFileProperties(dstUserID, newNoteID)}
	_ = srcUserID
	return newID, nil
}

// FindFileByNoteID replica Files.List con query appProperties: devuelve el
// archivo .md del dueño indexado con notes_note_id=noteID ("" si no existe).
// Excluye los adjuntos (notes_attachment=1) para no adoptar un binario como si
// fuera el Markdown de la nota. Ante varios candidatos (p. ej. reintentos
// previos) elige el de ID menor para ser determinista.
func (m *MockClient) FindFileByNoteID(ctx context.Context, ownerUserID string, noteID string) (string, error) {
	if err := m.checkOAuth(ownerUserID); err != nil {
		return "", err
	}
	if strings.TrimSpace(noteID) == "" {
		return "", nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	found := ""
	for id, f := range m.files {
		if f.OwnerID != ownerUserID || f.AppProperties["notes_note_id"] != noteID {
			continue
		}
		if f.AppProperties["notes_attachment"] == "1" {
			continue
		}
		if found == "" || id < found {
			found = id
		}
	}
	return found, nil
}

func (m *MockClient) VerifyFileAccess(ctx context.Context, userID string, driveFileID string) error {
	if err := m.checkOAuth(userID); err != nil {
		return err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if err, ok := m.GetErr[driveFileID]; ok && err != nil {
		return err
	}
	f, ok := m.files[driveFileID]
	if !ok {
		return &DriveError{Code: 404, Message: "Archivo no encontrado"}
	}
	// Trust boundary: el archivo debe pertenecer al usuario que lo declara.
	// En producción RealDriveClient.VerifyFileAccess hace Files.Get con el token
	// del usuario: si el fileID es ajeno y no compartido, Drive responde 403.
	// El mock replica esa semántica verificando OwnerID.
	if f.OwnerID != userID {
		return &DriveError{Code: 403, Message: "Nota no disponible en almacenamiento remoto"}
	}
	return nil
}

func (m *MockClient) GrantPermission(ctx context.Context, ownerUserID string, fileID string, granteeEmail string, role string) error {
	if err := m.checkOAuth(ownerUserID); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GrantCalls = append(m.GrantCalls, GrantCall{FileID: fileID, Email: granteeEmail, Role: role})
	if m.GrantErr != nil {
		if err, ok := m.GrantErr[granteeEmail]; ok && err != nil {
			return err
		}
	}
	m.setPermission(fileID, granteeEmail, granteeEmail)
	return nil
}
func (m *MockClient) setPermission(fileID, id, email string) {
	if m.permissions == nil {
		m.permissions = make(map[string]map[string]string)
	}
	if m.permissions[fileID] == nil {
		m.permissions[fileID] = make(map[string]string)
	}
	m.permissions[fileID][id] = email
}

func (m *MockClient) GrantLinkPermission(ctx context.Context, ownerUserID, fileID string) (string, error) {
	if err := m.checkOAuth(ownerUserID); err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LinkGrantCalls = append(m.LinkGrantCalls, fileID)
	if m.LinkGrantErr != nil {
		return "", m.LinkGrantErr
	}
	for id, email := range m.permissions[fileID] {
		if email == "" {
			return id, nil
		}
	}
	id := "permission_" + uuid.NewString()
	m.setPermission(fileID, id, "")
	return id, nil
}

func (m *MockClient) RevokePermissionByID(ctx context.Context, ownerUserID, fileID, permissionID string) error {
	if err := m.checkOAuth(ownerUserID); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RevokeIDCalls = append(m.RevokeIDCalls, GrantCall{FileID: fileID, Email: permissionID})
	if m.RevokeErr != nil {
		return m.RevokeErr
	}
	if permissionID == "" {
		for id, email := range m.permissions[fileID] {
			if email == "" {
				delete(m.permissions[fileID], id)
			}
		}
	} else {
		delete(m.permissions[fileID], permissionID)
	}
	return nil
}

func (m *MockClient) RevokePermission(ctx context.Context, ownerUserID string, fileID string, granteeEmail string) error {
	if err := m.checkOAuth(ownerUserID); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RevokeCalls = append(m.RevokeCalls, GrantCall{FileID: fileID, Email: granteeEmail})
	if m.RevokeErr != nil {
		return m.RevokeErr
	}
	delete(m.permissions[fileID], granteeEmail)
	return nil
}
func (m *MockClient) RevokeAllPermissions(ctx context.Context, ownerUserID string, fileID string) error {
	if err := m.checkOAuth(ownerUserID); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.RevokeErr != nil {
		return m.RevokeErr
	}
	delete(m.permissions, fileID)
	return nil
}

// InjectGrantError inyecta un fallo de GrantPermission para un email dado.
func (m *MockClient) InjectGrantError(email string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.GrantErr == nil {
		m.GrantErr = make(map[string]error)
	}
	m.GrantErr[email] = err
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

// FileMimeType expone el MIME con el que se almacenó un archivo (solo tests).
func (m *MockClient) FileMimeType(fileID string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	f, ok := m.files[fileID]
	if !ok {
		return "", false
	}
	return f.MimeType, true
}

// FileAppProperties expone las appProperties con las que se almacenó un
// archivo (solo tests): permite verificar notes_note_id / notes_owner_user_id.
func (m *MockClient) FileAppProperties(fileID string) (map[string]string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	f, ok := m.files[fileID]
	if !ok {
		return nil, false
	}
	return cloneProperties(f.AppProperties), true
}

// Ensure interface compliance
var _ Client = (*MockClient)(nil)
