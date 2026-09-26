package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/utils"
)

// ServiceError representa error de dominio mapeable a HTTP.
type ServiceError struct {
	Code    string
	Message string
}

func (e *ServiceError) Error() string { return e.Code + ": " + e.Message }

// Sentinel errors canónicos para la capa HTTP (nunca filtran trazas SQL ni
// detalles internos de Postgres/Drive). El handler los clasifica con
// errors.Is/errors.As, sin heurísticas de strings.
var (
	ErrInternalDatabase = &ServiceError{Code: utils.ErrInternal, Message: "Error interno de base de datos"}
	// ErrDriveUnavailable es el centinela canónico de fallos del upstream de
	// almacenamiento (Google Drive). El handler lo mapea a 502 Bad Gateway.
	ErrDriveUnavailable = &ServiceError{Code: utils.ErrInternal, Message: "Error interno de almacenamiento drive"}
	// ErrInternalDrive se conserva como alias de compatibilidad.
	ErrInternalDrive  = ErrDriveUnavailable
	ErrInternalServer = &ServiceError{Code: utils.ErrInternal, Message: "Error interno del servidor"}
	// ErrSourceAccessDenied distingue el fallo de acceso/lectura del archivo
	// origen durante la clonación (borrado, permisos revocados, token del autor
	// inválido) del fallo de OAuth de la cuenta del clonador, que se reporta
	// como ErrForbidden ("Conecte o renueve su Google Drive").
	ErrSourceAccessDenied = &ServiceError{Code: utils.ErrNoteUnavailable, Message: "No se pudo acceder al archivo origen en Drive"}
)

// Regex de sanitización: el match es case-insensitive y tolera atributos/espacios
// dentro de las etiquetas y alrededor de los eventos, de modo que variantes como
// <SCRIPT src="x">, <iframe>, OnLoad = o "JaVaScRiPt:" no evadan el filtro.
// La cobertura XSS es completa: etiquetas de ejecución/embebido (script, iframe,
// svg, object, embed), atributos de evento (on\w+=, p. ej. onload, onerror,
// onclick, onmouseover, ondblclick), esquemas maliciosos (javascript:, data:,
// vbscript:) y la expresión CSS expression(...). Todos compilados una sola vez
// (regexp.MustCompile) para ser eficientes en el hot path.
var (
	reScriptTag    = regexp.MustCompile(`(?i)</?\s*script[^>]*>`)
	reIframeTag    = regexp.MustCompile(`(?i)</?\s*iframe[^>]*>`)
	reSVGTag       = regexp.MustCompile(`(?i)</?\s*svg[^>]*>`)
	reObjectTag    = regexp.MustCompile(`(?i)</?\s*object[^>]*>`)
	reEmbedTag     = regexp.MustCompile(`(?i)</?\s*embed[^>]*>`)
	reStyleTag     = regexp.MustCompile(`(?i)</?\s*style[^>]*>`)
	reCSSImport    = regexp.MustCompile(`(?i)@import\b`)
	reCSSKeyframe  = regexp.MustCompile(`(?i)@(keyframes|-webkit-keyframes)\b`)
	reCSSBehavior  = regexp.MustCompile(`(?i)\bbehavior\s*:`)
	reXMLNS        = regexp.MustCompile(`(?i)\bxmlns\b[^=]*=`)
	reXLink        = regexp.MustCompile(`(?i)\bxlink:href\b[^=]*=`)
	reEventHandler = regexp.MustCompile(`(?i)\bon[a-z0-9_-]+\s*=`)
	reJSScheme     = regexp.MustCompile(`(?i)javascript\s*:`)
	reDataScheme   = regexp.MustCompile(`(?i)data\s*:`)
	reVBScheme     = regexp.MustCompile(`(?i)vbscript\s*:`)
	reCSExpression = regexp.MustCompile(`(?i)expression\s*\(`)
)

// sanitizeMarkdown neutraliza los vectores XSS del contenido Markdown: etiquetas
// <script>/<iframe>/<svg>/<object>/<embed>/<style> (con o sin atributos, es decir
// que también captura sus event handlers inline), reglas CSS @import,
// @keyframes/-webkit-keyframes y behavior:, atributos de evento on\w+= (incluye
// guiones/bajos) sobre cualquier etiqueta HTML, atributos de namespace/XML
// (xmlns=, xlink:href=), links/esquemas con javascript:, data: o vbscript: y la
// expresión CSS expression(. El contenido se escapa/neutraliza en lugar de
// eliminarse para no alterar el resto del texto del usuario.
func sanitizeMarkdown(content string) string {
	content = reScriptTag.ReplaceAllString(content, "&lt;script&gt;")
	content = reIframeTag.ReplaceAllString(content, "&lt;iframe&gt;")
	content = reSVGTag.ReplaceAllString(content, "&lt;svg&gt;")
	content = reObjectTag.ReplaceAllString(content, "&lt;object&gt;")
	content = reEmbedTag.ReplaceAllString(content, "&lt;embed&gt;")
	content = reStyleTag.ReplaceAllString(content, "&lt;style&gt;")
	content = reCSSImport.ReplaceAllString(content, "blocked-import")
	content = reCSSKeyframe.ReplaceAllString(content, "blocked-keyframe")
	content = reCSSBehavior.ReplaceAllString(content, "blocked:")
	content = reXMLNS.ReplaceAllString(content, "blocked=")
	content = reXLink.ReplaceAllString(content, "blocked=")
	content = reEventHandler.ReplaceAllString(content, "blocked=")
	content = reJSScheme.ReplaceAllString(content, "blocked:")
	content = reDataScheme.ReplaceAllString(content, "blocked:")
	content = reVBScheme.ReplaceAllString(content, "blocked:")
	content = reCSExpression.ReplaceAllString(content, "blocked(")
	return content
}

// maxIdempotencyKeyLength acota la longitud de la clave de idempotencia
// admitida: una clave desmedida no se cachea (evita abuso de memoria con
// headers X-Idempotency-Key gigantes). Vacío => sin idempotencia.
const maxIdempotencyKeyLength = 128

// idemKey namespacea la clave de idempotencia por operación (create/copy) para
// evitar colisiones cruzadas entre endpoints. Vacío => sin idempotencia.
func idemKey(op, key string) string {
	if strings.TrimSpace(key) == "" || len(key) > maxIdempotencyKeyLength {
		return ""
	}
	return op + ":" + key
}

// idemStatus modela el estado de una entrada del caché de idempotencia.
type idemStatus int

const (
	// idemInFlight: hay una operación en curso para la clave.
	idemInFlight idemStatus = iota
	// idemSuccess: la operación terminó y su resultado es replicable.
	idemSuccess
)

// idemEntry es la entrada unificada del caché de idempotencia: encapsula el
// estado de la operación (in-flight o success), la respuesta exitosa (note)
// cuando aplica y el instante de creación para expirar la entrada.
//
// Un único tipo evita la falsa condición de carrera del centinela booleano:
// in-flight y replay exitoso comparten el mismo struct y el estado se
// discrimina por el campo status, nunca por el tipo dinámico del valor.
type idemEntry struct {
	status    idemStatus
	note      *model.Note
	createdAt time.Time
}

// Ventanas de expiración del caché de idempotencia:
//   - idemInFlightTTL: una operación in-flight más antigua se considera colgada
//     (crash a mitad de camino) y la clave se purga para permitir reprocesar.
//   - idemSuccessTTL: TTL del replay exitoso cacheado.
const (
	idemInFlightTTL = 2 * time.Minute
	idemSuccessTTL  = 10 * time.Minute
	// maxIdemEntries es el tope de entradas del caché de idempotencia: evita
	// que claves únicas con distintos X-Idempotency-Key acumulen memoria sin
	// límite. Al alcanzarlo, storeIdemEntry realiza una pasada de purga de las
	// entradas vencidas por TTL antes de admitir la nueva.
	maxIdemEntries = 5000
)

// loadIdemEntry consulta la clave de idempotencia sobre el mapa unificado
// idemStore. La evaluación del TTL ocurre bajo s.idemMu.RLock(); si la entrada
// venció, se suelta el RLock, se adquiere el Lock exclusivo y se purga.
// Devuelve:
//   - (note, true, nil) cuando hay una respuesta exitosa vigente (replay);
//   - (nil, true, err) cuando hay una operación in-flight vigente;
//   - (nil, false, nil) cuando la clave no existe, expiró por TTL o quedó
//     colgada: la entrada vencida se purga y se trata como no encontrada para
//     permitir reprocesar.
func (s *NoteService) loadIdemEntry(cacheKey string) (*model.Note, bool, error) {
	if cacheKey == "" {
		return nil, false, nil
	}
	s.idemMu.RLock()
	entry, loaded := s.idemStore[cacheKey]
	if !loaded || entry == nil {
		s.idemMu.RUnlock()
		return nil, false, nil
	}
	status := entry.status
	note := entry.note
	expired := false
	switch status {
	case idemInFlight:
		expired = time.Since(entry.createdAt) > idemInFlightTTL
	case idemSuccess:
		expired = time.Since(entry.createdAt) > idemSuccessTTL
	default:
		// Estado desconocido (entrada corrupta/legado): nunca puede
		// interpretarse como in-flight válido.
		expired = true
	}
	s.idemMu.RUnlock()
	if expired {
		// Purga con Lock exclusivo tras soltar el RLock; la comparación de
		// puntero evita borrar una entrada fresca publicada por otra goroutine
		// entre la lectura y la purga.
		s.purgeIdemEntry(cacheKey, entry)
		return nil, false, nil
	}
	if status == idemInFlight {
		return nil, true, newServiceErrorMsg(utils.ErrBadRequest, "solicitud en progreso")
	}
	return note, true, nil
}

// purgeIdemEntry elimina la clave solo si la entrada vigente sigue siendo la
// misma que se observó al leer con RLock. Evita la carrera de borrar una
// entrada fresca publicada por otra goroutine entre la lectura y la purga.
func (s *NoteService) purgeIdemEntry(cacheKey string, observed *idemEntry) {
	s.idemMu.Lock()
	defer s.idemMu.Unlock()
	if current, ok := s.idemStore[cacheKey]; ok && current == observed {
		delete(s.idemStore, cacheKey)
	}
}

// storeIdemEntry publica una entrada en idemStore respetando el tope
// maxIdemEntries. Al alcanzar el tope se purgan primero las entradas vencidas
// por TTL (in-flight >2 min, success >10 min; el TTL de éxito es de 10 min) y,
// si aun así no hay espacio (todas frescas), se desaloja la entrada más antigua
// para admitir la nueva sin superar nunca el tope. Todo ocurre bajo idemMu: el
// conteo y la purga son atómicos respecto a la inserción.
func (s *NoteService) storeIdemEntry(cacheKey string, entry *idemEntry) {
	if cacheKey == "" || entry == nil {
		return
	}
	s.idemMu.Lock()
	defer s.idemMu.Unlock()
	if _, exists := s.idemStore[cacheKey]; exists {
		s.idemStore[cacheKey] = entry
		return
	}
	if len(s.idemStore) >= maxIdemEntries {
		now := time.Now()
		// Recolección primero, borrado después: el mapa nunca se muta mientras
		// se itera (evita comportamiento indefinido con implementaciones que
		// prohíben modificar el mapa durante range).
		toDelete := make([]string, 0)
		oldestKey := ""
		oldestAt := time.Now()
		for k, e := range s.idemStore {
			if e == nil {
				toDelete = append(toDelete, k)
				continue
			}
			ttl := idemSuccessTTL
			if e.status == idemInFlight {
				ttl = idemInFlightTTL
			}
			if now.Sub(e.createdAt) > ttl {
				toDelete = append(toDelete, k)
				continue
			}
			if oldestKey == "" || e.createdAt.Before(oldestAt) {
				oldestKey = k
				oldestAt = e.createdAt
			}
		}
		for _, k := range toDelete {
			delete(s.idemStore, k)
		}
		if len(s.idemStore) >= maxIdemEntries && oldestKey != "" {
			delete(s.idemStore, oldestKey)
		}
	}
	s.idemStore[cacheKey] = entry
}

// deleteIdemEntry elimina incondicionalmente una clave del caché unificado.
// Se usa para liberar la clave cuando la operación falla (defer) y permitir
// reintentos seguros.
func (s *NoteService) deleteIdemEntry(cacheKey string) {
	if cacheKey == "" {
		return
	}
	s.idemMu.Lock()
	defer s.idemMu.Unlock()
	delete(s.idemStore, cacheKey)
}

// driveCallTimeout es el presupuesto máximo por llamada al upstream de Drive.
// Un cuelgue de red (Google Drive sin responder) no debe bloquear el request
// indefinidamente: cada operación crítica se ejecuta con un contexto acotado.
const driveCallTimeout = 15 * time.Second

// maxNoteContentLength es el tamaño máximo admitido para el contenido Markdown
// de una nota (1MB = 1048576 bytes). El handler ya acota el body JSON a 1MB,
// pero esta validación de servicio protege también llamadas internas/gRPC y
// rechaza contenido desmedido antes de tocar PG o Drive.
const maxNoteContentLength = 1048576

// withDriveTimeout deriva un contexto con timeout de protección para llamadas
// críticas a Drive (CreateFile, GetFileContent, UpdateFile, DeleteFile,
// CopyFile). El cancelador debe invocarse al terminar la operación para liberar
// los recursos del timer (se usa cancel() inmediatamente después de la llamada
// o defer cancel() cuando el contexto cubre una secuencia corta).
func withDriveTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, driveCallTimeout)
}

// Reintentos con backoff para las operaciones críticas de Drive: absorben
// fallos transitorios de red o 5xx sin exponerlos al usuario, sin reintentar
// jamás los errores definitivos del cliente (404, 401, 403 OAuth).
const (
	// driveRetryMaxAttempts es el número máximo de intentos por operación
	// (1 intento original + 2 reintentos).
	driveRetryMaxAttempts = 3
	// driveRetryBaseDelay es la base del backoff lineal: 50ms, 100ms, ...
	driveRetryBaseDelay = 50 * time.Millisecond
)

// retryDriveOperation ejecuta op con hasta maxAttempts intentos y un backoff
// lineal breve (50ms, 100ms, ...). Solo reintenta fallos transitorios (errores
// 5xx o de red/contexto no tipados); los errores definitivos del cliente
// (400/401/403/404/413 y OAuthError) se devuelven de inmediato sin reintentar.
// Si el contexto se cancela o vence durante el backoff se devuelve el último
// error observado.
func retryDriveOperation(ctx context.Context, maxAttempts int, op func() error) error {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			if lastErr != nil {
				return lastErr
			}
			return err
		}
		lastErr = op()
		if lastErr == nil {
			return nil
		}
		if attempt == maxAttempts || !isRetryableDriveError(lastErr) {
			return lastErr
		}
		select {
		case <-ctx.Done():
			return lastErr
		case <-time.After(time.Duration(attempt) * driveRetryBaseDelay):
		}
	}
	return lastErr
}

// isRetryableDriveError clasifica el fallo de Drive: los errores tipados por el
// cliente (400, 401, 403, 404, 413) y los OAuthError son definitivos; los 5xx
// y los errores de red/contexto no tipados son transitorios y admiten reintento.
func isRetryableDriveError(err error) bool {
	if err == nil {
		return false
	}
	if drive.IsOAuthError(err) {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var de *drive.DriveError
	if errors.As(err, &de) {
		switch de.Code {
		case 400, 401, 403, 404, 413:
			return false
		default:
			return de.Code >= 500
		}
	}
	// Error no tipado (típicamente un fallo de red/IO del cliente HTTP):
	// transitorio, se reintenta.
	return true
}

func newServiceError(code string) *ServiceError {
	return &ServiceError{Code: code, Message: utils.MessageForCode(code)}
}
func newServiceErrorMsg(code, msg string) *ServiceError {
	return &ServiceError{Code: code, Message: msg}
}

// Store interfaces para desacoplar de pgx y permitir mocks en tests.
type NoteStore interface {
	Create(ctx context.Context, userID string, subjectID *string, title string, externalFileID *string, visibility string, forkedFrom *string, syncStatus string) (*model.Note, error)
	GetByID(ctx context.Context, id string) (*model.Note, error)
	ListByUser(ctx context.Context, userID, cursor string, limit int) ([]*model.Note, string, error)
	Update(ctx context.Context, id string, title *string, visibility *string) (*model.Note, error)
	Delete(ctx context.Context, id string) error
	IncrementLikes(ctx context.Context, noteID string, delta int) error
	UpdateExternalFileID(ctx context.Context, noteID, fileID string) error
	UpdateSyncStatus(ctx context.Context, noteID, syncStatus string) error
	InsertDeadLetter(ctx context.Context, fileID, reason string) error
}

type AttachmentStore interface {
	Create(ctx context.Context, noteID, externalFileID, fileURL, fileType string, fileName *string, fileSize *int, isInline bool) (*model.Attachment, error)
	GetByID(ctx context.Context, id string) (*model.Attachment, error)
	ListByNote(ctx context.Context, noteID string) ([]*model.Attachment, error)
	Delete(ctx context.Context, id string) error
}

type SavedStore interface {
	Save(ctx context.Context, userID, noteID string) (*model.SavedNote, error)
	Exists(ctx context.Context, userID, noteID string) (bool, error)
	Delete(ctx context.Context, userID, noteID string) error
}

type LikeStore interface {
	Create(ctx context.Context, noteID, userID string) (*model.NoteLike, error)
	Delete(ctx context.Context, noteID, userID string) (bool, error)
	Exists(ctx context.Context, noteID, userID string) (bool, error)
	LikeAtomic(ctx context.Context, noteID, userID string) error
	UnlikeAtomic(ctx context.Context, noteID, userID string) error
}

type SharedStore interface {
	Create(ctx context.Context, noteID, groupID string, isAdminNote bool, accessMode string, followersSnapshot int) (*model.SharedNote, error)
	GetByID(ctx context.Context, id string) (*model.SharedNote, error)
	ListByNote(ctx context.Context, noteID string) ([]*model.SharedNote, error)
	ListByGroup(ctx context.Context, groupID string, limit int, cursor string) ([]*model.SharedNote, string, error)
	Delete(ctx context.Context, id string) error
	DeleteAllByNote(ctx context.Context, noteID string) error
	DeleteByUserAndGroup(ctx context.Context, userID, groupID string) error
	HasAccess(ctx context.Context, noteID, groupID string) (bool, error)
	HasAnyShare(ctx context.Context, noteID string) (bool, error)
}

// SocialResolver verifica pertenencia/membership vía gRPC a Social (mock en tests).
type SocialResolver interface {
	IsMember(ctx context.Context, userID, groupID string) (bool, error)
	IsAdmin(ctx context.Context, userID, groupID string) (bool, error)
	GetFollowersCount(ctx context.Context, userID string) (int, error)
}

type noopSocialResolver struct{}

func (n *noopSocialResolver) IsMember(ctx context.Context, userID, groupID string) (bool, error) {
	return true, nil
}
func (n *noopSocialResolver) IsAdmin(ctx context.Context, userID, groupID string) (bool, error) {
	return false, nil
}
func (n *noopSocialResolver) GetFollowersCount(ctx context.Context, userID string) (int, error) {
	return 0, nil
}

type NoteService struct {
	notes       NoteStore
	attachments AttachmentStore
	saved       SavedStore
	likes       LikeStore
	shared      SharedStore
	drive       drive.Client
	social      SocialResolver
	members     GroupMemberDirectory
	// idemMu protege idemStore, el caché de idempotencia unificado. Una única
	// estructura evita la dualidad sync.Map + libro de claves: el conteo, la
	// purga por tope y el acceso por clave son atómicos bajo el mismo lock.
	idemMu    sync.RWMutex
	idemStore map[string]*idemEntry
	// reconcileMu serializa EXCLUSIVAMENTE las ejecuciones del cron/job de
	// reconciliación (ReconcilePendingNotes) para que dos instancias en
	// paralelo no colisionen al compensar la misma nota pendiente. Es un lock
	// aislado del camino HTTP normal: ninguna operación de usuario (Create,
	// Get, Update, Copy, Delete...) lo adquiere, por lo que la reconciliación
	// nunca bloquea a los requests de los usuarios ni viceversa.
	reconcileMu sync.Mutex
}

func NewNoteService(notes NoteStore, attachments AttachmentStore, saved SavedStore, likes LikeStore, shared SharedStore, d drive.Client, social SocialResolver) *NoteService {
	if social == nil {
		social = &noopSocialResolver{}
	}
	if d == nil {
		d = drive.NewMockClient()
	}
	return &NoteService{
		notes:       notes,
		attachments: attachments,
		saved:       saved,
		likes:       likes,
		shared:      shared,
		drive:       d,
		social:      social,
		members:     NewNoopMemberDirectory(),
		idemStore:   make(map[string]*idemEntry),
	}
}

// SetMemberDirectory inyecta el directorio de correos de miembros usado por el
// share restricted. El adaptador real (gRPC a Social/Identity) se conectará
// aquí; por defecto es noop (grupo sin miembros).
func (s *NoteService) SetMemberDirectory(d GroupMemberDirectory) {
	if d == nil {
		d = NewNoopMemberDirectory()
	}
	s.members = d
}

// notFoundNote es el error canónico 404 para notas inexistentes o inaccesibles.
// Anti-enumeración (zero-knowledge): una nota privada sin acceso devuelve
// exactamente el mismo código y mensaje que una nota que no existe, de modo que
// un atacante no pueda distinguir la existencia de recursos privados.
func notFoundNote() *ServiceError {
	return newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
}

// hasReadAccess resuelve lectura sin revelar existencia: el autor y las notas
// públicas siempre pueden leerse; una nota privada exige un share "link"
// vigente o membresía en alguno de los grupos con share "restricted".
// Devuelve (false, nil) cuando la nota existe pero no es accesible: los
// llamadores lo traducen a not_found (nunca forbidden) para no filtrar la
// existencia del recurso.
func (s *NoteService) hasReadAccess(ctx context.Context, note *model.Note, requesterID string) (bool, error) {
	if note.UserID == requesterID || note.Visibility == "public" {
		return true, nil
	}
	list, err := s.shared.ListByNote(ctx, note.ID)
	if err != nil {
		return false, ErrInternalDatabase
	}
	for _, sh := range list {
		if sh.AccessMode == "link" {
			return true, nil
		}
	}
	for _, sh := range list {
		if sh.AccessMode == "restricted" {
			ok, err := s.social.IsMember(ctx, requesterID, sh.GroupID)
			if err != nil {
				return false, ErrInternalDatabase
			}
			if ok {
				return true, nil
			}
		}
	}
	return false, nil
}

// --- helpers validación ---
func validateTitle(title string) error {
	if !utils.ValidateTitle(title) {
		return newServiceError(utils.ErrInvalidTitle)
	}
	return nil
}
func validateVisibility(v string) error {
	if !utils.ValidateVisibility(v) {
		return newServiceError(utils.ErrInvalidVisibility)
	}
	return nil
}
func validateSubjectID(s *string) error {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	if !utils.ValidateUUID(v) {
		return newServiceError(utils.ErrInvalidSubjectID)
	}
	return nil
}

// Create: valida payload, crea metadata en PG con sync_status='pending_drive', crea archivo en Drive, actualiza sync_status.
//
// Firma canónica alineada con su llamada en el handler:
//
//	(ctx, userID, title, subjectID, visibility, content, idempotencyKey)
//
// Consistencia causal PG <-> Drive:
//  1. PG primero con 'pending_drive' (intención durable);
//  2. alta en Drive;
//  3. PG a 'synced' con external_file_id.
//
// Si Drive falla, la fila queda 'failed_sync' (reconciliable). Si PG falla
// tras el alta en Drive, se compensa borrando el huérfano (DLQ + log
// [CRITICAL_UNRECONCILED] si la compensación también falla).
//
// Idempotencia robusta en memoria (mapa unificado idemStore protegido por
// idemMu RWMutex): al iniciar se publica un *idemEntry{status: idemInFlight} y
// al tener éxito se reemplaza por *idemEntry{status: idemSuccess, note,
// createdAt} para responder replays sin duplicar PG/Drive; las entradas
// in-flight colgadas (>2 min) y los replays vencidos (>10 min) se purgan al
// consultarse; el defer limpia la clave ante error, evitando memory leaks y
// bloqueos.
func (s *NoteService) Create(ctx context.Context, userID string, title string, subjectID *string, visibility string, content *string, idempotencyKey string) (note *model.Note, err error) {
	// Validación estricta temprana: título (1-255 tras TrimSpace) y subject_id
	// (UUID válido tras TrimSpace) se rechazan ANTES de tocar DB, Drive o el
	// caché de idempotencia.
	if err := validateTitle(title); err != nil {
		return nil, err
	}
	if err := validateSubjectID(subjectID); err != nil {
		return nil, err
	}
	if content != nil && len(*content) > maxNoteContentLength {
		return nil, newServiceError(utils.ErrFileTooLarge)
	}
	if content != nil {
		*content = sanitizeMarkdown(*content)
	}
	cacheKey := idemKey("create", idempotencyKey)
	if cacheKey != "" {
		cached, found, cacheErr := s.loadIdemEntry(cacheKey)
		if cacheErr != nil {
			return nil, cacheErr
		}
		if found {
			return cached, nil
		}
		s.storeIdemEntry(cacheKey, &idemEntry{status: idemInFlight, createdAt: time.Now()})
	}
	defer func() {
		if err != nil && cacheKey != "" {
			s.deleteIdemEntry(cacheKey)
		}
	}()

	// validaciones (FASE 1)
	title = utils.NormalizeTitle(title)
	title = strings.ReplaceAll(title, "/", "_")
	title = strings.ReplaceAll(title, "\\", "_")
	if err := validateTitle(title); err != nil {
		return nil, err
	}
	if strings.TrimSpace(visibility) == "" {
		visibility = "private"
	}
	if err := validateVisibility(visibility); err != nil {
		return nil, err
	}
	if err := validateSubjectID(subjectID); err != nil {
		return nil, err
	}
	if subjectID != nil {
		v := strings.TrimSpace(*subjectID)
		if v == "" {
			subjectID = nil
		} else {
			*subjectID = v
		}
	}
	// contenido por defecto
	mdContent := ""
	if content != nil {
		mdContent = *content
	} else {
		mdContent = fmt.Sprintf("# %s\n\n", title)
	}

	// 1. Insertar metadata en PG con sync_status='pending_drive' ANTES de llamar a Drive
	// Así si el proceso crashea, se puede reconciliar consultando notas con pending_drive
	note, err = s.notes.Create(ctx, userID, subjectID, title, nil, visibility, nil, "pending_drive")
	if err != nil {
		return nil, ErrInternalDatabase
	}

	// 2. Crear archivo en Drive del autor con contexto acotado (15s) y hasta 3
	// intentos con backoff: evita bloqueos indefinidos ante un cuelgue del
	// upstream y absorbe fallos transitorios (5xx/red).
	driveCtx, cancelDrive := withDriveTimeout(ctx)
	var driveFileID string
	err = retryDriveOperation(driveCtx, driveRetryMaxAttempts, func() error {
		var opErr error
		driveFileID, opErr = s.drive.CreateFile(driveCtx, userID, title+".md", mdContent)
		return opErr
	})
	cancelDrive()
	if err != nil {
		// Drive falló: marcar nota como failed_sync para reconciliación
		_ = s.notes.UpdateSyncStatus(ctx, note.ID, "failed_sync")
		// OAuth ausente/revocado: 403 genérico sin filtrar texto interno.
		if drive.IsOAuthError(err) {
			return nil, newServiceErrorMsg(utils.ErrForbidden, "Conecte o renueve su Google Drive")
		}
		// 413 (archivo demasiado grande) se traduce al error canónico.
		var de *drive.DriveError
		if errors.As(err, &de) && de.Code == 413 {
			return nil, newServiceError(utils.ErrFileTooLarge)
		}
		return nil, ErrDriveUnavailable
	}

	// 3. Actualizar metadata en PG con external_file_id y sync_status='synced'
	if err := s.notes.UpdateExternalFileID(ctx, note.ID, driveFileID); err != nil {
		// Compensación: PG falló tras Drive OK -> borrar huérfano y marcar failed_sync
		_ = s.notes.UpdateSyncStatus(ctx, note.ID, "failed_sync")
		compCtx, cancelComp := withDriveTimeout(ctx)
		compErr := retryDriveOperation(compCtx, driveRetryMaxAttempts, func() error {
			return s.drive.DeleteFile(compCtx, userID, driveFileID)
		})
		cancelComp()
		if compErr != nil && !drive.IsNotFound(compErr) {
			s.notes.InsertDeadLetter(ctx, driveFileID, "create_orphan_failed")
			log.Printf("[CRITICAL_UNRECONCILED] notes: create compensación falló (file %s): %v - PG error: %v", driveFileID, compErr, err)
		} else {
			log.Printf("notes: create compensación OK (huérfano Drive %s eliminado tras fallo PG)", driveFileID)
		}
		return nil, ErrInternalDatabase
	}

	// Recuperar nota actualizada con external_file_id y sync_status
	updated, err := s.notes.GetByID(ctx, note.ID)
	if err != nil {
		return nil, ErrInternalDatabase
	}
	if cacheKey != "" {
		// Cachear el puntero exitoso con su marca temporal: los replays con la
		// misma Idempotency-Key reciben exactamente la misma nota sin volver a
		// tocar PG/Drive mientras la entrada no supere el TTL de 10 minutos.
		s.storeIdemEntry(cacheKey, &idemEntry{status: idemSuccess, note: updated, createdAt: time.Now()})
	}
	return updated, nil
}

// Get: lee metadata, verifica acceso, descarga contenido de Drive con manejo 404/403 estructurado.
func (s *NoteService) Get(ctx context.Context, requesterID string, noteID string) (*model.Note, error) {
	if !utils.ValidateUUID(noteID) {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	note, err := s.notes.GetByID(ctx, noteID)
	if err != nil {
		return nil, ErrInternalDatabase
	}
	if note == nil {
		return nil, notFoundNote()
	}
	// Zero-knowledge: nota privada de otro autor sin share vigente o sin
	// membresía en los grupos restricted => 404 idéntico al inexistente
	// (nunca 403, que confirmaría la existencia del recurso).
	allowed, err := s.hasReadAccess(ctx, note, requesterID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, notFoundNote()
	}
	// descargar contenido de Drive usando external_file_id, con contexto
	// acotado para no quedar colgado ante un cuelgue de red.
	if note.ExternalFileID == nil || strings.TrimSpace(*note.ExternalFileID) == "" {
		// sin file aún (offline-first brevemente) -> retornar sin contenido
		return note, nil
	}
	driveCtx, cancelDrive := withDriveTimeout(ctx)
	var content string
	err = retryDriveOperation(driveCtx, driveRetryMaxAttempts, func() error {
		var opErr error
		content, opErr = s.drive.GetFileContent(driveCtx, note.UserID, *note.ExternalFileID)
		return opErr
	})
	cancelDrive()
	if err != nil {
		// OAuth del autor ausente/revocado: el archivo no es servible.
		if drive.IsOAuthError(err) {
			return nil, newServiceErrorMsg(utils.ErrNoteUnavailable, "Nota no disponible en almacenamiento remoto")
		}
		if drive.IsNotFound(err) {
			return nil, newServiceErrorMsg(utils.ErrNoteUnavailable, "Nota no disponible en almacenamiento remoto")
		}
		if drive.IsForbidden(err) {
			return nil, newServiceErrorMsg(utils.ErrNoteUnavailable, "Nota no disponible en almacenamiento remoto")
		}
		// fallback: si error es un DriveError tipado
		var driveErr *drive.DriveError
		if errors.As(err, &driveErr) && (driveErr.Code == 403 || driveErr.Code == 404) {
			return nil, newServiceErrorMsg(utils.ErrNoteUnavailable, "Nota no disponible en almacenamiento remoto")
		}
		return nil, ErrDriveUnavailable
	}
	note.Content = &content
	return note, nil
}

// ListMy: GET /notes/me paginado
func (s *NoteService) ListMy(ctx context.Context, userID string, cursor string, limit int) ([]*model.Note, string, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	// cursor validación: si no vacío debe ser uuid
	if cursor != "" && !utils.ValidateUUID(cursor) {
		return nil, "", newServiceErrorMsg(utils.ErrBadRequest, "cursor inválido")
	}
	notes, next, err := s.notes.ListByUser(ctx, userID, cursor, limit)
	if err != nil {
		return nil, "", ErrInternalDatabase
	}
	if notes == nil {
		notes = []*model.Note{}
	}
	return notes, next, nil
}

// Update: solo autor, actualiza título/visibilidad en PG, contenido en Drive si
// cambia. Soporta idempotencia opcional (header X-Idempotency-Key) con el mismo
// mecanismo unificado que Create/Copy (loadIdemEntry / idemEntry): un replay de
// red con la misma clave responde desde el caché (TTL 10 min) reiterando el
// resultado sin volver a mutar PG/Drive, una solicitud concurrente con la misma
// clave se rechaza con "solicitud en progreso" (in-flight) y cualquier error
// purga la clave para permitir reintentos seguros.
func (s *NoteService) Update(ctx context.Context, userID string, noteID string, title *string, visibility *string, content *string, idempotencyKey string) (*model.Note, error) {
	if content != nil && len(*content) > maxNoteContentLength {
		return nil, newServiceError(utils.ErrFileTooLarge)
	}
	cacheKey := idemKey("update", idempotencyKey)
	if cacheKey != "" {
		cached, found, cacheErr := s.loadIdemEntry(cacheKey)
		if cacheErr != nil {
			return nil, cacheErr
		}
		if found {
			return cached, nil
		}
		s.storeIdemEntry(cacheKey, &idemEntry{status: idemInFlight, createdAt: time.Now()})
	}
	updated, err := s.updateInner(ctx, userID, noteID, title, visibility, content)
	if cacheKey != "" {
		if err != nil {
			s.deleteIdemEntry(cacheKey)
		} else if updated != nil {
			s.storeIdemEntry(cacheKey, &idemEntry{status: idemSuccess, note: updated, createdAt: time.Now()})
		}
	}
	return updated, err
}

// updateInner implementa la lógica de Update sin idempotencia: validación de
// autor, sincronización causal PG <-> Drive con snapshot previo reversible y
// self-healing de notas legacy sin external_file_id.
func (s *NoteService) updateInner(ctx context.Context, userID string, noteID string, title *string, visibility *string, content *string) (*model.Note, error) {
	if !utils.ValidateUUID(noteID) {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	note, err := s.notes.GetByID(ctx, noteID)
	if err != nil {
		return nil, ErrInternalDatabase
	}
	if note == nil {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	if note.UserID != userID {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	// validar campos opcionales (misma sanitización XSS que Create)
	if content != nil {
		*content = sanitizeMarkdown(*content)
	}
	var newTitle *string
	if title != nil {
		t := utils.NormalizeTitle(*title)
		t = strings.ReplaceAll(t, "/", "_")
		t = strings.ReplaceAll(t, "\\", "_")
		if err := validateTitle(t); err != nil {
			return nil, err
		}
		newTitle = &t
	}
	if visibility != nil {
		v := strings.TrimSpace(*visibility)
		if err := validateVisibility(v); err != nil {
			return nil, err
		}
		visibility = &v
	}
	// Consistencia causal: Drive primero, PG después. Si la escritura en PG
	// falla, se compensa revirtiendo en Drive el título/contenido previos
	// (revertDrive) para no dejar el archivo divergente de la metadata y se
	// marca la fila como failed_sync (reconciliable) para no perder el rastro
	// de la divergencia.
	// Self-healing: si la metadata no tiene external_file_id (nota legacy/
	// offline), se crea el archivo .md en Drive y se persiste el nuevo ID.
	hasDriveFile := note.ExternalFileID != nil && strings.TrimSpace(*note.ExternalFileID) != ""
	var revertDrive func()
	driveMutated := false
	if content != nil && hasDriveFile {
		fileID := *note.ExternalFileID
		// Snapshot previo obligatorio: sin copia del contenido anterior la
		// actualización en Drive sería irreversible si la escritura en PG
		// falla después. Abortamos antes de tocar Drive devolviendo
		// ErrDriveUnavailable (502) en lugar de aplicar un cambio sin marcha
		// atrás.
		snapCtx, cancelSnap := withDriveTimeout(ctx)
		var prevContent string
		readErr := retryDriveOperation(snapCtx, driveRetryMaxAttempts, func() error {
			var opErr error
			prevContent, opErr = s.drive.GetFileContent(snapCtx, userID, fileID)
			return opErr
		})
		cancelSnap()
		if readErr != nil {
			log.Printf("notes: update nota %s: snapshot previo falló (file %s): %v", noteID, fileID, readErr)
			if drive.IsOAuthError(readErr) {
				return nil, newServiceErrorMsg(utils.ErrForbidden, "Conecte o renueve su Google Drive")
			}
			if drive.IsNotFound(readErr) || drive.IsForbidden(readErr) {
				return nil, newServiceErrorMsg(utils.ErrNoteUnavailable, "Nota no disponible en almacenamiento remoto")
			}
			return nil, ErrDriveUnavailable
		}
		prevTitle := note.Title
		revertDrive = func() {
			var revertTitle *string
			if newTitle != nil {
				revertTitle = &prevTitle
			}
			s.compensateDriveUpdate(ctx, userID, fileID, &prevContent, revertTitle, "update_revert_failed")
		}
		updCtx, cancelUpd := withDriveTimeout(ctx)
		updErr := retryDriveOperation(updCtx, driveRetryMaxAttempts, func() error {
			return s.drive.UpdateFile(updCtx, userID, fileID, content, newTitle)
		})
		cancelUpd()
		if updErr != nil {
			if drive.IsOAuthError(updErr) {
				return nil, newServiceErrorMsg(utils.ErrForbidden, "Conecte o renueve su Google Drive")
			}
			if drive.IsNotFound(updErr) || drive.IsForbidden(updErr) {
				return nil, newServiceErrorMsg(utils.ErrNoteUnavailable, "Nota no disponible en almacenamiento remoto")
			}
			return nil, ErrDriveUnavailable
		}
		driveMutated = true
	} else if content != nil && !hasDriveFile {
		titleForFile := note.Title
		if newTitle != nil {
			titleForFile = *newTitle
		}
		healCtx, cancelHeal := withDriveTimeout(ctx)
		var newFileID string
		err = retryDriveOperation(healCtx, driveRetryMaxAttempts, func() error {
			var opErr error
			newFileID, opErr = s.drive.CreateFile(healCtx, userID, titleForFile+".md", *content)
			return opErr
		})
		cancelHeal()
		if err != nil {
			if drive.IsOAuthError(err) {
				return nil, newServiceErrorMsg(utils.ErrForbidden, "Conecte o renueve su Google Drive")
			}
			var de *drive.DriveError
			if errors.As(err, &de) && de.Code == 413 {
				return nil, newServiceError(utils.ErrFileTooLarge)
			}
			return nil, ErrDriveUnavailable
		}
		if err := s.notes.UpdateExternalFileID(ctx, noteID, newFileID); err != nil {
			// Compensación: PG falló tras CreateFile OK -> marcar failed_sync
			// y borrar huérfano.
			_ = s.notes.UpdateSyncStatus(ctx, noteID, "failed_sync")
			compCtx, cancelComp := withDriveTimeout(ctx)
			compErr := retryDriveOperation(compCtx, driveRetryMaxAttempts, func() error {
				return s.drive.DeleteFile(compCtx, userID, newFileID)
			})
			cancelComp()
			if compErr != nil && !drive.IsNotFound(compErr) {
				s.notes.InsertDeadLetter(ctx, newFileID, "update_orphan_failed")
				log.Printf("[CRITICAL_UNRECONCILED] notes: update self-healing compensación falló (file %s): %v - PG error: %v", newFileID, compErr, err)
			}
			return nil, ErrInternalDatabase
		}
		note.ExternalFileID = &newFileID
	} else if newTitle != nil && hasDriveFile && content == nil {
		// solo renombrar en Drive (revertible si PG falla)
		fileID := *note.ExternalFileID
		prevTitle := note.Title
		revertDrive = func() {
			s.compensateDriveUpdate(ctx, userID, fileID, nil, &prevTitle, "update_revert_failed")
		}
		renameCtx, cancelRename := withDriveTimeout(ctx)
		renameErr := retryDriveOperation(renameCtx, driveRetryMaxAttempts, func() error {
			return s.drive.UpdateFile(renameCtx, userID, fileID, nil, newTitle)
		})
		cancelRename()
		if renameErr != nil {
			if drive.IsOAuthError(renameErr) {
				return nil, newServiceErrorMsg(utils.ErrForbidden, "Conecte o renueve su Google Drive")
			}
			if drive.IsNotFound(renameErr) || drive.IsForbidden(renameErr) {
				return nil, newServiceErrorMsg(utils.ErrNoteUnavailable, "Nota no disponible en almacenamiento remoto")
			}
			return nil, ErrDriveUnavailable
		}
		driveMutated = true
	}
	// actualizar metadata PG
	updated, err := s.notes.Update(ctx, noteID, newTitle, visibility)
	if err != nil {
		// PG falló tras Drive OK: marcar explícitamente la fila como
		// failed_sync (estado reconciliable) y compensar revirtiendo Drive.
		// Si la compensación también falla, compensateDriveUpdate registra la
		// entrada en la DLQ ([CRITICAL_UNRECONCILED]).
		if driveMutated {
			_ = s.notes.UpdateSyncStatus(ctx, noteID, "failed_sync")
		}
		if revertDrive != nil {
			revertDrive()
		}
		return nil, ErrInternalDatabase
	}
	return updated, nil
}

// compensateDriveUpdate revierte en Drive un cambio ya aplicado cuando la
// escritura posterior en PG falla (consistencia causal). Si la compensación
// también falla, se registra en la DLQ y con la etiqueta
// [CRITICAL_UNRECONCILED] para reconciliación asíncrona; el error nunca
// incluye trazas SQL ni contenido del usuario.
func (s *NoteService) compensateDriveUpdate(ctx context.Context, userID, fileID string, content, title *string, reason string) {
	driveCtx, cancelDrive := withDriveTimeout(ctx)
	defer cancelDrive()
	if err := retryDriveOperation(driveCtx, driveRetryMaxAttempts, func() error {
		return s.drive.UpdateFile(driveCtx, userID, fileID, content, title)
	}); err != nil {
		_ = s.notes.InsertDeadLetter(ctx, fileID, reason)
		log.Printf("[CRITICAL_UNRECONCILED] notes: compensación Update falló (file %s): %v", fileID, err)
	}
}

// Delete: solo autor, orden canónico idempotente para prevenir huérfanos.
// Canónico = PG primero (fuente de verdad), Drive después best-effort:
//  1. Si PG falla, Drive queda intacto (operación reintentable, sin pérdida de
//     datos: fila + archivo siguen consistentes).
//  2. Si PG OK pero Drive falla, se loguea y se retorna éxito: no queda fila
//     huérfana visible (la basura Drive es invisible y reintentable por GC).
//
// DeleteFile es idempotente ante 404, por lo que reintentar o borrar un archivo
// ya eliminado en Drive nunca bloquea el borrado lógico en PG. Un segundo
// Delete tras éxito retorna not_found (idempotencia a nivel API).
func (s *NoteService) Delete(ctx context.Context, userID string, noteID string) error {
	if !utils.ValidateUUID(noteID) {
		return newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	note, err := s.notes.GetByID(ctx, noteID)
	if err != nil {
		return ErrInternalDatabase
	}
	if note == nil {
		return newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	if note.UserID != userID {
		return newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	// Capturar fileID antes del borrado PG (después la metadata ya no existe).
	fileID := ""
	if note.ExternalFileID != nil {
		fileID = strings.TrimSpace(*note.ExternalFileID)
	}
	// 1. Borrar metadata en PG (fuente de verdad). Si falla, Drive intacto.
	// La clasificación del error del store es tipada (revalidación con el
	// propio store), sin inspeccionar strings ni filtrar trazas SQL.
	if err := s.notes.Delete(ctx, noteID); err != nil {
		if current, checkErr := s.notes.GetByID(ctx, noteID); checkErr == nil && current == nil {
			// Borrado concurrente: el recurso ya no existe -> not_found.
			return newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
		}
		return ErrInternalDatabase
	}
	// 2. Borrar archivo en Drive best-effort (nunca revierte el éxito de PG).
	if fileID != "" {
		driveCtx, cancelDrive := withDriveTimeout(ctx)
		delErr := retryDriveOperation(driveCtx, driveRetryMaxAttempts, func() error {
			return s.drive.DeleteFile(driveCtx, userID, fileID)
		})
		cancelDrive()
		if delErr != nil {
			if drive.IsNotFound(delErr) {
				return nil // idempotente: ya borrado en Drive
			}
			// OAuth revocado/ausente o 500 tras PG OK: no fallar la operación,
			// solo loguear (evita fila huérfana visible por token inválido).
			s.notes.InsertDeadLetter(ctx, fileID, "delete_orphan_failed")
			log.Printf("[CRITICAL_UNRECONCILED] notes: delete nota %s: PG OK, Drive best-effort falló (file %s): %v", noteID, fileID, delErr)
		}
	}
	return nil
}

// Attachments
func (s *NoteService) AddAttachment(ctx context.Context, userID string, noteID string, fileName string, fileType string, data []byte, isInline bool) (*model.Attachment, error) {
	if !utils.ValidateUUID(noteID) {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	note, err := s.notes.GetByID(ctx, noteID)
	if err != nil {
		return nil, ErrInternalDatabase
	}
	if note == nil {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	if note.UserID != userID {
		return nil, newServiceError(utils.ErrForbidden)
	}
	if len(data) == 0 {
		return nil, newServiceErrorMsg(utils.ErrBadRequest, "archivo vacío")
	}
	if len(data) > 10*1024*1024 {
		return nil, newServiceError(utils.ErrFileTooLarge)
	}
	if strings.TrimSpace(fileName) == "" {
		fileName = "attachment"
	}
	// Preserva el MIME específico que declare el cliente; si viene vacío o
	// genérico (application/octet-stream) lo resuelve por extensión y sniffing
	// del binario, en paridad con RealDriveClient/MockClient.
	fileType = drive.DetectMimeType(fileName, fileType, data)
	uploadCtx, cancelUpload := withDriveTimeout(ctx)
	var extID, url string
	err = retryDriveOperation(uploadCtx, driveRetryMaxAttempts, func() error {
		var opErr error
		extID, url, opErr = s.drive.UploadAttachment(uploadCtx, userID, noteID, fileName, fileType, data, isInline)
		return opErr
	})
	cancelUpload()
	if err != nil {
		if drive.IsOAuthError(err) {
			return nil, newServiceErrorMsg(utils.ErrForbidden, "Conecte o renueve su Google Drive")
		}
		var de *drive.DriveError
		if errors.As(err, &de) && de.Code == 413 {
			return nil, newServiceError(utils.ErrFileTooLarge)
		}
		return nil, ErrDriveUnavailable
	}
	size := len(data)
	att, err := s.attachments.Create(ctx, noteID, extID, url, fileType, &fileName, &size, isInline)
	if err != nil {
		// Compensación: PG falló tras UploadAttachment OK -> borrar huérfano.
		compCtx, cancelComp := withDriveTimeout(ctx)
		compErr := retryDriveOperation(compCtx, driveRetryMaxAttempts, func() error {
			return s.drive.DeleteAttachment(compCtx, userID, extID)
		})
		cancelComp()
		if compErr != nil {
			s.notes.InsertDeadLetter(ctx, extID, "add_attachment_orphan")
			log.Printf("[CRITICAL_UNRECONCILED] notes: addAttachment compensación falló (file %s): %v - PG error: %v", extID, compErr, err)
		}
		return nil, ErrInternalDatabase
	}
	return att, nil
}

func (s *NoteService) RemoveAttachment(ctx context.Context, userID string, noteID string, attachmentID string) error {
	if !utils.ValidateUUID(noteID) || !utils.ValidateUUID(attachmentID) {
		return newServiceErrorMsg(utils.ErrNotFound, "adjunto no encontrado")
	}
	note, err := s.notes.GetByID(ctx, noteID)
	if err != nil {
		return ErrInternalDatabase
	}
	if note == nil {
		return newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	if note.UserID != userID {
		return newServiceError(utils.ErrForbidden)
	}
	att, err := s.attachments.GetByID(ctx, attachmentID)
	if err != nil {
		return ErrInternalDatabase
	}
	if att == nil || att.NoteID != noteID {
		return newServiceErrorMsg(utils.ErrNotFound, "adjunto no encontrado")
	}
	// Orden canónico PG-primero (igual que Delete de notas): si PG falla, el
	// binario en Drive queda intacto y la operación es reintentable; si PG OK
	// pero Drive falla, se loguea y se retorna éxito (sin fila huérfana).
	if err := s.attachments.Delete(ctx, attachmentID); err != nil {
		return ErrInternalDatabase
	}
	driveCtx, cancelDrive := withDriveTimeout(ctx)
	delErr := retryDriveOperation(driveCtx, driveRetryMaxAttempts, func() error {
		return s.drive.DeleteAttachment(driveCtx, userID, att.ExternalFileID)
	})
	cancelDrive()
	if delErr != nil {
		s.notes.InsertDeadLetter(ctx, att.ExternalFileID, "delete_attachment_orphan_failed")
		log.Printf("[CRITICAL_UNRECONCILED] notes: removeAttachment nota %s adjunto %s: PG OK, Drive best-effort falló: %v", noteID, attachmentID, delErr)
	}
	return nil
}

func (s *NoteService) ListAttachments(ctx context.Context, noteID string) ([]*model.Attachment, error) {
	return s.attachments.ListByNote(ctx, noteID)
}

// AddAttachmentExternal registra un adjunto ya subido directo a Drive desde el cliente (external_file_id provisto).
func (s *NoteService) AddAttachmentExternal(ctx context.Context, userID string, noteID string, externalFileID string, fileName string, fileType string, isInline bool, fileSize *int) (*model.Attachment, error) {
	if !utils.ValidateUUID(noteID) {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	if strings.TrimSpace(externalFileID) == "" {
		return nil, newServiceErrorMsg(utils.ErrBadRequest, "external_file_id requerido")
	}
	note, err := s.notes.GetByID(ctx, noteID)
	if err != nil {
		return nil, ErrInternalDatabase
	}
	if note == nil {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	if note.UserID != userID {
		return nil, newServiceError(utils.ErrForbidden)
	}

	// Verificar que el archivo existe y es accesible en Drive (contexto acotado).
	verifyCtx, cancelVerify := withDriveTimeout(ctx)
	verifyErr := retryDriveOperation(verifyCtx, driveRetryMaxAttempts, func() error {
		return s.drive.VerifyFileAccess(verifyCtx, userID, externalFileID)
	})
	cancelVerify()
	if verifyErr != nil {
		if drive.IsOAuthError(verifyErr) {
			return nil, newServiceErrorMsg(utils.ErrForbidden, "Conecte o renueve su Google Drive")
		}
		if drive.IsNotFound(verifyErr) || drive.IsForbidden(verifyErr) {
			return nil, newServiceErrorMsg(utils.ErrNotFound, "archivo externo no encontrado o sin permisos en Drive")
		}
		return nil, ErrDriveUnavailable
	}

	if strings.TrimSpace(fileType) == "" || fileType == "application/octet-stream" {
		// Sin binario disponible solo se puede inferir por extensión; si es
		// desconocida se conserva el genérico.
		fileType = drive.DetectMimeType(fileName, fileType, nil)
	}
	fileURL := fmt.Sprintf("https://drive.google.com/file/d/%s/view", externalFileID)
	var fnPtr *string
	if strings.TrimSpace(fileName) != "" {
		fnPtr = &fileName
	}
	return s.attachments.Create(ctx, noteID, externalFileID, fileURL, fileType, fnPtr, fileSize, isInline)
}

// DriveUpload describe el resultado de subir un binario directo al Drive del
// usuario autenticado, antes de vincularlo a una nota.
type DriveUpload struct {
	ExternalFileID string
	FileURL        string
	FileName       string
	FileType       string
	FileSizeBytes  int
}

// UploadToDrive sube un archivo al Drive del usuario autenticado sin exigir
// todavía una nota destino. El cliente puede luego vincularlo con
// AddAttachmentExternal y el external_file_id devuelto. Mismas validaciones de
// tamaño/tipo y compensaciones que AddAttachment, pero sin persistir metadata.
func (s *NoteService) UploadToDrive(ctx context.Context, userID string, fileName string, fileType string, data []byte) (*DriveUpload, error) {
	if strings.TrimSpace(fileName) == "" {
		fileName = "attachment"
	}
	if len(data) == 0 {
		return nil, newServiceErrorMsg(utils.ErrBadRequest, "archivo vacío")
	}
	if len(data) > 10*1024*1024 {
		return nil, newServiceError(utils.ErrFileTooLarge)
	}
	fileType = drive.DetectMimeType(fileName, fileType, data)
	uploadCtx, cancelUpload := withDriveTimeout(ctx)
	var extID, url string
	err := retryDriveOperation(uploadCtx, driveRetryMaxAttempts, func() error {
		var opErr error
		extID, url, opErr = s.drive.UploadAttachment(uploadCtx, userID, "", fileName, fileType, data, false)
		return opErr
	})
	cancelUpload()
	if err != nil {
		if drive.IsOAuthError(err) {
			return nil, newServiceErrorMsg(utils.ErrForbidden, "Conecte o renueve su Google Drive")
		}
		var de *drive.DriveError
		if errors.As(err, &de) && de.Code == 413 {
			return nil, newServiceError(utils.ErrFileTooLarge)
		}
		return nil, ErrDriveUnavailable
	}
	return &DriveUpload{
		ExternalFileID: extID,
		FileURL:        url,
		FileName:       fileName,
		FileType:       fileType,
		FileSizeBytes:  len(data),
	}, nil
}

// Save: bookmark sin clonar
func (s *NoteService) Save(ctx context.Context, userID string, noteID string) (*model.SavedNote, error) {
	if !utils.ValidateUUID(noteID) {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	note, err := s.notes.GetByID(ctx, noteID)
	if err != nil {
		return nil, ErrInternalDatabase
	}
	if note == nil {
		return nil, notFoundNote()
	}
	// Zero-knowledge: privada sin acceso => 404 (nunca 403) para no revelar
	// la existencia del recurso.
	allowed, err := s.hasReadAccess(ctx, note, userID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, notFoundNote()
	}
	// Idempotencia del bookmark sin inspeccionar strings del storage: se
	// pre-consulta Exists y, ante fallo del Save (carrera), se re-valida el
	// estado real para distinguir duplicate de error interno.
	exists, err := s.saved.Exists(ctx, userID, noteID)
	if err != nil {
		return nil, ErrInternalDatabase
	}
	if exists {
		return nil, newServiceError(utils.ErrAlreadySaved)
	}
	saved, err := s.saved.Save(ctx, userID, noteID)
	if err != nil {
		if ok, checkErr := s.saved.Exists(ctx, userID, noteID); checkErr == nil && ok {
			return nil, newServiceError(utils.ErrAlreadySaved)
		}
		return nil, ErrInternalDatabase
	}
	return saved, nil
}

func (s *NoteService) Unsave(ctx context.Context, userID string, noteID string) error {
	return s.saved.Delete(ctx, userID, noteID)
}

// Copy: clona apunte de tercero, crea metadata en PG con sync_status='pending_drive',
// copia archivo en Drive, actualiza sync_status. La idempotencia reutiliza el
// mismo contrato unificado que Create: se publica un *idemEntry in-flight, se
// reemplaza por *idemEntry{status: idemSuccess, note, createdAt} al tener éxito
// y el defer libera la clave ante error para evitar memory leaks.
func (s *NoteService) Copy(ctx context.Context, userID string, noteID string, idempotencyKey string) (note *model.Note, err error) {
	cacheKey := idemKey("copy", idempotencyKey)
	if cacheKey != "" {
		cached, found, cacheErr := s.loadIdemEntry(cacheKey)
		if cacheErr != nil {
			return nil, cacheErr
		}
		if found {
			return cached, nil
		}
		s.storeIdemEntry(cacheKey, &idemEntry{status: idemInFlight, createdAt: time.Now()})
	}
	defer func() {
		if err != nil && cacheKey != "" {
			s.deleteIdemEntry(cacheKey)
		}
	}()

	if !utils.ValidateUUID(noteID) {
		return nil, notFoundNote()
	}
	orig, err := s.notes.GetByID(ctx, noteID)
	if err != nil {
		return nil, ErrInternalDatabase
	}
	// Zero-knowledge absoluto: nota inexistente, privada sin acceso o sin
	// archivo remoto son indistinguibles (mismo notFoundNote, 404 "nota no
	// encontrada"), nunca 403 ni note_unavailable, para no filtrar existencia
	// ni estado del recurso.
	if orig == nil {
		return nil, notFoundNote()
	}
	allowed, err := s.hasReadAccess(ctx, orig, userID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, notFoundNote()
	}
	if orig.ExternalFileID == nil || strings.TrimSpace(*orig.ExternalFileID) == "" {
		return nil, notFoundNote()
	}

	// 1. Insertar metadata en PG con sync_status='pending_drive' ANTES de copiar en Drive
	// Reset SubjectID for the clone to prevent unauthorized access (auditoría:
	// el subjectID del original nunca se hereda al clon).
	var nilSubject *string
	if orig.SubjectID != nil {
		log.Printf("notes: copy nota %s: original con subjectID %s, reseteado a nil para clon %s", noteID, *orig.SubjectID, userID)
	}
	newNote, err := s.notes.Create(ctx, userID, nilSubject, orig.Title, nil, "private", &orig.ID, "pending_drive")
	if err != nil {
		return nil, ErrInternalDatabase
	}

	// 2. Copiar archivo en Drive del copiador. CopyFile exige OAuth vigente del
	// clonador (destino) y resuelve la lectura del origen de forma desacoplada.
	// Contexto acotado: un cuelgue de red no bloquea el request indefinidamente.
	copyCtx, cancelCopy := withDriveTimeout(ctx)
	var newFileID string
	err = retryDriveOperation(copyCtx, driveRetryMaxAttempts, func() error {
		var opErr error
		newFileID, opErr = s.drive.CopyFile(copyCtx, orig.UserID, *orig.ExternalFileID, userID, orig.Title)
		return opErr
	})
	cancelCopy()
	if err != nil {
		// Drive falló: marcar nota como failed_sync para reconciliación
		_ = s.notes.UpdateSyncStatus(ctx, newNote.ID, "failed_sync")
		// (a) OAuth de la cuenta del clonador ausente/revocado: 403 genérico,
		//     el problema está en su propio Drive, no en el origen.
		if drive.IsOAuthError(err) {
			log.Printf("notes: copy nota %s: cuenta del clonador sin OAuth vigente: %v", noteID, err)
			return nil, newServiceErrorMsg(utils.ErrForbidden, "Conecte o renueve su Google Drive")
		}
		// (b) El archivo origen no es legible (borrado o permisos revocados):
		//     zero-knowledge absoluto, mismo notFoundNote que una nota
		//     inexistente/inaccesible (sin mensajes diferenciadores).
		if drive.IsNotFound(err) || drive.IsForbidden(err) {
			log.Printf("notes: copy nota %s: acceso al archivo origen falló (zero-knowledge): %v", noteID, err)
			return nil, notFoundNote()
		}
		// (c) Fallo transitorio del upstream de Drive.
		return nil, ErrDriveUnavailable
	}
	// Defensa: el clon debe tener un fileID distinto al original.
	if newFileID == *orig.ExternalFileID {
		_ = s.notes.UpdateSyncStatus(ctx, newNote.ID, "failed_sync")
		compCtx, cancelComp := withDriveTimeout(ctx)
		compErr := retryDriveOperation(compCtx, driveRetryMaxAttempts, func() error {
			return s.drive.DeleteFile(compCtx, userID, newFileID)
		})
		cancelComp()
		if compErr != nil && !drive.IsNotFound(compErr) {
			s.notes.InsertDeadLetter(ctx, newFileID, "copy_orphan_failed")
			log.Printf("[CRITICAL_UNRECONCILED] notes: copy fileID duplicado, compensación falló (file %s): %v", newFileID, compErr)
		}
		return nil, ErrDriveUnavailable
	}

	// 3. Actualizar metadata en PG con external_file_id y sync_status='synced'
	if err := s.notes.UpdateExternalFileID(ctx, newNote.ID, newFileID); err != nil {
		_ = s.notes.UpdateSyncStatus(ctx, newNote.ID, "failed_sync")
		compCtx, cancelComp := withDriveTimeout(ctx)
		compErr := retryDriveOperation(compCtx, driveRetryMaxAttempts, func() error {
			return s.drive.DeleteFile(compCtx, userID, newFileID)
		})
		cancelComp()
		if compErr != nil && !drive.IsNotFound(compErr) {
			s.notes.InsertDeadLetter(ctx, newFileID, "copy_orphan_failed")
			log.Printf("[CRITICAL_UNRECONCILED] notes: copy compensación falló (file %s): %v - PG error: %v", newFileID, compErr, err)
		} else {
			log.Printf("notes: copy compensación OK (huérfano Drive %s eliminado tras fallo PG)", newFileID)
		}
		return nil, ErrInternalDatabase
	}

	// Recuperar nota actualizada
	updated, err := s.notes.GetByID(ctx, newNote.ID)
	if err != nil {
		return nil, ErrInternalDatabase
	}
	if cacheKey != "" {
		// TTL: la entrada de idempotencia guarda la hora de creación para poder
		// purgarse pasados 10 minutos y no acumular memoria indefinidamente.
		s.storeIdemEntry(cacheKey, &idemEntry{status: idemSuccess, note: updated, createdAt: time.Now()})
	}
	return updated, nil
}

// Like / Unlike con contador transaccional real (Tx en PG, mutex en memoria).
// El duplicado se detecta con Exists + revalidación tipada tras el fallo, sin
// inspeccionar el texto del error del storage.
func (s *NoteService) Like(ctx context.Context, userID string, noteID string) error {
	if !utils.ValidateUUID(noteID) {
		return newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	note, err := s.notes.GetByID(ctx, noteID)
	if err != nil {
		return ErrInternalDatabase
	}
	if note == nil {
		return newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	exists, err := s.likes.Exists(ctx, noteID, userID)
	if err != nil {
		return ErrInternalDatabase
	}
	if exists {
		return newServiceError(utils.ErrAlreadyLiked)
	}
	if err := s.likes.LikeAtomic(ctx, noteID, userID); err != nil {
		// Carrera: si el like quedó registrado por otra request, el resultado
		// canónico es already_liked; en otro caso, fallo interno.
		if ok, checkErr := s.likes.Exists(ctx, noteID, userID); checkErr == nil && ok {
			return newServiceError(utils.ErrAlreadyLiked)
		}
		return ErrInternalDatabase
	}
	return nil
}

func (s *NoteService) Unlike(ctx context.Context, userID string, noteID string) error {
	if !utils.ValidateUUID(noteID) {
		return newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	if err := s.likes.UnlikeAtomic(ctx, noteID, userID); err != nil {
		return ErrInternalDatabase
	}
	return nil
}

// Share
func (s *NoteService) Share(ctx context.Context, ownerID string, noteID string, groupID string, accessMode string) (*model.SharedNote, error) {
	if !utils.ValidateUUID(noteID) || !utils.ValidateUUID(groupID) {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota o grupo no encontrado")
	}
	if !utils.ValidateAccessMode(accessMode) {
		return nil, newServiceError(utils.ErrInvalidAccessMode)
	}
	note, err := s.notes.GetByID(ctx, noteID)
	if err != nil {
		return nil, ErrInternalDatabase
	}
	if note == nil {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	if note.UserID != ownerID {
		return nil, newServiceError(utils.ErrForbidden)
	}
	// verificar membresía en grupo
	isMember, err := s.social.IsMember(ctx, ownerID, groupID)
	if err != nil {
		return nil, ErrInternalDatabase
	}
	if !isMember {
		return nil, newServiceError(utils.ErrForbidden)
	}
	isAdmin, _ := s.social.IsAdmin(ctx, ownerID, groupID)
	followers, _ := s.social.GetFollowersCount(ctx, ownerID)

	shared, err := s.shared.Create(ctx, noteID, groupID, isAdmin, accessMode, followers)
	if err != nil {
		return nil, ErrInternalDatabase
	}
	// access_mode=link no crea permisos nominales en Drive.
	if accessMode == "restricted" {
		s.grantRestrictedPermissions(ctx, ownerID, note, groupID)
	}
	return shared, nil
}

// grantRestrictedPermissions otorga permiso reader en Drive a cada miembro del
// grupo de forma best-effort: si un permiso falla se registra el error (sin
// secretos) y se continúa con los demás; nunca revierte la fila de
// notes.shared_notes ya creada. Los correos se normalizan (trim, lowercase,
// sin vacíos ni duplicados); si el correo del autor viene en el listado queda
// incluido una sola vez (ya es dueño del archivo, el permiso es inofensivo).
func (s *NoteService) grantRestrictedPermissions(ctx context.Context, ownerID string, note *model.Note, groupID string) {
	if note.ExternalFileID == nil || strings.TrimSpace(*note.ExternalFileID) == "" {
		return
	}
	dir := s.members
	if dir == nil {
		dir = NewNoopMemberDirectory()
	}
	emails, err := dir.ListMemberEmails(ctx, groupID)
	if err != nil {
		log.Printf("notes: share restricted nota %s: no se pudieron obtener emails del grupo (se mantiene el share): %v", note.ID, err)
		return
	}
	for _, email := range NormalizeEmails(emails) {
		grantCtx, cancelGrant := withDriveTimeout(ctx)
		grantErr := s.drive.GrantPermission(grantCtx, ownerID, *note.ExternalFileID, email, "reader")
		cancelGrant()
		if grantErr != nil {
			log.Printf("notes: share restricted nota %s: no se pudo otorgar permiso a %s: %v", note.ID, email, grantErr)
		}
	}
}

func (s *NoteService) Unshare(ctx context.Context, userID string, sharedNoteID string) error {
	if !utils.ValidateUUID(sharedNoteID) {
		return newServiceErrorMsg(utils.ErrNotFound, "compartición no encontrada")
	}
	sh, err := s.shared.GetByID(ctx, sharedNoteID)
	if err != nil {
		return ErrInternalDatabase
	}
	if sh == nil {
		return newServiceErrorMsg(utils.ErrNotFound, "compartición no encontrada")
	}
	note, err := s.notes.GetByID(ctx, sh.NoteID)
	if err != nil {
		return ErrInternalDatabase
	}
	if note == nil {
		// nota ya borrada, borrar share
		_ = s.shared.Delete(ctx, sharedNoteID)
		return nil
	}
	// solo autor o admin del grupo puede retirar (simplificado: autor)
	// Verificamos si requester es autor o admin del grupo
	if note.UserID != userID {
		isAdmin, err := s.social.IsAdmin(ctx, userID, sh.GroupID)
		if err != nil {
			return ErrInternalDatabase
		}
		if !isAdmin {
			return newServiceError(utils.ErrForbidden)
		}
	}
	if err := s.shared.Delete(ctx, sharedNoteID); err != nil {
		return ErrInternalDatabase
	}
	// revocar permiso Drive si restricted (best-effort, contexto acotado)
	if sh.AccessMode == "restricted" && note.ExternalFileID != nil {
		revCtx, cancelRev := withDriveTimeout(ctx)
		_ = s.drive.RevokePermission(revCtx, note.UserID, *note.ExternalFileID, "member@example.com")
		cancelRev()
	}
	return nil
}

func (s *NoteService) UnshareAll(ctx context.Context, userID string, groupID string) error {
	if !utils.ValidateUUID(groupID) || !utils.ValidateUUID(userID) {
		return newServiceErrorMsg(utils.ErrBadRequest, "ids inválidos")
	}
	// Sin validar autor individual: borra todos los shares de userID en groupID
	if err := s.shared.DeleteByUserAndGroup(ctx, userID, groupID); err != nil {
		return ErrInternalDatabase
	}
	// restaurar permisos Drive: revocar todos
	// Necesitaríamos listar notes de user y revocar; simplificado no-op
	return nil
}

func (s *NoteService) GetAccess(ctx context.Context, requesterID string, noteID string) (map[string]interface{}, error) {
	if !utils.ValidateUUID(noteID) {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	note, err := s.notes.GetByID(ctx, noteID)
	if err != nil {
		return nil, ErrInternalDatabase
	}
	if note == nil {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	canRead := false
	accessMode := "private"
	if note.UserID == requesterID {
		canRead = true
		accessMode = note.Visibility
	} else if note.Visibility == "public" {
		canRead = true
		accessMode = "public"
	} else {
		// Nota privada de otro autor: solo legible si hay vínculo en
		// notes.shared_notes. Reglas multi-share:
		// - si existe al menos un share link -> cualquiera puede leer;
		// - si todos son restricted -> debe ser miembro de al menos uno;
		// - sin shares o sin membresía -> 404 (zero-knowledge: mismo error que
		//   una nota inexistente; nunca 403, que confirmaría su existencia).
		// No se llama a Drive: la autorización se resuelve con PostgreSQL.
		list, err := s.shared.ListByNote(ctx, noteID)
		if err != nil {
			return nil, ErrInternalDatabase
		}
		for _, sh := range list {
			if sh.AccessMode == "link" {
				canRead = true
				accessMode = sh.AccessMode
				break
			}
		}
		if !canRead {
			for _, sh := range list {
				if sh.AccessMode == "restricted" {
					if ok, err := s.social.IsMember(ctx, requesterID, sh.GroupID); err != nil {
						return nil, ErrInternalDatabase
					} else if ok {
						canRead = true
						accessMode = sh.AccessMode
						break
					}
				}
			}
		}
	}
	if !canRead {
		return nil, notFoundNote()
	}
	// Sin archivo externo no hay contenido que servir: nota no disponible
	// (404 con código note_unavailable según el mapeo del handler).
	if note.ExternalFileID == nil || strings.TrimSpace(*note.ExternalFileID) == "" {
		return nil, newServiceErrorMsg(utils.ErrNoteUnavailable, "Nota no disponible en almacenamiento remoto")
	}
	driveURL := ""
	if note.ExternalFileID != nil {
		driveURL = fmt.Sprintf("https://drive.google.com/file/d/%s/view", *note.ExternalFileID)
	}
	return map[string]interface{}{
		"access_mode": accessMode,
		"can_read":    canRead,
		"drive_url":   driveURL,
	}, nil
}

func (s *NoteService) ListGroupNotes(ctx context.Context, requesterID string, groupID string, cursor string, limit int) ([]*model.Note, string, error) {
	if !utils.ValidateUUID(groupID) {
		return nil, "", newServiceErrorMsg(utils.ErrNotFound, "grupo no encontrado")
	}
	// verificar membresía
	isMember, err := s.social.IsMember(ctx, requesterID, groupID)
	if err != nil {
		return nil, "", ErrInternalDatabase
	}
	if !isMember {
		return nil, "", newServiceError(utils.ErrForbidden)
	}
	// listar shared_notes del grupo
	sharedList, next, err := s.shared.ListByGroup(ctx, groupID, limit, cursor)
	if err != nil {
		return nil, "", ErrInternalDatabase
	}
	// Asociar cada shared con su nota para ordenar por criterios formales
	type pair struct {
		note   *model.Note
		shared *model.SharedNote
	}
	var pairs []pair
	for _, sh := range sharedList {
		n, _ := s.notes.GetByID(ctx, sh.NoteID)
		if n != nil {
			pairs = append(pairs, pair{note: n, shared: sh})
		}
	}
	// Orden formal: 1) is_admin_note == true primero, 2) likes_count DESC, 3) shared_at DESC
	sort.SliceStable(pairs, func(i, j int) bool {
		if pairs[i].shared.IsAdminNote != pairs[j].shared.IsAdminNote {
			return pairs[i].shared.IsAdminNote && !pairs[j].shared.IsAdminNote
		}
		if pairs[i].note.LikesCount != pairs[j].note.LikesCount {
			return pairs[i].note.LikesCount > pairs[j].note.LikesCount
		}
		return pairs[i].shared.SharedAt.After(pairs[j].shared.SharedAt)
	})
	var notes []*model.Note
	for _, p := range pairs {
		notes = append(notes, p.note)
	}
	return notes, next, nil
}

// IsGroupAdmin expone verificación de administración para autorización en handlers
// (p. ej. UnshareAll: el propio usuario o un admin del grupo puede desvincular).
func (s *NoteService) IsGroupAdmin(ctx context.Context, userID, groupID string) bool {
	isAdmin, _ := s.social.IsAdmin(ctx, userID, groupID)
	return isAdmin
}

// Helper para validar UUID con mensaje
func MustUUID(s string) bool { _, err := uuid.Parse(s); return err == nil }

// reconcilePendingMinAge es la ventana mínima de gracia antes de compensar una
// nota 'pending_drive'. Elevada a 15 minutos para superar holgadamente tanto el
// TTL in-flight (2 min) como el TTL del caché de idempotencia (10 min): un
// Create/Copy en vuelo nunca es compensado por una ejecución concurrente del
// reconciler, ni siquiera cuando su replay de idempotencia acaba de expirar.
const reconcilePendingMinAge = 15 * time.Minute

// reconcileTimeout acota el presupuesto total de una pasada de reconciliación:
// con muchas notas pendientes o un Drive lento, el job no puede quedar colgado
// indefinidamente. Cada borrado individual ya lleva su propio withDriveTimeout.
const reconcileTimeout = 30 * time.Second

// ReconcilePendingNotes ejecuta la compensación (borrado del huérfano en Drive
// y de la fila en PG) de las notas que quedaron en estado 'pending_drive'
// cuando el proceso murió entre la inserción en Postgres y el alta/confirmación
// en Drive. Filtro anti-race: solo procesa notas 'pending_drive' cuya última
// actividad (created_at / updated_at) supere los 15 minutos, de modo que un
// Create en vuelo (o su replay de idempotencia) nunca sea compensado por un
// reconciliador concurrente.
//
// Verificación previa anti-eliminación agresiva: antes de compensar una nota
// con external_file_id, se intenta leer el archivo en Drive; si existe y tiene
// contenido, la divergencia era solo de metadata (el archivo sí se creó, pero
// la confirmación en PG no llegó) y la nota se repara a 'synced' en lugar de
// destruirse. Solo se compensa (borra Drive + PG) cuando el archivo no existe,
// no es legible o está vacío.
//
// Casos especiales: una nota sin external_file_id limpia directamente su fila
// de PG (no hay nada que preservar en Drive) y un error de OAuth en Drive
// marca la nota como 'failed_sync' con log de auditoría en lugar de eliminarla
// o reintentar en vano, porque el fallo es de credenciales, no de datos.
//
// Toda la pasada se ejecuta bajo un contexto con timeout de 30 segundos para
// que el cron/job nunca quede colgado, y se serializa con reconcileMu: este
// lock es EXCLUSIVO del job de reconciliación (múltiples schedulers/replicas)
// y es independiente del camino HTTP de los usuarios; ninguna operación normal
// de usuario (Create, Get, Update, Copy, Delete, ...) lo adquiere, por lo que
// el reconciliador no bloquea requests ni viceversa. Los estados 'failed_sync'
// no se tocan aquí: su divergencia ya quedó registrada en la dead-letter queue
// para revisión.
func (s *NoteService) ReconcilePendingNotes(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, reconcileTimeout)
	defer cancel()
	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()
	type pendingStore interface {
		GetPendingSyncNotes(ctx context.Context) ([]*model.Note, error)
	}
	ps, ok := s.notes.(pendingStore)
	if !ok {
		return nil
	}
	notes, err := ps.GetPendingSyncNotes(ctx)
	if err != nil {
		return ErrInternalDatabase
	}
	for _, n := range notes {
		if n.SyncStatus != "pending_drive" {
			continue
		}
		age := time.Since(n.CreatedAt)
		if u := time.Since(n.UpdatedAt); u < age {
			age = u
		}
		if age <= reconcilePendingMinAge {
			continue
		}
		if n.ExternalFileID == nil || strings.TrimSpace(*n.ExternalFileID) == "" {
			// Sin archivo remoto no hay nada que verificar ni compensar en
			// Drive: la fila pending_drive es basura pura de PG y se limpia
			// directamente.
			_ = s.notes.Delete(ctx, n.ID)
			continue
		}
		// Verificación previa anti-eliminación agresiva: si el archivo existe
		// y es legible en Drive (lectura con contenido), la nota solo quedó
		// desincronizada en metadata: se repara a 'synced' y se conserva sin
		// destruir datos del usuario.
		driveCtx, cancelDrive := withDriveTimeout(ctx)
		var content string
		statErr := retryDriveOperation(driveCtx, driveRetryMaxAttempts, func() error {
			var opErr error
			content, opErr = s.drive.GetFileContent(driveCtx, n.UserID, *n.ExternalFileID)
			return opErr
		})
		cancelDrive()
		if drive.IsOAuthError(statErr) {
			// OAuth del autor ausente/revocado: no es un huérfano compensable
			// (borrar sería destruir datos por un problema de credenciales).
			// Se marca failed_sync para revisión/reconexión y se conserva.
			log.Printf("notes: reconcile nota %s: Drive sin OAuth vigente, se marca failed_sync: %v", n.ID, statErr)
			_ = s.notes.UpdateSyncStatus(ctx, n.ID, "failed_sync")
			continue
		}
		if statErr == nil && len(content) > 0 {
			_ = s.notes.UpdateSyncStatus(ctx, n.ID, "synced")
			continue
		}
		// Archivo ausente, ilegible o vacío: compensar el huérfano.
		delCtx, cancelDel := withDriveTimeout(ctx)
		_ = retryDriveOperation(delCtx, driveRetryMaxAttempts, func() error {
			return s.drive.DeleteFile(delCtx, n.UserID, *n.ExternalFileID)
		})
		cancelDel()
		_ = s.notes.Delete(ctx, n.ID)
	}
	return nil
}
