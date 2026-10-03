package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/repository"
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
// admitida: una clave desmedida no se registra (evita abuso con headers
// X-Idempotency-Key gigantes). Vacío => sin idempotencia.
const maxIdempotencyKeyLength = 128

// idemKey normaliza la clave de idempotencia: la operación ya namespacea la
// clave en la UNIQUE (user_id, operation, idempotency_key) de
// notes.idempotency_keys. Vacío o demasiado larga => sin idempotencia.
func idemKey(op, key string) string {
	if strings.TrimSpace(key) == "" || len(key) > maxIdempotencyKeyLength {
		return ""
	}
	return strings.TrimSpace(key)
}

// Ventanas de la idempotencia durable:
//   - idemClaimTTL es el lease del claim 'in_progress': si el proceso muere a
//     mitad de la operación, un reintento con la misma clave puede reclamarla
//     pasados 2 minutos (crash recovery) en lugar de quedar bloqueado.
//   - inMemoryIdempotencyReplayTTL es la retención de un registro 'completed'
//     en el store en memoria (espejo del intervalo '24 hours' de PostgreSQL).
const (
	idemClaimTTL                 = 2 * time.Minute
	inMemoryIdempotencyReplayTTL = 24 * time.Hour
)

// idempotencyRequestHash deriva el hash canónico del payload de una operación:
// misma clave + mismo hash => replay legítimo; misma clave + hash distinto =>
// conflicto 409.
func idempotencyRequestHash(operation string, parts ...string) string {
	h := sha256.New()
	h.Write([]byte(operation))
	for _, part := range parts {
		h.Write([]byte{0})
		h.Write([]byte(part))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// derefOrEmpty aplana un *string opcional para el hash de idempotencia.
func derefOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// beginIdempotentOperation reclama la clave durable en BD o resuelve un replay.
//
// Contrato de retorno:
//   - (rec, nil, true, nil): el llamador reclamó la clave; debe ejecutar la
//     operación (rec.ResourceID lleva el recurso preasociado efectivo, que en
//     un claim recuperado tras crash puede ser el noteID del intento anterior)
//     y, al terminar, completeIdempotentOperation (o
//     releaseIdempotentOperation si falla).
//   - (rec, note, false, nil): replay de una operación 'completed'; note es el
//     recurso persistido.
//   - (rec, nil, false, err): conflicto 409 (hash distinto o solicitud en
//     progreso) o error interno de la idempotencia.
//
// matchHash controla el replay: Create/Copy/Update exigen que el hash del
// payload coincida (payload distinto con la misma clave => 409).
//
// preassignedResourceID pre-asocia el recurso antes de crearlo (crash recovery
// lógico): el claim nuevo persiste resource_id = preassignedResourceID y un
// claim vencido conserva el resource_id del intento que murió.
func (s *NoteService) beginIdempotentOperation(ctx context.Context, userID, operation, key, requestHash string, matchHash bool, preassignedResourceID *string) (*model.IdempotencyKey, *model.Note, bool, error) {
	rec, claimed, err := s.notes.GetOrClaimIdempotencyKey(ctx, userID, operation, key, requestHash, preassignedResourceID, time.Now().Add(idemClaimTTL))
	if err != nil {
		return nil, nil, false, ErrInternalDatabase
	}
	// Un registro 'completed' cuyo recurso ya no existe se libera y se reclama
	// de nuevo: la operación se re-ejecuta en lugar de responder un replay
	// fantasma. Acotado a dos intentos para no competir con writers activos.
	// Un registro 'recoverable' (fallo posterior a la inserción local) solo
	// llega aquí si el store NO otorgó el claim: el request_hash difiere, así
	// que la misma clave con otro payload es 409 Conflict sin esperar al lease.
	for attempt := 0; !claimed && rec != nil && attempt < 2; attempt++ {
		if rec.Status == model.IdempotencyStatusRecoverable {
			if matchHash && rec.RequestHash != requestHash {
				return rec, nil, false, newServiceError(utils.ErrConflict)
			}
			// El store otorga el claim inmediato de un recoverable con el mismo
			// hash; si no lo hizo pudo ser una carrera con otro retry idéntico
			// que acaba de marcar recoverable: se reintenta el claim una vez.
			rec, claimed, err = s.notes.GetOrClaimIdempotencyKey(ctx, userID, operation, key, requestHash, preassignedResourceID, time.Now().Add(idemClaimTTL))
			if err != nil {
				return rec, nil, false, ErrInternalDatabase
			}
			if claimed {
				return rec, nil, true, nil
			}
			return rec, nil, false, newServiceErrorMsg(utils.ErrConflict, "solicitud en recuperación")
		}
		if rec.Status != model.IdempotencyStatusCompleted {
			return rec, nil, false, newServiceErrorMsg(utils.ErrConflict, "solicitud en progreso")
		}
		if matchHash && rec.RequestHash != requestHash {
			return rec, nil, false, newServiceError(utils.ErrConflict)
		}
		if rec.ResourceID != nil {
			existing, getErr := s.observedNotesGetByID(ctx, *rec.ResourceID)
			if getErr != nil {
				return rec, nil, false, ErrInternalDatabase
			}
			if existing != nil {
				return rec, existing, false, nil
			}
		}
		if delErr := s.notes.DeleteIdempotencyKey(ctx, userID, operation, key); delErr != nil {
			return rec, nil, false, ErrInternalDatabase
		}
		rec, claimed, err = s.notes.GetOrClaimIdempotencyKey(ctx, userID, operation, key, requestHash, preassignedResourceID, time.Now().Add(idemClaimTTL))
		if err != nil {
			return rec, nil, false, ErrInternalDatabase
		}
	}
	if !claimed {
		return rec, nil, false, ErrInternalDatabase
	}
	return rec, nil, true, nil
}

// releaseIdempotentOperation libera la clave reclamada tras un fallo de la
// operación para permitir reintentos inmediatos con la misma clave. Usa un
// contexto desacoplado de la cancelación del request para no dejar el claim
// colgado hasta que venza el lease.
func (s *NoteService) releaseIdempotentOperation(ctx context.Context, userID, operation, key string) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), driveCallTimeout)
	defer cancel()
	if err := s.notes.DeleteIdempotencyKey(cleanupCtx, userID, operation, key); err != nil {
		log.Printf("notes: no se pudo liberar la clave de idempotencia %s: %v", operation, err)
	}
}

// markIdempotentRecoverable transiciona la clave reclamada a 'recoverable'
// cuando la operación falló DESPUÉS de insertar el recurso local (Drive,
// timeout, OAuthError): la nota ya existe en PG, así que se preserva su id para
// que el reintento inmediato con la misma clave la retome y reintente
// Drive/reconciliación sin duplicar ni responder 409. Usa un contexto
// desacoplado de la cancelación del request para que la marca no se pierda.
func (s *NoteService) markIdempotentRecoverable(ctx context.Context, userID, operation, key, resourceID string, cause error) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), driveCallTimeout)
	defer cancel()
	lastErr := ""
	if cause != nil {
		lastErr = cause.Error()
	}
	log.Printf("notes: operación %s marcada recoverable (recurso %s): %s", operation, resourceID, lastErr)
	if err := s.notes.MarkIdempotencyRecoverable(cleanupCtx, userID, operation, key, resourceID, lastErr); err != nil {
		log.Printf("notes: no se pudo marcar la clave de idempotencia %s como recoverable: %v", operation, err)
	}
}

// completeIdempotentOperation publica el recurso asociado a la clave
// reclamada. Si falla se registra el error: la operación ya tuvo efecto y el
// recurso se recupera por reconciliación, así que no se propaga al usuario.
func (s *NoteService) completeIdempotentOperation(ctx context.Context, userID, operation, key, resourceID string) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), driveCallTimeout)
	defer cancel()
	if err := s.notes.CompleteIdempotencyKey(cleanupCtx, userID, operation, key, resourceID); err != nil {
		log.Printf("notes: no se pudo completar la clave de idempotencia %s: %v", operation, err)
	}
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

// findOrphanDriveFile busca el archivo .md que un intento previo pudo dejar
// huérfano en Drive, indexado con appProperties (notes_note_id). Devuelve ""
// sin error cuando no existe; un error significa que no se pudo probar la
// ausencia (OAuth/red), no que el archivo no exista.
func (s *NoteService) findOrphanDriveFile(ctx context.Context, ownerUserID, noteID string) (string, error) {
	findCtx, cancelFind := withDriveTimeout(ctx)
	defer cancelFind()
	var fileID string
	if err := retryDriveOperation(findCtx, driveRetryMaxAttempts, func() error {
		var opErr error
		fileID, opErr = s.observedDriveFindFileByNoteID(findCtx, ownerUserID, noteID)
		return opErr
	}); err != nil {
		return "", err
	}
	return strings.TrimSpace(fileID), nil
}

func newServiceError(code string) *ServiceError {
	return &ServiceError{Code: code, Message: utils.MessageForCode(code)}
}
func newServiceErrorMsg(code, msg string) *ServiceError {
	return &ServiceError{Code: code, Message: msg}
}

// Store interfaces para desacoplar de pgx y permitir mocks en tests.
type NoteStore interface {
	// Create inserta la nota con el id preasignado noteID (vacío => generado por
	// el store). La pre-asignación permite que un retry tras crash recupere la
	// misma fila en lugar de duplicarla.
	Create(ctx context.Context, noteID string, userID string, subjectID *string, title string, externalFileID *string, visibility string, forkedFrom *string, syncStatus string) (*model.Note, error)
	GetByID(ctx context.Context, id string) (*model.Note, error)
	ListByUser(ctx context.Context, userID, cursor string, limit int) ([]*model.Note, string, error)
	// Update aplica el cambio de metadata con versionado optimista: solo muta
	// si la fila sigue en expectedVersion; si otro writer la cambió devuelve
	// repository.ErrConflict (o un error equivalente del store).
	Update(ctx context.Context, id string, title *string, visibility *string, expectedVersion int64) (*model.Note, error)
	Delete(ctx context.Context, id string) error
	IncrementLikes(ctx context.Context, noteID string, delta int) error
	UpdateExternalFileID(ctx context.Context, noteID, fileID string) error
	UpdateSyncStatus(ctx context.Context, noteID, syncStatus string) error
	DeleteWithDriveCleanup(ctx context.Context, noteID, requesterID string) (string, error)
	EnqueueDriveOperation(ctx context.Context, op, noteID, attID, fileID, ownerUserID string, payload map[string]any) error
	ClaimDriveOperations(ctx context.Context, limit int, lockDuration time.Duration) ([]*model.DriveOperation, error)
	CompleteDriveOperation(ctx context.Context, id string) error
	FailDriveOperation(ctx context.Context, id, errStr string, nextAttempt time.Time) error
	// Idempotencia durable sobre notes.idempotency_keys: GetOrClaim... reclama
	// la clave (o devuelve el registro vigente) pre-asociando preassignedResourceID
	// cuando el claim es nuevo, y Complete... publica el recurso asociado.
	// Delete... libera el claim cuando la operación falla antes de insertar.
	// Mark... marca 'recoverable' cuando el fallo ocurre después de insertar,
	// preservando el recurso para un retry inmediato sin duplicados.
	GetOrClaimIdempotencyKey(ctx context.Context, userID, operation, idempotencyKey, requestHash string, preassignedResourceID *string, expiresAt time.Time) (*model.IdempotencyKey, bool, error)
	CompleteIdempotencyKey(ctx context.Context, userID, operation, idempotencyKey, resourceID string) error
	DeleteIdempotencyKey(ctx context.Context, userID, operation, idempotencyKey string) error
	MarkIdempotencyRecoverable(ctx context.Context, userID, operation, idempotencyKey, resourceID, lastErr string) error
}

type AttachmentStore interface {
	Create(ctx context.Context, noteID, externalFileID, fileURL, fileType string, fileName *string, fileSize *int, isInline bool) (*model.Attachment, error)
	GetByID(ctx context.Context, id string) (*model.Attachment, error)
	ListByNote(ctx context.Context, noteID string) ([]*model.Attachment, error)
	Delete(ctx context.Context, id string) error
	DeleteAttachmentWithDriveCleanup(ctx context.Context, noteID, attachmentID, requesterID string) (string, error)
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
	// Estado deseado de permisos de Drive (notes.drive_managed_permissions) y
	// convergencia: la BD es la fuente de verdad; Drive se sincroniza después.
	UpdatePermissionSyncStatus(ctx context.Context, id, status string) error
	UpdateNotePermissionSyncStatus(ctx context.Context, noteID, status string) error
	ListNotesWithPendingPermissionSync(ctx context.Context, limit int) ([]string, error)
	ListManagedPermissions(ctx context.Context, noteID string) ([]*model.DriveManagedPermission, error)
	UpsertManagedPermission(ctx context.Context, permission *model.DriveManagedPermission) (*model.DriveManagedPermission, error)
	DeleteManagedPermission(ctx context.Context, noteID, principalType, principalKey string) error
	DeleteManagedPermissionsByNote(ctx context.Context, noteID string) error
	MarkManagedPermissionsPending(ctx context.Context, noteID string) error
	// WithNoteLock ejecuta fn sosteniendo un lock distribuido por nota
	// (pg_advisory_xact_lock(hashtextextended(noteID, 0)) en PG; mutex por nota
	// en el store en memoria). Serializa Share/Unshare/Update y el reconciliador
	// antes de mutar notes.drive_managed_permissions y permission_sync_status.
	WithNoteLock(ctx context.Context, noteID string, fn func(context.Context) error) error
}

// SocialResolver verifica pertenencia/membership y seguidores. En producción se
// inyecta el adaptador HTTP a Social (SocialHTTPAdapter); los resolvers en
// memoria/noop solo se usan en tests.
type SocialResolver interface {
	IsMember(ctx context.Context, userID, groupID string) (bool, error)
	IsAdmin(ctx context.Context, userID, groupID string) (bool, error)
	GetFollowersCount(ctx context.Context, userID string) (int, error)
}

// noopSocialResolver es el fallback de construcción cuando no se inyecta un
// resolver: asume membresía (comportamiento de desarrollo). main.go siempre
// inyecta el adaptador HTTP real en producción; los tests usan MemorySocialResolver.
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
	// La idempotencia de Create/Copy/Update vive en notes.idempotency_keys
	// (durable, compartida entre réplicas): no hay caché en memoria que perder
	// ante un crash ni que purgar por TTL. idemClaimTTL actúa como lease de
	// recuperación de claims colgados.
	// reconcileMu serializa EXCLUSIVAMENTE las ejecuciones del cron/job de
	// reconciliación (ReconcilePendingNotes) para que dos instancias en
	// paralelo no colisionen al compensar la misma nota pendiente. Es un lock
	// aislado del camino HTTP normal: ninguna operación de usuario (Create,
	// Get, Update, Copy, Delete...) lo adquiere, por lo que la reconciliación
	// nunca bloquea a los requests de los usuarios ni viceversa.
	// La exclusión de los cambios de ACL (Share/Unshare/Update/reconciliador)
	// ya no depende de un mutex local: usa shared.WithNoteLock, un lock
	// distribuido por nota (pg_advisory_xact_lock en PG) que serializa también
	// entre réplicas.
	reconcileMu sync.Mutex
}

func NewNoteService(notes NoteStore, attachments AttachmentStore, saved SavedStore, likes LikeStore, shared SharedStore, d drive.Client, social SocialResolver) *NoteService {
	if provider, ok := notes.(interface{ memoryNoteStore() *MemoryNoteStore }); ok {
		if binder, ok := attachments.(interface{ SetNoteStore(*MemoryNoteStore) }); ok {
			binder.SetNoteStore(provider.memoryNoteStore())
		}
	}
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
	}
}

// SetMemberDirectory inyecta el directorio de correos de miembros usado por el
// share restricted. En producción main.go inyecta el adaptador HTTP real
// (SocialHTTPAdapter); el default es noop (grupo sin miembros).
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
// Idempotencia durable en notes.idempotency_keys (compartida entre réplicas):
// la clave se reclama antes de tocar PG/Drive; al tener éxito se publica el
// recurso con CompleteIdempotencyKey (replay si coincide request_hash, 409
// Conflict si difiere). Un fallo ANTES de insertar la fila local libera la
// clave con DeleteIdempotencyKey (retry inmediato); un fallo DESPUÉS (Drive,
// timeout, OAuth) la marca 'recoverable' con MarkIdempotencyRecoverable,
// preservando el noteID: el retry con la misma clave retoma la nota existente y
// reintenta Drive/reconciliación sin duplicar ni responder 409 falso. Un claim
// colgado por crash se recupera al vencer su lease (idemClaimTTL).
//
// Pre-asociación del ResourceID (crash recovery lógico): noteID se genera ANTES
// del claim y se persiste como resource_id. Si el proceso muere a mitad de la
// operación, el reintento con la misma clave recupera ese noteID preasociado,
// reutiliza la fila/búsqueda remota existente y continúa la reconciliación en
// lugar de crear una nota duplicada.
func (s *NoteService) Create(ctx context.Context, userID string, title string, subjectID *string, visibility string, content *string, idempotencyKey string) (note *model.Note, err error) {
	// Validación estricta temprana: título (1-255 tras TrimSpace) y subject_id
	// (UUID válido tras TrimSpace) se rechazan ANTES de tocar DB, Drive o la
	// idempotencia durable.
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

	// 0. Reclamar la clave de idempotencia durable ANTES de tocar PG/Drive:
	// misma clave + mismo payload => replay del recurso ya creado; misma clave
	// + payload distinto => 409 Conflict; claim en progreso => 409. El noteID se
	// preasigna antes del claim para que quede durable como resource_id.
	noteID := uuid.NewString()
	recoveredFromCrash := false
	// pgInserted distingue el punto de fallo: antes de materializar la fila en
	// notes.notes el claim se libera (DELETE); después, la nota ya existe y se
	// marca 'recoverable' preservando su id para que el retry no duplique.
	pgInserted := false
	opKey := idemKey("create", idempotencyKey)
	if opKey != "" {
		requestHash := idempotencyRequestHash("create", userID, title, derefOrEmpty(subjectID), visibility, mdContent)
		rec, replay, claimed, idemErr := s.beginIdempotentOperation(ctx, userID, "create", opKey, requestHash, true, &noteID)
		if idemErr != nil {
			return nil, idemErr
		}
		if replay != nil {
			return replay, nil
		}
		if !claimed {
			return nil, ErrInternalDatabase
		}
		if rec != nil && rec.ResourceID != nil && strings.TrimSpace(*rec.ResourceID) != "" && *rec.ResourceID != noteID {
			// Claim recuperado (crash o recoverable): el intento previo dejó su
			// noteID preasociado; el retry lo reutiliza sin duplicar la nota.
			noteID = *rec.ResourceID
			recoveredFromCrash = true
		}
		pgInserted = recoveredFromCrash
		defer func() {
			if err == nil {
				return
			}
			if pgInserted {
				s.markIdempotentRecoverable(ctx, userID, "create", opKey, noteID, err)
			} else {
				s.releaseIdempotentOperation(ctx, userID, "create", opKey)
			}
		}()
	}

	// 1. Insertar metadata en PG con sync_status='pending_drive' ANTES de llamar a Drive
	// Así si el proceso crashea, se puede reconciliar consultando notas con pending_drive.
	// El id preasignado se materializa en la fila; si el intento anterior murió
	// después de insertarla, el retry reutiliza esa misma fila (sin duplicar).
	if recoveredFromCrash {
		note, err = s.observedNotesGetByID(ctx, noteID)
		if err != nil {
			return nil, ErrInternalDatabase
		}
	}
	if note == nil {
		note, err = s.observedNotesCreate(ctx, noteID, userID, subjectID, title, nil, visibility, nil, "pending_drive")
		if err != nil {
			return nil, ErrInternalDatabase
		}
		pgInserted = true
	} else if note.SyncStatus == "synced" && note.ExternalFileID != nil && strings.TrimSpace(*note.ExternalFileID) != "" {
		// El crash ocurrió después de persistir el archivo: la operación ya está
		// completa. Se publica el recurso preasociado y se responde sin duplicar.
		if opKey != "" {
			s.completeIdempotentOperation(ctx, userID, "create", opKey, note.ID)
		}
		return note, nil
	}

	// 2. Crear archivo en Drive del autor con contexto acotado (15s) y hasta 3
	// intentos con backoff: evita bloqueos indefinidos ante un cuelgue del
	// upstream y absorbe fallos transitorios (5xx/red). El archivo se indexa
	// con appProperties (notes_note_id) para poder recuperarlo tras un crash.
	// En un retry de crash se busca primero el huérfano remoto para no duplicarlo.
	driveFileID := ""
	if note.ExternalFileID != nil {
		driveFileID = strings.TrimSpace(*note.ExternalFileID)
	}
	createdDriveFile := false
	if driveFileID == "" && recoveredFromCrash {
		found, findErr := s.findOrphanDriveFile(ctx, userID, note.ID)
		if findErr != nil {
			_ = s.observedNotesUpdateSyncStatus(ctx, note.ID, "failed_sync")
			log.Printf("notes: create retry nota %s: no se pudo verificar el huérfano en Drive: %v", note.ID, findErr)
			return nil, ErrDriveUnavailable
		}
		driveFileID = found
	}
	if driveFileID == "" {
		driveCtx, cancelDrive := withDriveTimeout(ctx)
		err = retryDriveOperation(driveCtx, driveRetryMaxAttempts, func() error {
			var opErr error
			driveFileID, opErr = s.observedDriveCreateFile(driveCtx, userID, note.ID, title+".md", mdContent)
			return opErr
		})
		cancelDrive()
		if err != nil {
			// Drive falló: marcar nota como failed_sync para reconciliación
			_ = s.observedNotesUpdateSyncStatus(ctx, note.ID, "failed_sync")
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
		createdDriveFile = true
	}

	// Public visibility requires an actual reader link before publishing metadata.
	// Solo se otorga para un archivo recién creado en este intento: un archivo
	// adoptado por recovery de crash no se toca para no duplicar permisos.
	if visibility == "public" && createdDriveFile {
		permissionCtx, cancelPermission := withDriveTimeout(ctx)
		linkPermissionID, permissionErr := s.observedDriveGrantLinkPermission(permissionCtx, userID, driveFileID)
		cancelPermission()
		if permissionErr != nil {
			cleanupCtx, cancelCleanup := withDriveTimeout(context.WithoutCancel(ctx))
			defer cancelCleanup()
			_ = s.observedNotesUpdateSyncStatus(cleanupCtx, note.ID, "failed_sync")
			if cleanupErr := s.observedDriveDeleteFile(cleanupCtx, userID, driveFileID); cleanupErr != nil && !drive.IsNotFound(cleanupErr) {
				_ = s.observedNotesEnqueueDriveOperation(cleanupCtx, "delete_file", note.ID, "", driveFileID, userID, map[string]any{"reason": "public_permission_failed"})
			}
			return nil, ErrDriveUnavailable
		}
		// Persistir el permiso link como estado deseado/convergido (BD primero
		// respecto de cualquier cambio futuro de visibilidad). Best-effort: si
		// falla, el reconciliador no pierde la visibilidad pública de la nota.
		var linkPID *string
		if strings.TrimSpace(linkPermissionID) != "" {
			linkPID = &linkPermissionID
		}
		if persistErr := s.persistManagedPermission(ctx, note.ID, driveFileID, model.PermissionPrincipalAnyone, "", linkPID, model.PermissionSyncInSync); persistErr != nil {
			log.Printf("notes: create nota %s: no se pudo persistir el permiso link: %v", note.ID, persistErr)
		}
	}

	// 3. Actualizar metadata en PG con external_file_id y sync_status='synced'
	if err := s.observedNotesUpdateExternalFileID(ctx, note.ID, driveFileID); err != nil {
		// Compensación: PG falló tras Drive OK -> borrar huérfano y marcar failed_sync.
		_ = s.observedNotesUpdateSyncStatus(ctx, note.ID, "failed_sync")
		_ = s.shared.DeleteManagedPermissionsByNote(ctx, note.ID)
		if !createdDriveFile {
			// El archivo adoptado pertenece a un intento previo que sí lo creó:
			// no se borra (evita pérdida de datos); la nota queda reconciliable.
			log.Printf("notes: create retry nota %s: fallo al persistir external_file_id tras adoptar huérfano %s: %v", note.ID, driveFileID, err)
			return nil, ErrInternalDatabase
		}
		compCtx, cancelComp := withDriveTimeout(ctx)
		compErr := retryDriveOperation(compCtx, driveRetryMaxAttempts, func() error {
			return s.observedDriveDeleteFile(compCtx, userID, driveFileID)
		})
		cancelComp()
		if compErr != nil && !drive.IsNotFound(compErr) {
			_ = s.observedNotesEnqueueDriveOperation(ctx, "delete_file", note.ID, "", driveFileID, userID, map[string]any{"reason": "create_orphan_failed"})
			log.Printf("[CRITICAL_UNRECONCILED] notes: create compensación falló (file %s): %v - PG error: %v", driveFileID, compErr, err)
		} else {
			log.Printf("notes: create compensación OK (huérfano Drive %s eliminado tras fallo PG)", driveFileID)
		}
		return nil, ErrInternalDatabase
	}

	// Recuperar nota actualizada con external_file_id y sync_status
	updated, err := s.observedNotesGetByID(ctx, note.ID)
	if err != nil {
		return nil, ErrInternalDatabase
	}
	if opKey != "" {
		// Publicar el recurso en la clave durable: los replays con la misma
		// Idempotency-Key y el mismo payload reciben exactamente esta nota sin
		// volver a tocar PG/Drive.
		s.completeIdempotentOperation(ctx, userID, "create", opKey, updated.ID)
	}
	return updated, nil
}

// Get: lee metadata, verifica acceso, descarga contenido de Drive con manejo 404/403 estructurado.
func (s *NoteService) Get(ctx context.Context, requesterID string, noteID string) (*model.Note, error) {
	if !utils.ValidateUUID(noteID) {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	note, err := s.observedNotesGetByID(ctx, noteID)
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
		return nil, newServiceErrorMsg(utils.ErrNoteUnavailable, "Nota no disponible en almacenamiento remoto")
	}
	driveCtx, cancelDrive := withDriveTimeout(ctx)
	var content string
	err = retryDriveOperation(driveCtx, driveRetryMaxAttempts, func() error {
		var opErr error
		content, opErr = s.observedDriveGetFileContent(driveCtx, note.UserID, *note.ExternalFileID)
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
	notes, next, err := s.observedNotesListByUser(ctx, userID, cursor, limit)
	if err != nil {
		return nil, "", ErrInternalDatabase
	}
	if notes == nil {
		notes = []*model.Note{}
	}
	return notes, next, nil
}

// Update: solo autor, actualiza título/visibilidad en PG, contenido en Drive si
// cambia. Soporta idempotencia opcional (header X-Idempotency-Key) sobre
// notes.idempotency_keys (durable, compartida entre réplicas): un replay de red
// con la misma clave y el mismo body responde el resultado ya aplicado sin
// volver a mutar PG/Drive; la misma clave con un body distinto responde 409
// Conflict, una solicitud concurrente con la misma clave se rechaza con 409
// "solicitud en progreso" y cualquier error libera la clave para permitir
// reintentos seguros.
func (s *NoteService) Update(ctx context.Context, userID string, noteID string, title *string, visibility *string, content *string, idempotencyKey string) (*model.Note, error) {
	return s.UpdateWithExpectedVersion(ctx, userID, noteID, title, visibility, content, 0, idempotencyKey)
}

// UpdateWithExpectedVersion es Update con precondición de versión explícita
// (versionado optimista de PATCH): expectedVersion > 0 exige que la nota siga en
// esa versión y, si otro writer ya la modificó, responde 409 Conflict. Con
// expectedVersion <= 0 la precondición la fija la versión leída dentro de la
// operación (bajo el lock por nota), de modo que una escritura concurrente
// tampoco se pisa silenciosamente.
func (s *NoteService) UpdateWithExpectedVersion(ctx context.Context, userID string, noteID string, title *string, visibility *string, content *string, expectedVersion int64, idempotencyKey string) (*model.Note, error) {
	if content != nil && len(*content) > maxNoteContentLength {
		return nil, newServiceError(utils.ErrFileTooLarge)
	}
	opKey := idemKey("update", idempotencyKey)
	if opKey == "" {
		return s.updateInner(ctx, userID, noteID, title, visibility, content, expectedVersion)
	}
	requestHash := idempotencyRequestHash("update", userID, noteID, derefOrEmpty(title), derefOrEmpty(visibility), derefOrEmpty(content), strconv.FormatInt(expectedVersion, 10))
	// matchHash=true: la misma X-Idempotency-Key con un body distinto es un
	// conflicto 409, no un replay silencioso del resultado anterior.
	_, replay, claimed, idemErr := s.beginIdempotentOperation(ctx, userID, "update", opKey, requestHash, true, nil)
	if idemErr != nil {
		return nil, idemErr
	}
	if replay != nil {
		return replay, nil
	}
	if !claimed {
		return nil, ErrInternalDatabase
	}
	updated, err := s.updateInner(ctx, userID, noteID, title, visibility, content, expectedVersion)
	if err != nil {
		s.releaseIdempotentOperation(ctx, userID, "update", opKey)
		return nil, err
	}
	if updated != nil {
		s.completeIdempotentOperation(ctx, userID, "update", opKey, updated.ID)
	}
	return updated, nil
}

// updateInner implementa la lógica de Update sin idempotencia: validación de
// autor, sincronización causal PG <-> Drive con snapshot previo reversible,
// self-healing de notas legacy sin external_file_id y control de concurrencia.
// Toda la operación se ejecuta bajo shared.WithNoteLock (lock distribuido por
// nota), lo que serializa los cambios de ACL/metadata entre réplicas, y la
// escritura final de metadata usa versionado optimista.
func (s *NoteService) updateInner(ctx context.Context, userID string, noteID string, title *string, visibility *string, content *string, expectedVersion int64) (*model.Note, error) {
	if !utils.ValidateUUID(noteID) {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	var updated *model.Note
	lockErr := s.shared.WithNoteLock(ctx, noteID, func(lockCtx context.Context) error {
		var opErr error
		updated, opErr = s.updateInnerLocked(lockCtx, userID, noteID, title, visibility, content, expectedVersion)
		return opErr
	})
	if lockErr != nil {
		return nil, lockErr
	}
	return updated, nil
}

func (s *NoteService) updateInnerLocked(ctx context.Context, userID string, noteID string, title *string, visibility *string, content *string, expectedVersion int64) (*model.Note, error) {
	note, err := s.observedNotesGetByID(ctx, noteID)
	if err != nil {
		return nil, ErrInternalDatabase
	}
	if note == nil {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	if note.UserID != userID {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	// Precondición de versión del cliente: se evalúa antes de tocar Drive para
	// no aplicar efectos remotos de un cambio que ya no es válido.
	if expectedVersion > 0 && note.Version != expectedVersion {
		return nil, newServiceError(utils.ErrConflict)
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
			prevContent, opErr = s.observedDriveGetFileContent(snapCtx, userID, fileID)
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
			return s.observedDriveUpdateFile(updCtx, userID, fileID, content, newTitle)
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
			newFileID, opErr = s.observedDriveCreateFile(healCtx, userID, noteID, titleForFile+".md", *content)
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
		if err := s.observedNotesUpdateExternalFileID(ctx, noteID, newFileID); err != nil {
			// Compensación: PG falló tras CreateFile OK -> marcar failed_sync
			// y borrar huérfano.
			_ = s.observedNotesUpdateSyncStatus(ctx, noteID, "failed_sync")
			compCtx, cancelComp := withDriveTimeout(ctx)
			compErr := retryDriveOperation(compCtx, driveRetryMaxAttempts, func() error {
				return s.observedDriveDeleteFile(compCtx, userID, newFileID)
			})
			cancelComp()
			if compErr != nil && !drive.IsNotFound(compErr) {
				_ = s.observedNotesEnqueueDriveOperation(ctx, "delete_file", noteID, "", newFileID, userID, map[string]any{"reason": "update_orphan_failed"})
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
			return s.observedDriveUpdateFile(renameCtx, userID, fileID, nil, newTitle)
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
	newDriveFile := !hasDriveFile && content != nil
	permissionsChanged := (visibility != nil && *visibility != note.Visibility) || newDriveFile
	var desiredPermissions DesiredDrivePermissions
	if permissionsChanged {
		shares, policyErr := s.shared.ListByNote(ctx, noteID)
		if policyErr != nil {
			if newDriveFile {
				_ = s.observedNotesUpdateSyncStatus(ctx, noteID, "failed_sync")
			}
			if revertDrive != nil {
				revertDrive()
			}
			return nil, ErrInternalDatabase
		}
		desiredVisibility := note.Visibility
		if visibility != nil {
			desiredVisibility = *visibility
		}
		desiredPermissions, policyErr = s.ComputeDesiredDrivePermissions(ctx, desiredVisibility, shares)
		if policyErr != nil {
			if newDriveFile {
				_ = s.observedNotesUpdateSyncStatus(ctx, noteID, "failed_sync")
			}
			if revertDrive != nil {
				revertDrive()
			}
			return nil, policyErr
		}
	}
	// actualizar metadata PG con versionado optimista. Si el cliente no fijó
	// precondición, se usa la versión leída al inicio: una escritura
	// concurrente que haya ganado la carrera produce ErrConflict.
	expected := expectedVersion
	if expected <= 0 {
		expected = note.Version
		if expected <= 0 {
			expected = 1
		}
	}
	updated, err := s.observedNotesUpdate(ctx, noteID, newTitle, visibility, expected)
	if err != nil {
		if errors.Is(err, repository.ErrConflict) {
			// Otro writer modificó la nota entre la lectura y la escritura:
			// revertir el efecto en Drive (si lo hubo) para no dejar el
			// contenido divergente de la metadata y responder 409.
			if revertDrive != nil {
				revertDrive()
			}
			return nil, newServiceError(utils.ErrConflict)
		}
		// PG falló tras Drive OK: marcar explícitamente la fila como
		// failed_sync (estado reconciliable) y compensar revirtiendo Drive.
		// Si la compensación también falla, compensateDriveUpdate registra la
		// entrada en la DLQ ([CRITICAL_UNRECONCILED]).
		if driveMutated {
			_ = s.observedNotesUpdateSyncStatus(ctx, noteID, "failed_sync")
		}
		if revertDrive != nil {
			revertDrive()
		}
		return nil, ErrInternalDatabase
	}
	if updated == nil {
		// La fila desapareció entre la lectura y la escritura (borrado
		// concurrente): el resultado canónico es not_found.
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	// Desired-state permissions: la metadata (visibilidad) ya está en BD; se
	// converge el ACL de Drive después. Un fallo de Drive no revierte la
	// actualización: queda 'failed' y el reconciliador converge.
	if permissionsChanged {
		if syncErr := s.syncDrivePermissions(ctx, note, desiredPermissions, !newDriveFile); syncErr != nil {
			log.Printf("notes: update nota %s: convergencia de permisos diferida: %v", noteID, syncErr)
		}
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
		return s.observedDriveUpdateFile(driveCtx, userID, fileID, content, title)
	}); err != nil {
		_ = s.observedNotesEnqueueDriveOperation(ctx, "update_file", "", "", fileID, userID, map[string]any{"reason": reason, "content": content, "title": title})
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
	note, err := s.observedNotesGetByID(ctx, noteID)
	if err != nil {
		return ErrInternalDatabase
	}
	if note == nil {
		return newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	if note.UserID != userID {
		return newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	// 1. Borrar metadata en PG (fuente de verdad). Si falla, Drive intacto.
	// La clasificación del error del store es tipada (revalidación con el
	// propio store), sin inspeccionar strings ni filtrar trazas SQL.
	fileID, err := s.observedNotesDeleteWithDriveCleanup(ctx, noteID, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) || errors.Is(err, repository.ErrForbidden) {
			return newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
		}
		if current, checkErr := s.observedNotesGetByID(ctx, noteID); checkErr == nil && current == nil {
			// Borrado concurrente: el recurso ya no existe -> not_found.
			return newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
		}
		return ErrInternalDatabase
	}
	// 2. Borrar archivo en Drive best-effort (nunca revierte el éxito de PG).
	// La nota ya no existe: el ACL local se limpia; los permisos de Drive
	// mueren con el archivo (borrado abajo o por la outbox).
	if cleanupErr := s.shared.DeleteManagedPermissionsByNote(ctx, noteID); cleanupErr != nil {
		log.Printf("notes: delete nota %s: no se pudieron limpiar permisos administrados: %v", noteID, cleanupErr)
	}
	if fileID != "" {
		driveCtx, cancelDrive := withDriveTimeout(ctx)
		delErr := retryDriveOperation(driveCtx, driveRetryMaxAttempts, func() error {
			return s.observedDriveDeleteFile(driveCtx, userID, fileID)
		})
		cancelDrive()
		if delErr != nil {
			if drive.IsNotFound(delErr) {
				log.Printf("[WARN] notes: delete nota %s: archivo %s ya no existe en Drive: %v", noteID, fileID, delErr)
				return nil // idempotente: ya borrado en Drive
			}
			// OAuth revocado/ausente o 500 tras PG OK: no fallar la operación,
			// solo loguear (evita fila huérfana visible por token inválido).
			// La transacción ya dejó la limpieza durable en la outbox.
			log.Printf("[WARN] notes: delete nota %s: PG OK, limpieza Drive pendiente (file %s): %v", noteID, fileID, delErr)
		}
	}
	return nil
}

// Attachments
func (s *NoteService) AddAttachment(ctx context.Context, userID string, noteID string, fileName string, fileType string, data []byte, isInline bool) (*model.Attachment, error) {
	if !utils.ValidateUUID(noteID) {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	note, err := s.observedNotesGetByID(ctx, noteID)
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
		extID, url, opErr = s.observedDriveUploadAttachment(uploadCtx, userID, noteID, fileName, fileType, data, isInline)
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
	att, err := s.observedAttachmentsCreate(ctx, noteID, extID, url, fileType, &fileName, &size, isInline)
	if err != nil {
		// Compensación: PG falló tras UploadAttachment OK -> borrar huérfano.
		compCtx, cancelComp := withDriveTimeout(ctx)
		compErr := retryDriveOperation(compCtx, driveRetryMaxAttempts, func() error {
			return s.observedDriveDeleteAttachment(compCtx, userID, extID)
		})
		cancelComp()
		if compErr != nil {
			_ = s.observedNotesEnqueueDriveOperation(ctx, "delete_attachment", noteID, "", extID, userID, map[string]any{"reason": "add_attachment_orphan"})
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
	note, err := s.observedNotesGetByID(ctx, noteID)
	if err != nil {
		return ErrInternalDatabase
	}
	if note == nil {
		return newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	if note.UserID != userID {
		return newServiceError(utils.ErrForbidden)
	}
	att, err := s.observedAttachmentsGetByID(ctx, attachmentID)
	if err != nil {
		return ErrInternalDatabase
	}
	if att == nil || att.NoteID != noteID {
		return newServiceErrorMsg(utils.ErrNotFound, "adjunto no encontrado")
	}
	// Orden canónico PG-primero (igual que Delete de notas): si PG falla, el
	// binario en Drive queda intacto y la operación es reintentable; si PG OK
	// pero Drive falla, se loguea y se retorna éxito (sin fila huérfana).
	fileID, err := s.observedAttachmentsDeleteAttachmentWithDriveCleanup(ctx, noteID, attachmentID, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return newServiceErrorMsg(utils.ErrNotFound, "adjunto no encontrado")
		}
		if errors.Is(err, repository.ErrForbidden) {
			return newServiceError(utils.ErrForbidden)
		}
		return ErrInternalDatabase
	}
	if fileID == "" {
		return nil
	}
	driveCtx, cancelDrive := withDriveTimeout(ctx)
	delErr := retryDriveOperation(driveCtx, driveRetryMaxAttempts, func() error {
		return s.observedDriveDeleteAttachment(driveCtx, userID, fileID)
	})
	cancelDrive()
	if delErr != nil {
		log.Printf("[WARN] notes: removeAttachment nota %s adjunto %s: PG OK, Drive best-effort falló: %v", noteID, attachmentID, delErr)
	}
	return nil
}

func (s *NoteService) ListAttachments(ctx context.Context, noteID string) ([]*model.Attachment, error) {
	return s.observedAttachmentsListByNote(ctx, noteID)
}

// AddAttachmentExternal registra un adjunto ya subido directo a Drive desde el cliente (external_file_id provisto).
func (s *NoteService) AddAttachmentExternal(ctx context.Context, userID string, noteID string, externalFileID string, fileName string, fileType string, isInline bool, fileSize *int) (*model.Attachment, error) {
	if !utils.ValidateUUID(noteID) {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	if strings.TrimSpace(externalFileID) == "" {
		return nil, newServiceErrorMsg(utils.ErrBadRequest, "external_file_id requerido")
	}
	note, err := s.observedNotesGetByID(ctx, noteID)
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
		return s.observedDriveVerifyFileAccess(verifyCtx, userID, externalFileID)
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
	return s.observedAttachmentsCreate(ctx, noteID, externalFileID, fileURL, fileType, fnPtr, fileSize, isInline)
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
		extID, url, opErr = s.observedDriveUploadAttachment(uploadCtx, userID, "", fileName, fileType, data, false)
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
	note, err := s.observedNotesGetByID(ctx, noteID)
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
	saved, err := s.observedSavedSave(ctx, userID, noteID)
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
// copia archivo en Drive, actualiza sync_status. La idempotencia es durable en
// notes.idempotency_keys (compartida entre réplicas): misma clave + mismo
// origen devuelve el clon ya creado, payload distinto responde 409 y un fallo
// antes de insertar libera la clave (DELETE) mientras que un fallo posterior
// (Drive, timeout, OAuth) la marca 'recoverable' conservando el clon para que
// el retry inmediato lo retome sin duplicar.
//
// Pre-asociación del ResourceID (crash recovery lógico): el noteID del clon se
// genera ANTES del claim y se persiste como resource_id; un reintento con la
// misma clave recupera ese id, reutiliza la fila/meta remota existente y no
// duplica el clon.
func (s *NoteService) Copy(ctx context.Context, userID string, noteID string, idempotencyKey string) (note *model.Note, err error) {
	if !utils.ValidateUUID(noteID) {
		return nil, notFoundNote()
	}
	orig, err := s.observedNotesGetByID(ctx, noteID)
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

	// 0. Reclamar la clave de idempotencia durable antes de crear fila/archivo:
	// misma clave + mismo origen => replay del clon ya creado; misma clave +
	// origen distinto => 409 Conflict. El id del clon se preasigna al claim.
	newNoteID := uuid.NewString()
	recoveredFromCrash := false
	// pgInserted distingue el punto de fallo: antes de materializar el clon en
	// notes.notes el claim se libera (DELETE); después, el clon ya existe y se
	// marca 'recoverable' preservando su id para que el retry no duplique.
	pgInserted := false
	opKey := idemKey("copy", idempotencyKey)
	if opKey != "" {
		requestHash := idempotencyRequestHash("copy", userID, noteID)
		rec, replay, claimed, idemErr := s.beginIdempotentOperation(ctx, userID, "copy", opKey, requestHash, true, &newNoteID)
		if idemErr != nil {
			return nil, idemErr
		}
		if replay != nil {
			return replay, nil
		}
		if !claimed {
			return nil, ErrInternalDatabase
		}
		if rec != nil && rec.ResourceID != nil && strings.TrimSpace(*rec.ResourceID) != "" && *rec.ResourceID != newNoteID {
			newNoteID = *rec.ResourceID
			recoveredFromCrash = true
		}
		pgInserted = recoveredFromCrash
		defer func() {
			if err == nil {
				return
			}
			if pgInserted {
				s.markIdempotentRecoverable(ctx, userID, "copy", opKey, newNoteID, err)
			} else {
				s.releaseIdempotentOperation(ctx, userID, "copy", opKey)
			}
		}()
	}

	// 1. Insertar metadata en PG con sync_status='pending_drive' ANTES de copiar en Drive.
	// Reset SubjectID for the clone to prevent unauthorized access (auditoría:
	// el subjectID del original nunca se hereda al clon). Si el intento anterior
	// murió después de insertar el clon, se reutiliza la misma fila preasociada.
	var nilSubject *string
	if orig.SubjectID != nil {
		log.Printf("notes: copy nota %s: original con subjectID %s, reseteado a nil para clon %s", noteID, *orig.SubjectID, userID)
	}
	var newNote *model.Note
	if recoveredFromCrash {
		newNote, err = s.observedNotesGetByID(ctx, newNoteID)
		if err != nil {
			return nil, ErrInternalDatabase
		}
	}
	if newNote == nil {
		newNote, err = s.observedNotesCreate(ctx, newNoteID, userID, nilSubject, orig.Title, nil, "private", &orig.ID, "pending_drive")
		if err != nil {
			return nil, ErrInternalDatabase
		}
		pgInserted = true
	} else if newNote.SyncStatus == "synced" && newNote.ExternalFileID != nil && strings.TrimSpace(*newNote.ExternalFileID) != "" {
		// El crash ocurrió después de persistir el clon: ya está completo.
		if opKey != "" {
			s.completeIdempotentOperation(ctx, userID, "copy", opKey, newNote.ID)
		}
		return newNote, nil
	}

	// 2. Copiar archivo en Drive del copiador. CopyFile exige OAuth vigente del
	// clonador (destino) y resuelve la lectura del origen de forma desacoplada.
	// Contexto acotado: un cuelgue de red no bloquea el request indefinidamente.
	// El clon se indexa con appProperties (notes_note_id de la nueva nota).
	// En un retry de crash se busca primero el clon huérfano para no duplicarlo.
	newFileID := ""
	if newNote.ExternalFileID != nil {
		newFileID = strings.TrimSpace(*newNote.ExternalFileID)
	}
	createdDriveFile := false
	if newFileID == "" && recoveredFromCrash {
		found, findErr := s.findOrphanDriveFile(ctx, userID, newNote.ID)
		if findErr != nil {
			_ = s.observedNotesUpdateSyncStatus(ctx, newNote.ID, "failed_sync")
			log.Printf("notes: copy retry clon %s: no se pudo verificar el huérfano en Drive: %v", newNote.ID, findErr)
			return nil, ErrDriveUnavailable
		}
		newFileID = found
	}
	if newFileID == "" {
		copyCtx, cancelCopy := withDriveTimeout(ctx)
		err = retryDriveOperation(copyCtx, driveRetryMaxAttempts, func() error {
			var opErr error
			newFileID, opErr = s.observedDriveCopyFile(copyCtx, orig.UserID, *orig.ExternalFileID, userID, newNote.ID, orig.Title)
			return opErr
		})
		cancelCopy()
		if err != nil {
			// Drive falló: marcar nota como failed_sync para reconciliación
			_ = s.observedNotesUpdateSyncStatus(ctx, newNote.ID, "failed_sync")
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
		createdDriveFile = true
	}
	// Defensa: el clon debe tener un fileID distinto al original.
	if newFileID == *orig.ExternalFileID {
		_ = s.observedNotesUpdateSyncStatus(ctx, newNote.ID, "failed_sync")
		compCtx, cancelComp := withDriveTimeout(ctx)
		compErr := retryDriveOperation(compCtx, driveRetryMaxAttempts, func() error {
			return s.observedDriveDeleteFile(compCtx, userID, newFileID)
		})
		cancelComp()
		if compErr != nil && !drive.IsNotFound(compErr) {
			_ = s.observedNotesEnqueueDriveOperation(ctx, "delete_file", newNote.ID, "", newFileID, userID, map[string]any{"reason": "copy_orphan_failed"})
			log.Printf("[CRITICAL_UNRECONCILED] notes: copy fileID duplicado, compensación falló (file %s): %v", newFileID, compErr)
		}
		return nil, ErrDriveUnavailable
	}

	// 3. Actualizar metadata en PG con external_file_id y sync_status='synced'
	if err := s.observedNotesUpdateExternalFileID(ctx, newNote.ID, newFileID); err != nil {
		_ = s.observedNotesUpdateSyncStatus(ctx, newNote.ID, "failed_sync")
		if !createdDriveFile {
			// El clon adoptado pertenece a un intento previo que sí lo creó: no
			// se borra (evita pérdida de datos); queda reconciliable.
			log.Printf("notes: copy retry clon %s: fallo al persistir external_file_id tras adoptar huérfano %s: %v", newNote.ID, newFileID, err)
			return nil, ErrInternalDatabase
		}
		compCtx, cancelComp := withDriveTimeout(ctx)
		compErr := retryDriveOperation(compCtx, driveRetryMaxAttempts, func() error {
			return s.observedDriveDeleteFile(compCtx, userID, newFileID)
		})
		cancelComp()
		if compErr != nil && !drive.IsNotFound(compErr) {
			_ = s.observedNotesEnqueueDriveOperation(ctx, "delete_file", newNote.ID, "", newFileID, userID, map[string]any{"reason": "copy_orphan_failed"})
			log.Printf("[CRITICAL_UNRECONCILED] notes: copy compensación falló (file %s): %v - PG error: %v", newFileID, compErr, err)
		} else {
			log.Printf("notes: copy compensación OK (huérfano Drive %s eliminado tras fallo PG)", newFileID)
		}
		return nil, ErrInternalDatabase
	}

	// Recuperar nota actualizada
	updated, err := s.observedNotesGetByID(ctx, newNote.ID)
	if err != nil {
		return nil, ErrInternalDatabase
	}
	if opKey != "" {
		// Publicar el clon en la clave durable: los replays con la misma clave
		// y el mismo origen reciben exactamente este clon.
		s.completeIdempotentOperation(ctx, userID, "copy", opKey, updated.ID)
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
	note, err := s.observedNotesGetByID(ctx, noteID)
	if err != nil {
		return ErrInternalDatabase
	}
	if note == nil {
		return newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	// Check access before likes so neither existence nor a previous like leaks.
	allowed, err := s.hasReadAccess(ctx, note, userID)
	if err != nil {
		return err
	}
	if !allowed {
		return notFoundNote()
	}
	exists, err := s.likes.Exists(ctx, noteID, userID)
	if err != nil {
		return ErrInternalDatabase
	}
	if exists {
		return newServiceError(utils.ErrAlreadyLiked)
	}
	if err := s.observedLikesLikeAtomic(ctx, noteID, userID); err != nil {
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
	// UnlikeAtomic decrements only when DELETE affects a row and clamps at zero.
	if err := s.observedLikesUnlikeAtomic(ctx, noteID, userID); err != nil {
		return ErrInternalDatabase
	}
	return nil
}

// Share registra la intención de compartir en PostgreSQL ANTES de tocar Drive
// (desired-state permissions): el share se inserta 'pending' y el estado deseado
// se materializa en notes.drive_managed_permissions; después se intenta
// converger el ACL de Drive de inmediato. Si Drive falla, la operación NO se
// revierte: el share persiste y permission_sync_status queda 'failed' para que
// el reconciliador converja en el siguiente tick (convergencia ante fallos).
// Se ejecuta bajo shared.WithNoteLock (lock distribuido por nota) para que dos
// réplicas no intercalen cambios de ACL de la misma nota.
func (s *NoteService) Share(ctx context.Context, ownerID string, noteID string, groupID string, accessMode string) (*model.SharedNote, error) {
	if !utils.ValidateUUID(noteID) || !utils.ValidateUUID(groupID) {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota o grupo no encontrado")
	}
	var shared *model.SharedNote
	lockErr := s.shared.WithNoteLock(ctx, noteID, func(lockCtx context.Context) error {
		var opErr error
		shared, opErr = s.shareLocked(lockCtx, ownerID, noteID, groupID, accessMode)
		return opErr
	})
	if lockErr != nil {
		return nil, lockErr
	}
	return shared, nil
}

func (s *NoteService) shareLocked(ctx context.Context, ownerID string, noteID string, groupID string, accessMode string) (*model.SharedNote, error) {
	if !utils.ValidateAccessMode(accessMode) {
		return nil, newServiceError(utils.ErrInvalidAccessMode)
	}
	note, err := s.observedNotesGetByID(ctx, noteID)
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

	previous, err := s.shared.ListByNote(ctx, noteID)
	if err != nil {
		return nil, ErrInternalDatabase
	}
	candidate := &model.SharedNote{NoteID: noteID, GroupID: groupID, AccessMode: accessMode}
	shares := make([]*model.SharedNote, 0, len(previous)+1)
	shares = append(shares, previous...)
	shares = append(shares, candidate)
	// El estado deseado se calcula ANTES de persistir: si el directorio de
	// miembros no responde no se crea un share que no podamos converger.
	desired, err := s.ComputeDesiredDrivePermissions(ctx, note.Visibility, shares)
	if err != nil {
		return nil, err
	}

	// 1) BD primero: intención durable del share ('pending') y de los permisos.
	shared, err := s.observedSharedCreate(ctx, noteID, groupID, isAdmin, accessMode, followers)
	if err != nil {
		return nil, ErrInternalDatabase
	}
	// 2) Sincronizar Drive; ante fallo queda 'failed' y el reconciliador converge.
	shared.PermissionSyncStatus = model.PermissionSyncInSync
	if syncErr := s.syncDrivePermissions(ctx, note, desired, true); syncErr != nil {
		shared.PermissionSyncStatus = model.PermissionSyncFailed
		log.Printf("notes: share nota %s grupo %s persistido; convergencia Drive diferida: %v", noteID, groupID, syncErr)
	}
	return shared, nil
}

// DesiredDrivePermissions is the union of the note visibility and all active shares.
// Nominal grants remain independent of link access, so removing the last link
// does not accidentally remove access required by restricted groups.
type DesiredDrivePermissions struct {
	Anyone bool
	Emails []string
}

func (s *NoteService) ComputeDesiredDrivePermissions(ctx context.Context, visibility string, shares []*model.SharedNote) (DesiredDrivePermissions, error) {
	policy := DesiredDrivePermissions{Anyone: visibility == "public"}
	groups := map[string]bool{}
	for _, sh := range shares {
		if sh.AccessMode == "link" {
			policy.Anyone = true
		}
		if sh.AccessMode != "restricted" || groups[sh.GroupID] {
			continue
		}
		groups[sh.GroupID] = true
		if s.members == nil {
			return DesiredDrivePermissions{}, ErrInternalDatabase
		}
		emails, err := s.members.ListMemberEmails(ctx, sh.GroupID)
		if err != nil {
			return DesiredDrivePermissions{}, ErrInternalDatabase
		}
		policy.Emails = append(policy.Emails, emails...)
	}
	policy.Emails = NormalizeEmails(policy.Emails)
	return policy, nil
}

// persistManagedPermission hace upsert del estado deseado/observado de un
// permiso administrado en notes.drive_managed_permissions (BD primero).
func (s *NoteService) persistManagedPermission(ctx context.Context, noteID, fileID, principalType, principalKey string, permissionID *string, status string) error {
	_, err := s.observedSharedUpsertManagedPermission(ctx, &model.DriveManagedPermission{
		NoteID:            noteID,
		ExternalFileID:    fileID,
		PrincipalType:     principalType,
		PrincipalKey:      principalKey,
		DrivePermissionID: permissionID,
		Role:              "reader",
		SyncStatus:        status,
	})
	if err != nil {
		return ErrInternalDatabase
	}
	return nil
}

// isPermissionAlreadyExists reconoce el 409 de Drive (permiso ya otorgado):
// un conflicto al re-otorgar significa que el ACL remoto ya convergió, no un
// fallo que deba reintentarse.
func isPermissionAlreadyExists(err error) bool {
	var de *drive.DriveError
	return errors.As(err, &de) && de.Code == 409
}

// syncDrivePermissions converge el ACL remoto del archivo de la nota al estado
// deseado (visibilidad + shares) y persiste cada transición en
// notes.drive_managed_permissions. Contrato de convergencia:
//   - La BD es la fuente de verdad: el estado deseado se registra como
//     'pending' ANTES de llamar a Drive y luego pasa a 'in_sync'/'failed'.
//   - Las bajas se revocan (idempotente ante 404) y su fila se elimina.
//   - Los fallos de Drive/BD se persisten como 'failed' y NO revierten las
//     intenciones ya guardadas: el reconciliador reintenta.
//   - permission_sync_status de notes.shared_notes se actualiza a in_sync o
//     failed según el resultado global de la nota.
//
// El llamador debe sostener el lock distribuido por nota
// (shared.WithNoteLock), que serializa los cambios de ACL de una nota frente a
// Share/Unshare/Update y al reconciliador incluso entre réplicas.
func (s *NoteService) syncDrivePermissions(ctx context.Context, note *model.Note, desired DesiredDrivePermissions, legacyLinkExists bool) error {
	ctx = utils.WithNotesLogger(ctx, utils.NotesLogger(ctx).With("note_id", note.ID))
	if note == nil {
		return nil
	}
	if note.ExternalFileID == nil || strings.TrimSpace(*note.ExternalFileID) == "" {
		// Sin archivo remoto no hay ACL que converger: el estado de aplicación
		// queda sincronizado y el alta del archivo (Create/self-healing) o el
		// reconciliador aplicará los permisos cuando exista el archivo.
		return s.observedSharedUpdateNotePermissionSyncStatus(ctx, note.ID, model.PermissionSyncInSync)
	}
	fileID, ownerID := *note.ExternalFileID, note.UserID
	rows, err := s.shared.ListManagedPermissions(ctx, note.ID)
	if err != nil {
		return ErrInternalDatabase
	}
	// Notas públicas previas al desired-state no tienen fila anyone en la
	// tabla: si el archivo remoto ya existía se asume el link otorgado (evita
	// duplicar grants en Drive) y se materializa la fila para que una baja de
	// visibilidad pueda revocarlo. Con un archivo recién creado no se asume
	// nada: el link debe otorgarse.
	hasAnyoneRow := false
	for _, row := range rows {
		if row.PrincipalType == model.PermissionPrincipalAnyone {
			hasAnyoneRow = true
			break
		}
	}
	if !hasAnyoneRow && note.Visibility == "public" && legacyLinkExists {
		if err := s.persistManagedPermission(ctx, note.ID, fileID, model.PermissionPrincipalAnyone, "", nil, model.PermissionSyncInSync); err != nil {
			log.Printf("notes: sync nota %s: no se pudo materializar el link legacy: %v", note.ID, err)
		}
		rows = append(rows, &model.DriveManagedPermission{
			NoteID:         note.ID,
			ExternalFileID: fileID,
			PrincipalType:  model.PermissionPrincipalAnyone,
			Role:           "reader",
			SyncStatus:     model.PermissionSyncInSync,
		})
	}
	desiredEmails := make(map[string]bool, len(desired.Emails))
	for _, email := range desired.Emails {
		desiredEmails[email] = true
	}
	existing := make(map[string]*model.DriveManagedPermission, len(desired.Emails))
	var anyoneRow *model.DriveManagedPermission
	stale := make([]*model.DriveManagedPermission, 0, len(rows))
	for _, row := range rows {
		if row.ExternalFileID != fileID {
			// El archivo remoto anterior ya no existe (self-healing/legacy):
			// el permiso murió con él, no hay nada que revocar en Drive.
			stale = append(stale, row)
			continue
		}
		switch row.PrincipalType {
		case model.PermissionPrincipalEmail:
			if desiredEmails[row.PrincipalKey] {
				existing[row.PrincipalKey] = row
			} else {
				stale = append(stale, row)
			}
		case model.PermissionPrincipalAnyone:
			if desired.Anyone {
				anyoneRow = row
			} else {
				stale = append(stale, row)
			}
		}
	}

	var firstErr error
	noteErr := func(err error) {
		if firstErr == nil {
			firstErr = err
		}
	}

	// 1) Bajas: persistir 'pending' antes de revocar en Drive (si el proceso
	// muere a mitad, la fila pendiente dispara la convergencia).
	for _, row := range stale {
		if row.ExternalFileID != fileID {
			continue
		}
		if err := s.persistManagedPermission(ctx, note.ID, fileID, row.PrincipalType, row.PrincipalKey, row.DrivePermissionID, model.PermissionSyncPending); err != nil {
			noteErr(err)
		}
	}

	// 2) Altas: persistir el estado deseado como 'pending' antes de tocar Drive.
	toGrant := make([]string, 0, len(desired.Emails))
	for _, email := range desired.Emails {
		row := existing[email]
		if row != nil && row.SyncStatus == model.PermissionSyncInSync {
			continue
		}
		toGrant = append(toGrant, email)
	}
	needAnyone := desired.Anyone && (anyoneRow == nil || anyoneRow.SyncStatus != model.PermissionSyncInSync)
	for _, email := range toGrant {
		if err := s.persistManagedPermission(ctx, note.ID, fileID, model.PermissionPrincipalEmail, email, nil, model.PermissionSyncPending); err != nil {
			noteErr(err)
		}
	}
	if needAnyone {
		if err := s.persistManagedPermission(ctx, note.ID, fileID, model.PermissionPrincipalAnyone, "", nil, model.PermissionSyncPending); err != nil {
			noteErr(err)
		}
	}

	// 3) Drive: revocar bajas (404 = ya no existe, idempotente).
	for _, row := range stale {
		if row.ExternalFileID != fileID {
			if err := s.shared.DeleteManagedPermission(ctx, note.ID, row.PrincipalType, row.PrincipalKey); err != nil {
				noteErr(ErrInternalDatabase)
			}
			continue
		}
		driveCtx, cancel := withDriveTimeout(ctx)
		var revErr error
		switch row.PrincipalType {
		case model.PermissionPrincipalAnyone:
			permissionID := derefOrEmpty(row.DrivePermissionID)
			revErr = retryDriveOperation(driveCtx, driveRetryMaxAttempts, func() error {
				return s.observedDriveRevokePermissionByID(driveCtx, ownerID, fileID, permissionID)
			})
		case model.PermissionPrincipalEmail:
			email := row.PrincipalKey
			revErr = retryDriveOperation(driveCtx, driveRetryMaxAttempts, func() error {
				return s.observedDriveRevokePermission(driveCtx, ownerID, fileID, email)
			})
		}
		cancel()
		if revErr != nil && !drive.IsNotFound(revErr) {
			if err := s.persistManagedPermission(ctx, note.ID, fileID, row.PrincipalType, row.PrincipalKey, row.DrivePermissionID, model.PermissionSyncFailed); err != nil {
				noteErr(err)
			}
			noteErr(ErrDriveUnavailable)
			continue
		}
		if err := s.shared.DeleteManagedPermission(ctx, note.ID, row.PrincipalType, row.PrincipalKey); err != nil {
			noteErr(ErrInternalDatabase)
		}
	}

	// 4) Drive: otorgar altas (409 = ya otorgado, cuenta como convergencia).
	for _, email := range toGrant {
		driveCtx, cancel := withDriveTimeout(ctx)
		grantErr := retryDriveOperation(driveCtx, driveRetryMaxAttempts, func() error {
			return s.observedDriveGrantPermission(driveCtx, ownerID, fileID, email, "reader")
		})
		cancel()
		if grantErr != nil && !isPermissionAlreadyExists(grantErr) {
			if err := s.persistManagedPermission(ctx, note.ID, fileID, model.PermissionPrincipalEmail, email, nil, model.PermissionSyncFailed); err != nil {
				noteErr(err)
			}
			noteErr(ErrDriveUnavailable)
			continue
		}
		if err := s.persistManagedPermission(ctx, note.ID, fileID, model.PermissionPrincipalEmail, email, nil, model.PermissionSyncInSync); err != nil {
			noteErr(err)
		}
	}
	if needAnyone {
		driveCtx, cancel := withDriveTimeout(ctx)
		var permissionID string
		grantErr := retryDriveOperation(driveCtx, driveRetryMaxAttempts, func() error {
			var opErr error
			permissionID, opErr = s.observedDriveGrantLinkPermission(driveCtx, ownerID, fileID)
			return opErr
		})
		cancel()
		if grantErr != nil && !isPermissionAlreadyExists(grantErr) {
			if err := s.persistManagedPermission(ctx, note.ID, fileID, model.PermissionPrincipalAnyone, "", nil, model.PermissionSyncFailed); err != nil {
				noteErr(err)
			}
			noteErr(ErrDriveUnavailable)
		} else {
			var pid *string
			if strings.TrimSpace(permissionID) != "" {
				pid = &permissionID
			}
			if err := s.persistManagedPermission(ctx, note.ID, fileID, model.PermissionPrincipalAnyone, "", pid, model.PermissionSyncInSync); err != nil {
				noteErr(err)
			}
		}
	}

	status := model.PermissionSyncInSync
	if firstErr != nil {
		status = model.PermissionSyncFailed
	}
	if err := s.observedSharedUpdateNotePermissionSyncStatus(ctx, note.ID, status); err != nil {
		noteErr(ErrInternalDatabase)
	}
	return firstErr
}

// reconcileManagedPermissions converge las notas con permisos pendientes o
// fallidos en notes.drive_managed_permissions/shared_notes. Es idempotente:
// converger una nota ya sincronizada no genera llamadas a Drive. Un fallo de
// Social (directorio) no descarta la intención: queda 'failed' y se reintenta.
func (s *NoteService) reconcileManagedPermissions(ctx context.Context) error {
	noteIDs, err := s.shared.ListNotesWithPendingPermissionSync(ctx, 100)
	if err != nil {
		return ErrInternalDatabase
	}
	var result error
	for _, noteID := range noteIDs {
		if err := ctx.Err(); err != nil {
			return errors.Join(result, err)
		}
		note, err := s.observedNotesGetByID(ctx, noteID)
		if err != nil {
			result = errors.Join(result, ErrInternalDatabase)
			continue
		}
		if note == nil {
			// Nota borrada: la tabla sin FK puede conservar filas huérfanas.
			if err := s.shared.DeleteManagedPermissionsByNote(ctx, noteID); err != nil {
				result = errors.Join(result, ErrInternalDatabase)
			}
			continue
		}
		lockErr := s.shared.WithNoteLock(ctx, noteID, func(lockCtx context.Context) error {
			shares, err := s.shared.ListByNote(lockCtx, noteID)
			if err != nil {
				return ErrInternalDatabase
			}
			desired, err := s.ComputeDesiredDrivePermissions(lockCtx, note.Visibility, shares)
			if err != nil {
				// No se puede calcular el estado deseado (p. ej. Social caído):
				// preservar failed para el siguiente tick sin perder la intención.
				_ = s.observedSharedUpdateNotePermissionSyncStatus(lockCtx, noteID, model.PermissionSyncFailed)
				return nil
			}
			return s.syncDrivePermissions(lockCtx, note, desired, true)
		})
		if lockErr != nil {
			result = errors.Join(result, lockErr)
		}
	}
	return result
}

// Unshare elimina el share en PostgreSQL ANTES de revocar el ACL de Drive
// (desired-state permissions). El estado deseado pasa a materializarse en
// notes.drive_managed_permissions: las filas afectadas se marcan 'pending' y se
// converge de inmediato; si Drive falla, la baja queda persistida y el
// reconciliador la converge después sin bloquear al usuario ni resucitar el
// share. Se ejecuta bajo shared.WithNoteLock (lock distribuido por nota).
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
	return s.shared.WithNoteLock(ctx, sh.NoteID, func(lockCtx context.Context) error {
		return s.unshareLocked(lockCtx, userID, sharedNoteID)
	})
}

func (s *NoteService) unshareLocked(ctx context.Context, userID string, sharedNoteID string) error {
	sh, err := s.shared.GetByID(ctx, sharedNoteID)
	if err != nil {
		return ErrInternalDatabase
	}
	if sh == nil {
		return newServiceErrorMsg(utils.ErrNotFound, "compartición no encontrada")
	}
	note, err := s.observedNotesGetByID(ctx, sh.NoteID)
	if err != nil {
		return ErrInternalDatabase
	}
	if note == nil {
		// nota ya borrada, borrar share
		_ = s.observedSharedDelete(ctx, sharedNoteID)
		_ = s.shared.DeleteManagedPermissionsByNote(ctx, sh.NoteID)
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
	// 1) BD primero: el share deja de existir en la aplicación.
	if err := s.observedSharedDelete(ctx, sharedNoteID); err != nil {
		return ErrInternalDatabase
	}
	// 1b) Marcar los permisos administrados como pendientes: si la convergencia
	// inmediata no puede completarse (Drive o Social caídos), el reconciliador
	// tendrá el disparo durable para revocar lo que ya no es deseado.
	if err := s.observedSharedMarkManagedPermissionsPending(ctx, note.ID); err != nil {
		log.Printf("notes: unshare nota %s: no se pudo marcar permisos pendientes: %v", note.ID, err)
	}
	// 2) Convergencia inmediata best-effort del estado deseado restante.
	remaining, err := s.shared.ListByNote(ctx, note.ID)
	if err != nil {
		_ = s.observedSharedUpdateNotePermissionSyncStatus(ctx, note.ID, model.PermissionSyncFailed)
		return nil
	}
	desired, err := s.ComputeDesiredDrivePermissions(ctx, note.Visibility, remaining)
	if err != nil {
		_ = s.observedSharedUpdateNotePermissionSyncStatus(ctx, note.ID, model.PermissionSyncFailed)
		return nil
	}
	if syncErr := s.syncDrivePermissions(ctx, note, desired, true); syncErr != nil {
		log.Printf("notes: unshare nota %s: convergencia Drive diferida: %v", note.ID, syncErr)
	}
	return nil
}

func (s *NoteService) UnshareAll(ctx context.Context, userID string, groupID string) error {
	if !utils.ValidateUUID(groupID) || !utils.ValidateUUID(userID) {
		return newServiceErrorMsg(utils.ErrBadRequest, "ids inv?lidos")
	}
	// Page through the owner's notes, not the shares being deleted: removing a
	// share must not invalidate the pagination cursor or skip another author.
	cursor := ""
	for {
		notes, next, err := s.observedNotesListByUser(ctx, userID, cursor, 100)
		if err != nil {
			return ErrInternalDatabase
		}
		for _, note := range notes {
			shares, err := s.shared.ListByNote(ctx, note.ID)
			if err != nil {
				return ErrInternalDatabase
			}
			for _, sh := range shares {
				if sh.GroupID == groupID {
					if err := s.Unshare(ctx, userID, sh.ID); err != nil {
						return err
					}
				}
			}
		}
		if next == "" {
			return nil
		}
		cursor = next
	}
}

func (s *NoteService) GetAccess(ctx context.Context, requesterID string, noteID string) (*model.NoteAccessResponse, error) {
	if !utils.ValidateUUID(noteID) {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	note, err := s.observedNotesGetByID(ctx, noteID)
	if err != nil {
		return nil, ErrInternalDatabase
	}
	if note == nil {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	canRead := false
	accessMode := "restricted"
	if note.UserID == requesterID {
		canRead = true
		accessMode = "owner"
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
	driveURL := fmt.Sprintf("https://drive.google.com/file/d/%s/view", *note.ExternalFileID)
	// El estado real de convergencia de los permisos de Drive se deriva de
	// notes.drive_managed_permissions (no del sync_status del archivo .md):
	// failed > pending > synced, de forma inequívoca para link y restricted.
	driveSyncStatus, err := s.GetAccessInfo(ctx, note.ID)
	if err != nil {
		return nil, err
	}
	driveAccessVerified, driveConnectionRequired := s.driveAccessFlags(ctx, requesterID, note.Visibility, accessMode, *note.ExternalFileID)
	return &model.NoteAccessResponse{
		CanRead:                 canRead,
		CanWrite:                note.UserID == requesterID,
		Visibility:              note.Visibility,
		AccessMode:              accessMode,
		DriveSyncStatus:         driveSyncStatus,
		DriveAccessVerified:     driveAccessVerified,
		DriveConnectionRequired: driveConnectionRequired,
		DriveURL:                &driveURL,
	}, nil
}

// driveAccessFlags deriva las dos banderas de Drive de GET /notes/{id}/access
// con semántica estricta por modo de acceso:
//
//   - link, public (cualquier visibilidad pública) y restricted: el archivo se
//     sirve con la autorización de la aplicación (enlace público, permiso de
//     lectura en Drive o permiso de grupo), no con el token del solicitante.
//     Drive no se consulta: ambas banderas quedan en false y una eventual falta
//     de conexión OAuth no se reporta, porque no impide abrir la nota.
//   - owner sobre nota no pública: el archivo sólo es legible con la conexión
//     del autor, así que se verifica con VerifyFileAccess (best-effort y con
//     contexto acotado). err == nil -> verificado y sin conexión requerida;
//     drive.OAuthError -> no verificado y conexión requerida; 403/404 (permisos
//     o archivo ausente) o cualquier otro fallo del upstream -> no verificado y
//     sin conexión requerida, porque no es falta de token.
//
// GetAccess nunca falla por Drive: los errores de la verificación se traducen a
// banderas.
func (s *NoteService) driveAccessFlags(ctx context.Context, requesterID, visibility, accessMode, fileID string) (verified, connectionRequired bool) {
	if accessMode != "owner" || visibility == "public" {
		return false, false
	}
	if s.drive == nil || strings.TrimSpace(fileID) == "" {
		return false, false
	}
	verifyCtx, cancelVerify := withDriveTimeout(ctx)
	err := s.observedDriveVerifyFileAccess(verifyCtx, requesterID, fileID)
	cancelVerify()
	if err == nil {
		return true, false
	}
	if drive.IsOAuthError(err) {
		return false, true
	}
	return false, false
}

// Estados de convergencia de los permisos administrados de Drive expuestos por
// NoteAccessResponse.DriveSyncStatus.
const (
	DriveSyncStatusSynced  = "synced"
	DriveSyncStatusPending = "pending"
	DriveSyncStatusFailed  = "failed"
)

// GetAccessInfo deriva el estado real de convergencia de los permisos de Drive
// de una nota a partir de notes.drive_managed_permissions:
//   - si alguna fila está 'failed'  -> DriveSyncStatusFailed;
//   - si alguna fila está 'pending' -> DriveSyncStatusPending;
//   - si todas están 'in_sync' (o no hay filas) -> DriveSyncStatusSynced.
//
// El orden de precedencia failed > pending > synced garantiza que un fallo de
// convergencia nunca quede enmascarado por filas ya sincronizadas.
func (s *NoteService) GetAccessInfo(ctx context.Context, noteID string) (string, error) {
	rows, err := s.shared.ListManagedPermissions(ctx, noteID)
	if err != nil {
		return "", ErrInternalDatabase
	}
	return DriveSyncStatusFromManagedPermissions(rows), nil
}

// DriveSyncStatusFromManagedPermissions calcula el estado agregado de
// convergencia de las filas de notes.drive_managed_permissions.
func DriveSyncStatusFromManagedPermissions(rows []*model.DriveManagedPermission) string {
	status := DriveSyncStatusSynced
	for _, row := range rows {
		if row == nil {
			continue
		}
		switch row.SyncStatus {
		case model.PermissionSyncFailed:
			return DriveSyncStatusFailed
		case model.PermissionSyncPending:
			status = DriveSyncStatusPending
		}
	}
	return status
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
		n, _ := s.observedNotesGetByID(ctx, sh.NoteID)
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
// lease del claim de idempotencia (2 min) como el presupuesto de una operación
// con reintentos: un Create/Copy en vuelo nunca es compensado por una ejecución
// concurrente del reconciler.
const reconcilePendingMinAge = 15 * time.Minute

// reconcileTimeout acota el presupuesto total de una pasada de reconciliación:
// con muchas notas pendientes o un Drive lento, el job no puede quedar colgado
// indefinidamente. Cada borrado individual ya lleva su propio withDriveTimeout.
const reconcileTimeout = 30 * time.Second

// ReconcilePendingNotes repairs stale pending/failed notes and drains the durable
// Drive outbox. A readable empty Markdown file is valid and must be preserved.
// Antes de descartar una nota sin external_file_id busca por appProperties
// (FindFileByNoteID) el archivo que un crash pudo dejar huérfano y, si existe,
// adopta ese fileID para recuperar el apunte.
func (s *NoteService) ReconcilePendingNotes(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, reconcileTimeout)
	defer cancel()
	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	queueErr := s.reconcileDriveOperations(ctx)
	permissionErr := s.reconcileManagedPermissions(ctx)
	type pendingStore interface {
		GetPendingSyncNotes(ctx context.Context) ([]*model.Note, error)
	}
	ps, ok := s.notes.(pendingStore)
	if !ok {
		return errors.Join(queueErr, permissionErr)
	}
	notes, err := ps.GetPendingSyncNotes(ctx)
	if err != nil {
		return errors.Join(queueErr, permissionErr, ErrInternalDatabase)
	}
	result := errors.Join(queueErr, permissionErr)
	for _, n := range notes {
		if err := ctx.Err(); err != nil {
			return errors.Join(result, err)
		}
		if n.SyncStatus != "pending_drive" && n.SyncStatus != "failed_sync" {
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
			// Crash recovery: el alta en Drive pudo completarse antes de que el
			// proceso muriera sin persistir external_file_id. Se busca el
			// archivo huérfano por appProperties (notes_note_id) antes de
			// descartar la nota: si aparece, se recupera el apunte en lugar de
			// perder su contenido.
			findCtx, cancelFind := withDriveTimeout(ctx)
			var orphanFileID string
			findErr := retryDriveOperation(findCtx, driveRetryMaxAttempts, func() error {
				var opErr error
				orphanFileID, opErr = s.observedDriveFindFileByNoteID(findCtx, n.UserID, n.ID)
				return opErr
			})
			cancelFind()
			if findErr == nil && strings.TrimSpace(orphanFileID) != "" {
				if err := s.observedNotesUpdateExternalFileID(ctx, n.ID, orphanFileID); err != nil {
					if errors.Is(err, repository.ErrNotFound) {
						continue // la fila desapareció: nada que recuperar
					}
					result = errors.Join(result, ErrInternalDatabase)
				}
				continue
			}
			if findErr != nil {
				// No se puede probar la ausencia del archivo (OAuth, permiso o
				// fallo transitorio): preservar la fila para el siguiente tick.
				log.Printf("notes: reconcile note %s: no se pudo buscar huérfano por appProperties: %v", n.ID, findErr)
				if err := s.observedNotesUpdateSyncStatus(ctx, n.ID, "failed_sync"); err != nil {
					result = errors.Join(result, ErrInternalDatabase)
				}
				continue
			}
			if n.SyncStatus == "failed_sync" {
				continue // No evidence that failed user data may safely be deleted.
			}
			if _, err := s.observedNotesDeleteWithDriveCleanup(ctx, n.ID, n.UserID); err != nil && !errors.Is(err, repository.ErrNotFound) {
				result = errors.Join(result, ErrInternalDatabase)
			}
			continue
		}
		driveCtx, cancelDrive := withDriveTimeout(ctx)
		statErr := retryDriveOperation(driveCtx, driveRetryMaxAttempts, func() error {
			_, err := s.observedDriveGetFileContent(driveCtx, n.UserID, *n.ExternalFileID)
			return err
		})
		cancelDrive()
		if statErr == nil {
			// Zero bytes is valid Markdown, including for recovered failed_sync notes.
			if err := s.observedNotesUpdateSyncStatus(ctx, n.ID, "synced"); err != nil {
				result = errors.Join(result, ErrInternalDatabase)
			}
			continue
		}
		if err := ctx.Err(); err != nil {
			return errors.Join(result, err)
		}
		if !drive.IsNotFound(statErr) || n.SyncStatus == "failed_sync" {
			// OAuth, permission and transient failures do not establish data loss.
			log.Printf("notes: reconcile note %s: preserving failed_sync after Drive read error: %v", n.ID, statErr)
			if err := s.observedNotesUpdateSyncStatus(ctx, n.ID, "failed_sync"); err != nil {
				result = errors.Join(result, ErrInternalDatabase)
			}
			continue
		}
		// Only stale incomplete creation with a missing remote file is removed;
		// the transaction also durably schedules cleanup for its attachments.
		if err := s.Delete(ctx, n.UserID, n.ID); err != nil {
			result = errors.Join(result, err)
		}
	}
	return result
}

func (s *NoteService) reconcileDriveOperations(ctx context.Context) error {
	var result error
	// Claim one job at a time so queued work cannot outlive its lease while
	// waiting behind slow Drive calls. Bound the work per scheduler tick.
	for i := 0; i < 100; i++ {
		if err := ctx.Err(); err != nil {
			return errors.Join(result, err)
		}
		ops, err := s.observedNotesClaimDriveOperations(ctx, 1, 2*reconcileTimeout)
		if err != nil {
			return errors.Join(result, ErrInternalDatabase)
		}
		if len(ops) == 0 {
			break
		}
		op := ops[0]
		driveCtx, cancelDrive := withDriveTimeout(ctx)
		opErr := retryDriveOperation(driveCtx, driveRetryMaxAttempts, func() error {
			return s.executeDriveOperation(driveCtx, op)
		})
		cancelDrive()
		if opErr == nil {
			if err := s.observedNotesCompleteDriveOperation(ctx, op.ID); err != nil {
				result = errors.Join(result, ErrInternalDatabase)
			}
			continue
		}
		// Exponential retry delay, capped at one hour; the claimed attempt is 1-based.
		delay := 30 * time.Second
		for attempt := 1; attempt < op.Attempts && delay < time.Hour; attempt++ {
			delay *= 2
		}
		if delay > time.Hour {
			delay = time.Hour
		}
		if err := s.observedNotesFailDriveOperation(ctx, op.ID, opErr.Error(), time.Now().Add(delay)); err != nil {
			result = errors.Join(result, ErrInternalDatabase)
		}
		log.Printf("notes: Drive operation %s (%s) scheduled for retry: %v", op.ID, op.Operation, opErr)
	}
	return result
}

func (s *NoteService) executeDriveOperation(ctx context.Context, op *model.DriveOperation) (err error) {
	started := time.Now()
	ctx = utils.WithNotesLogger(ctx, utils.NotesLogger(ctx).With(
		"note_id", derefOrEmpty(op.NoteID), "attachment_id", derefOrEmpty(op.AttachmentID),
		"outbox_id", op.ID, "operation", op.Operation, "drive_owner_user_id", op.OwnerUserID))
	defer func() {
		logNotesResult(ctx, started, "OUTBOX EXECUTE", derefOrEmpty(op.ExternalFileID), err,
			"drive_file_id", derefOrEmpty(op.ExternalFileID))
	}()
	if op.ExternalFileID == nil || strings.TrimSpace(*op.ExternalFileID) == "" {
		return &drive.DriveError{Code: 400, Message: "outbox operation requires external_file_id"}
	}
	switch op.Operation {
	case "delete_file":
		err = s.observedDriveDeleteFile(ctx, op.OwnerUserID, *op.ExternalFileID)
	case "delete_attachment":
		err = s.observedDriveDeleteAttachment(ctx, op.OwnerUserID, *op.ExternalFileID)
	case "update_file":
		var payload struct {
			Content *string `json:"content"`
			Title   *string `json:"title"`
		}
		if err := json.Unmarshal(op.Payload, &payload); err != nil {
			return &drive.DriveError{Code: 400, Message: "invalid update_file payload"}
		}
		if payload.Content == nil && payload.Title == nil {
			return &drive.DriveError{Code: 400, Message: "update_file payload requires content or title"}
		}
		return s.observedDriveUpdateFile(ctx, op.OwnerUserID, *op.ExternalFileID, payload.Content, payload.Title)
	default:
		return &drive.DriveError{Code: 400, Message: "unsupported outbox operation"}
	}
	if drive.IsNotFound(err) {
		return nil // Replaying a successful deletion is safe.
	}
	return err
}
