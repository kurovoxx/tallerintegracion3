package drive

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"

	"golang.org/x/oauth2"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// RealDriveClient opera contra la API oficial de Google Drive (drive/v3)
// usando el access_token del usuario dueño del archivo, resuelto vía TokenProvider
// desde identity.oauth_connections. Se activa con STORAGE_MODE=drive.
type RealDriveClient struct {
	tokens TokenProvider
	// newService crea el servicio Drive autenticado; inyectable en tests.
	newService func(ctx context.Context, accessToken string) (*drive.Service, error)
}

// NewRealDriveClient crea el cliente real. tokens no puede ser nil.
func NewRealDriveClient(tokens TokenProvider) *RealDriveClient {
	return &RealDriveClient{
		tokens:     tokens,
		newService: defaultDriveService,
	}
}

func defaultDriveService(ctx context.Context, accessToken string) (*drive.Service, error) {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken})
	hc := oauth2.NewClient(ctx, ts)
	return drive.NewService(ctx, option.WithHTTPClient(hc))
}

// serviceFor resuelve token + servicio para un usuario, con error tipado si no hay conexión.
func (r *RealDriveClient) serviceFor(ctx context.Context, userID string) (*drive.Service, error) {
	if r.tokens == nil {
		return nil, &OAuthError{Message: "Conecte o renueve su Google Drive"}
	}
	token, err := r.tokens.GetValidAccessToken(ctx, userID)
	if err != nil {
		return nil, err
	}
	srv, err := r.newService(ctx, token)
	if err != nil {
		log.Printf("drive: no se pudo crear servicio Drive: %v", err)
		return nil, fmt.Errorf("drive: google_unavailable creando servicio: %w", err)
	}
	return srv, nil
}

// AppFolderName es la carpeta dedicada donde viven todos los archivos de la app
// en el Drive de cada usuario. Permisos mínimos: con scope drive.file la app
// solo ve lo que ella crea; la carpeta única evita regar archivos en la raíz
// y delimita la superficie de escritura.
const AppFolderName = "Apuntes TI3"

// appFolderMarker identifica nuestra carpeta vía appProperties (robusto ante
// renombres por el usuario: se busca por marcador, no por nombre).
func appFolderMarker() map[string]string {
	return map[string]string{"notes_app_folder": "1"}
}

// ensureAppFolder devuelve el id de la carpeta de la app, creándola si no existe.
func (r *RealDriveClient) ensureAppFolder(ctx context.Context, srv *drive.Service) (string, error) {
	list, err := srv.Files.List().
		Q("mimeType = 'application/vnd.google-apps.folder' and appProperties has { key='notes_app_folder' and value='1' } and trashed = false").
		Fields("files(id)").
		PageSize(10).
		Context(ctx).
		Do()
	if err != nil {
		return "", mapGoogleError("EnsureAppFolder/list", err)
	}
	for _, f := range list.Files {
		if strings.TrimSpace(f.Id) != "" {
			return f.Id, nil
		}
	}
	created, err := srv.Files.Create(&drive.File{
		Name:          AppFolderName,
		MimeType:      "application/vnd.google-apps.folder",
		AppProperties: appFolderMarker(),
	}).Fields("id").Context(ctx).Do()
	if err != nil {
		return "", mapGoogleError("EnsureAppFolder/create", err)
	}
	if strings.TrimSpace(created.Id) == "" {
		return "", &DriveError{Code: 500, Message: "Drive no devolvió id de carpeta"}
	}
	log.Printf("drive: carpeta de app creada id=%s", created.Id)
	return created.Id, nil
}

// mapGoogleError traduce errores de la API a DriveError semánticos.
func mapGoogleError(op string, err error) error {
	if err == nil {
		return nil
	}
	var gerr *googleapi.Error
	if !errors.As(err, &gerr) {
		log.Printf("drive: %s falló: %v", op, err)
		return &DriveError{Code: 500, Message: fmt.Sprintf("error en Drive (%s): %v", op, err)}
	}
	switch gerr.Code {
	case 404:
		return &DriveError{Code: 404, Message: "Nota no disponible en almacenamiento remoto"}
	case 401, 403:
		log.Printf("drive: %s permiso denegado (code=%d): %v", op, gerr.Code, err)
		return &DriveError{Code: 403, Message: "Nota no disponible en almacenamiento remoto"}
	case 400:
		return &DriveError{Code: 400, Message: fmt.Sprintf("solicitud inválida a Drive (%s)", op)}
	case 413:
		return &DriveError{Code: 413, Message: "Archivo muy grande"}
	default:
		log.Printf("drive: %s error Google code=%d: %v", op, gerr.Code, err)
		return &DriveError{Code: 500, Message: fmt.Sprintf("error en Drive (%s): %v", op, err)}
	}
}

func (r *RealDriveClient) CreateFile(ctx context.Context, userID string, noteID string, title string, content string) (string, error) {
	if strings.TrimSpace(title) == "" {
		return "", &DriveError{Code: 400, Message: "título vacío"}
	}
	srv, err := r.serviceFor(ctx, userID)
	if err != nil {
		return "", err
	}
	folderID, err := r.ensureAppFolder(ctx, srv)
	if err != nil {
		return "", err
	}
	f, err := srv.Files.Create(&drive.File{Name: title, MimeType: MimeMarkdown, Parents: []string{folderID}, AppProperties: noteFileProperties(userID, noteID)}).
		Media(strings.NewReader(content)).
		Fields("id").
		Context(ctx).
		Do()
	if err != nil {
		return "", mapGoogleError("CreateFile", err)
	}
	log.Printf("drive: archivo creado id=%s", f.Id)
	return f.Id, nil
}

// FindFileByNoteID lista los archivos indexados con notes_note_id=noteID en el
// Drive del dueño (query appProperties) y devuelve el fileID del .md; "" sin
// error cuando no existe. Recupera apuntes cuyo Create crasheó entre el alta
// en Drive y la persistencia del external_file_id.
func (r *RealDriveClient) FindFileByNoteID(ctx context.Context, ownerUserID string, noteID string) (string, error) {
	if strings.TrimSpace(noteID) == "" {
		return "", nil
	}
	srv, err := r.serviceFor(ctx, ownerUserID)
	if err != nil {
		return "", err
	}
	// Excluye adjuntos (notes_attachment=1): solo el .md se adopta como
	// archivo de la nota. Un binario con la misma notes_note_id nunca debe
	// devolverse aquí (rompería la recuperación de huérfanos con bytes).
	query := fmt.Sprintf(
		"appProperties has { key='notes_note_id' and value='%s' } and not appProperties has { key='notes_attachment' } and trashed = false",
		escapeDriveQueryValue(noteID),
	)
	list, err := srv.Files.List().Q(query).Fields("files(id,appProperties)").PageSize(100).Context(ctx).Do()
	if err != nil {
		return "", mapGoogleError("FindFileByNoteID", err)
	}
	best := ""
	for _, f := range list.Files {
		if f.AppProperties["notes_owner_user_id"] != ownerUserID {
			continue
		}
		if best == "" || f.Id < best {
			best = f.Id
		}
	}
	return best, nil
}

// escapeDriveQueryValue neutraliza comillas simples dentro de un literal de la
// query de Drive (defensivo: los IDs son UUID, nunca deberían contenerlas).
func escapeDriveQueryValue(value string) string {
	return strings.ReplaceAll(value, "'", "\\'")
}

func (r *RealDriveClient) GetFileContent(ctx context.Context, userID string, driveFileID string) (string, error) {
	srv, err := r.serviceFor(ctx, userID)
	if err != nil {
		return "", err
	}
	resp, err := srv.Files.Get(driveFileID).Download()
	if err != nil {
		return "", mapGoogleError("GetFileContent", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", &DriveError{Code: 500, Message: fmt.Sprintf("error leyendo contenido de Drive: %v", err)}
	}
	return string(data), nil
}

func (r *RealDriveClient) UpdateFile(ctx context.Context, userID string, driveFileID string, newContent *string, newTitle *string) error {
	srv, err := r.serviceFor(ctx, userID)
	if err != nil {
		return err
	}
	meta := &drive.File{}
	if newTitle != nil {
		meta.Name = *newTitle
	}
	call := srv.Files.Update(driveFileID, meta)
	if newContent != nil {
		call = call.Media(strings.NewReader(*newContent))
	}
	if _, err := call.Context(ctx).Do(); err != nil {
		return mapGoogleError("UpdateFile", err)
	}
	return nil
}

func (r *RealDriveClient) DeleteFile(ctx context.Context, userID string, driveFileID string) error {
	srv, err := r.serviceFor(ctx, userID)
	if err != nil {
		return err
	}
	if err := srv.Files.Delete(driveFileID).Context(ctx).Do(); err != nil {
		if IsNotFound(mapGoogleError("DeleteFile", err)) {
			return nil // idempotente, igual que MockClient
		}
		return mapGoogleError("DeleteFile", err)
	}
	return nil
}

func (r *RealDriveClient) UploadAttachment(ctx context.Context, userID string, noteID string, fileName string, fileType string, data []byte, isInline bool) (string, string, error) {
	if len(data) > 10*1024*1024 {
		return "", "", &DriveError{Code: 413, Message: "Archivo muy grande"}
	}
	if strings.TrimSpace(fileName) == "" {
		fileName = "attachment"
	}
	// Preserva el MIME declarado cuando es específico; si viene vacío o
	// genérico (application/octet-stream) lo resuelve por extensión y, como
	// último recurso, por sniffing del binario. Así Drive guarda
	// image/jpeg, image/png, application/pdf o text/markdown correctos.
	fileType = DetectMimeType(fileName, fileType, data)
	// Garantiza nombre con extensión coherente (imagen.png, documento.pdf):
	// sin ella Drive muestra el binario como octet-stream sin previsualización.
	fileName = ensureAttachmentFileName(fileName, fileType)
	srv, err := r.serviceFor(ctx, userID)
	if err != nil {
		return "", "", err
	}
	folderID, err := r.ensureAppFolder(ctx, srv)
	if err != nil {
		return "", "", err
	}
	// El binario se crea junto al .md en la carpeta de la app y se indexa con
	// las mismas appProperties (dueño + nota) que el Markdown, de modo que
	// ambos archivos quedan ligados a la misma nota en Google Drive. UploadToDrive
	// lo invoca con noteID vacío (nota aún sin crear) y AddAttachment lo vincula
	// después; cuando el noteID existe, el adjunto nace ya asociado.
	props := attachmentFileProperties(userID, noteID)
	_ = isInline // la clase inline/adjunto vive en Postgres, no en Drive.
	f, err := srv.Files.Create(&drive.File{
		Name:          fileName,
		MimeType:      fileType,
		Parents:       []string{folderID},
		AppProperties: props,
	}).
		Media(bytes.NewReader(data)).
		Fields("id, webViewLink").
		Context(ctx).
		Do()
	if err != nil {
		return "", "", mapGoogleError("UploadAttachment", err)
	}
	url := f.WebViewLink
	if url == "" {
		url = fmt.Sprintf("https://drive.google.com/file/d/%s/view", f.Id)
	}
	return f.Id, url, nil
}

func (r *RealDriveClient) DeleteAttachment(ctx context.Context, userID string, externalFileID string) error {
	return r.DeleteFile(ctx, userID, externalFileID)
}

func (r *RealDriveClient) CopyFile(ctx context.Context, srcUserID string, srcFileID string, dstUserID string, newNoteID string, newTitle string) (string, error) {
	// Desacoplado del token del autor: la clonación de apuntes públicos no debe
	// romperse si el autor revocó Drive o su token expiró sin refresh. Se exige
	// OAuth válido solo del clonador (dst). La descarga se intenta primero con
	// el servicio del clonador (funciona si el archivo es público o fue
	// compartido restricted con permiso reader); solo como fallback se intenta
	// con el token del autor para archivos aún no compartidos en Drive.
	dstSrv, err := r.serviceFor(ctx, dstUserID)
	if err != nil {
		return "", err
	}
	data, err := downloadWith(dstSrv, srcFileID)
	if err != nil {
		srcSrv, errSrc := r.serviceFor(ctx, srcUserID)
		if errSrc != nil {
			// Sin token del autor tampoco hay fallback: retornar el error
			// original del clonador (más relevante para el usuario que clona).
			return "", err
		}
		dataFallback, errFallback := downloadWith(srcSrv, srcFileID)
		if errFallback != nil {
			return "", errFallback
		}
		data = dataFallback
	}
	dstFolderID, err := r.ensureAppFolder(ctx, dstSrv)
	if err != nil {
		return "", err
	}
	f, err := dstSrv.Files.Create(&drive.File{Name: newTitle, MimeType: MimeMarkdown, Parents: []string{dstFolderID}, AppProperties: noteFileProperties(dstUserID, newNoteID)}).
		Media(bytes.NewReader(data)).
		Fields("id").
		Context(ctx).
		Do()
	if err != nil {
		return "", mapGoogleError("CopyFile/create", err)
	}
	log.Printf("drive: archivo copiado %s -> %s", srcFileID, f.Id)
	return f.Id, nil
}

// downloadWith descarga el contenido crudo de un archivo con errores mapeados.
func downloadWith(srv *drive.Service, fileID string) ([]byte, error) {
	resp, err := srv.Files.Get(fileID).Download()
	if err != nil {
		return nil, mapGoogleError("CopyFile/download", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &DriveError{Code: 500, Message: fmt.Sprintf("error leyendo original de Drive: %v", err)}
	}
	return data, nil
}

func (r *RealDriveClient) VerifyFileAccess(ctx context.Context, userID string, driveFileID string) error {
	srv, err := r.serviceFor(ctx, userID)
	if err != nil {
		return err
	}
	_, err = srv.Files.Get(driveFileID).Fields("id").Context(ctx).Do()
	if err != nil {
		return mapGoogleError("VerifyFileAccess", err)
	}
	return nil
}

// ListAppFileIDs lista en 1 llamada (paginada) los archivos activos de la app
// del dueño para reconciliación eficiente. Solo indexados con
// notes_owner_user_id; la papelera se excluye igual que en FindFileByNoteID.
func (r *RealDriveClient) ListAppFileIDs(ctx context.Context, userID string) ([]string, error) {
	srv, err := r.serviceFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(
		"appProperties has { key='notes_owner_user_id' and value='%s' } and trashed = false",
		escapeDriveQueryValue(userID),
	)
	var ids []string
	pageToken := ""
	for {
		call := srv.Files.List().Q(query).Fields("files(id)", "nextPageToken").PageSize(1000).Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		list, err := call.Do()
		if err != nil {
			return nil, mapGoogleError("ListAppFileIDs", err)
		}
		for _, f := range list.Files {
			ids = append(ids, f.Id)
		}
		if list.NextPageToken == "" {
			break
		}
		pageToken = list.NextPageToken
	}
	return ids, nil
}

// FileGone confirma ausencia definitiva sin descargar contenido: 404 de la
// API o papelera (trashed=true, que para la app cuenta como eliminado porque
// el usuario lo borró en Drive). Cualquier otro error se propaga para no
// borrar metadata ante fallos temporales.
func (r *RealDriveClient) FileGone(ctx context.Context, userID string, driveFileID string) (bool, error) {
	srv, err := r.serviceFor(ctx, userID)
	if err != nil {
		return false, err
	}
	f, err := srv.Files.Get(driveFileID).Fields("id,trashed").Context(ctx).Do()
	if err != nil {
		if IsNotFound(mapGoogleError("FileGone", err)) {
			return true, nil
		}
		return false, mapGoogleError("FileGone", err)
	}
	return f.Trashed, nil
}

func (r *RealDriveClient) GrantPermission(ctx context.Context, ownerUserID string, fileID string, granteeEmail string, role string) error {
	srv, err := r.serviceFor(ctx, ownerUserID)
	if err != nil {
		return err
	}
	_, err = srv.Permissions.Create(fileID, &drive.Permission{
		Type: "user", Role: role, EmailAddress: granteeEmail,
	}).SendNotificationEmail(false).Context(ctx).Do()
	if err != nil {
		return mapGoogleError("GrantPermission", err)
	}
	return nil
}

func (r *RealDriveClient) GrantLinkPermission(ctx context.Context, ownerUserID, fileID string) (string, error) {
	srv, err := r.serviceFor(ctx, ownerUserID)
	if err != nil {
		return "", err
	}
	p, err := srv.Permissions.Create(fileID, &drive.Permission{
		Type: "anyone", Role: "reader", AllowFileDiscovery: false,
		ForceSendFields: []string{"AllowFileDiscovery"},
	}).Fields("id").Context(ctx).Do()
	if err != nil {
		return "", mapGoogleError("GrantLinkPermission", err)
	}
	if p.Id == "" {
		return "", &DriveError{Code: 500, Message: "Drive returned an empty permission ID"}
	}
	return p.Id, nil
}

func (r *RealDriveClient) RevokePermissionByID(ctx context.Context, ownerUserID, fileID, permissionID string) error {
	srv, err := r.serviceFor(ctx, ownerUserID)
	if err != nil {
		return err
	}
	ids := []string{permissionID}
	if permissionID == "" {
		ids = nil
		page := ""
		for {
			list, err := srv.Permissions.List(fileID).Fields("nextPageToken,permissions(id,type,role)").PageToken(page).Context(ctx).Do()
			if err != nil {
				mapped := mapGoogleError("RevokePermissionByID/list", err)
				if IsNotFound(mapped) {
					return nil
				}
				return mapped
			}
			for _, p := range list.Permissions {
				if p.Type == "anyone" && p.Role != "owner" {
					ids = append(ids, p.Id)
				}
			}
			page = list.NextPageToken
			if page == "" {
				break
			}
		}
	}
	for _, id := range ids {
		if err := srv.Permissions.Delete(fileID, id).Context(ctx).Do(); err != nil {
			mapped := mapGoogleError("RevokePermissionByID/delete", err)
			if !IsNotFound(mapped) {
				return mapped
			}
		}
	}
	return nil
}

func (r *RealDriveClient) RevokePermission(ctx context.Context, ownerUserID string, fileID string, granteeEmail string) error {
	srv, err := r.serviceFor(ctx, ownerUserID)
	if err != nil {
		return err
	}
	page := ""
	for {
		list, err := srv.Permissions.List(fileID).Fields("nextPageToken,permissions(id,emailAddress,role)").PageToken(page).Context(ctx).Do()
		if err != nil {
			return mapGoogleError("RevokePermission/list", err)
		}
		for _, p := range list.Permissions {
			if p.Role != "owner" && strings.EqualFold(p.EmailAddress, granteeEmail) {
				if err := srv.Permissions.Delete(fileID, p.Id).Context(ctx).Do(); err != nil {
					mapped := mapGoogleError("RevokePermission/delete", err)
					if !IsNotFound(mapped) {
						return mapped
					}
				}
				return nil
			}
		}
		page = list.NextPageToken
		if page == "" {
			break
		}
	}
	return nil // idempotente: permiso no encontrado
}

func (r *RealDriveClient) RevokeAllPermissions(ctx context.Context, ownerUserID string, fileID string) error {
	srv, err := r.serviceFor(ctx, ownerUserID)
	if err != nil {
		return err
	}
	list, err := srv.Permissions.List(fileID).Fields("permissions(id,role)").Context(ctx).Do()
	if err != nil {
		return mapGoogleError("RevokeAllPermissions/list", err)
	}
	for _, p := range list.Permissions {
		if p.Role == "owner" {
			continue // al owner no se le puede quitar
		}
		if err := srv.Permissions.Delete(fileID, p.Id).Context(ctx).Do(); err != nil {
			return mapGoogleError("RevokeAllPermissions/delete", err)
		}
	}
	return nil
}

// Ensure interface compliance
var _ Client = (*RealDriveClient)(nil)
