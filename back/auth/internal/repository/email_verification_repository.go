package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Política de reenvío por cuenta, aplicada en SQL de forma atómica (válida
// con varias réplicas): 60 s entre envíos y como máximo 5 por hora.
const (
	EmailVerificationCooldown     = 60 * time.Second
	EmailVerificationWindow       = time.Hour
	EmailVerificationMaxPerWindow = 5
)

// IssueOutcome es el resultado de intentar emitir un token.
type IssueOutcome string

const (
	IssueIssued          IssueOutcome = "issued"
	IssueAlreadyVerified IssueOutcome = "already_verified"
	IssueThrottled       IssueOutcome = "throttled"
	IssueUserNotFound    IssueOutcome = "user_not_found"
)

// ConfirmOutcome es el resultado de canjear un token.
type ConfirmOutcome string

const (
	ConfirmVerified        ConfirmOutcome = "verified"
	ConfirmAlreadyVerified ConfirmOutcome = "already_verified"
	ConfirmInvalid         ConfirmOutcome = "invalid" // inexistente o reemplazado
	ConfirmExpired         ConfirmOutcome = "expired"
	ConfirmUsed            ConfirmOutcome = "used"
)

// EmailVerificationRepository guarda solo el SHA-256 de cada token en
// identity.email_verifications (migración 007).
//
// Orden de bloqueo común a Issue y Confirm: primero la fila de
// identity.users (FOR NO KEY UPDATE, que no bloquea las FK de otras tablas)
// y después la de email_verifications. Así una emisión y un canje
// simultáneos se serializan sin interbloqueos.
type EmailVerificationRepository struct {
	pool           *pgxpool.Pool
	cooldown       time.Duration
	window         time.Duration
	maxPerWindow   int
	statementLimit time.Duration
}

func NewEmailVerificationRepository(pool *pgxpool.Pool) *EmailVerificationRepository {
	return &EmailVerificationRepository{
		pool:           pool,
		cooldown:       EmailVerificationCooldown,
		window:         EmailVerificationWindow,
		maxPerWindow:   EmailVerificationMaxPerWindow,
		statementLimit: 5 * time.Second,
	}
}

func seconds(d time.Duration) float64 { return d.Seconds() }

// Issue emite (o reemplaza) el token de userID. El enfriamiento y el tope por
// hora se evalúan en el mismo UPSERT, así que dos peticiones simultáneas o de
// réplicas distintas no pueden saltárselos.
func (r *EmailVerificationRepository) Issue(ctx context.Context, userID string, tokenHash []byte, ttl time.Duration) (IssueOutcome, error) {
	ctx, cancel := context.WithTimeout(ctx, r.statementLimit)
	defer cancel()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var verified bool
	err = tx.QueryRow(ctx, `SELECT email_verified_at IS NOT NULL FROM identity.users WHERE id = $1 FOR NO KEY UPDATE`, userID).Scan(&verified)
	if errors.Is(err, pgx.ErrNoRows) {
		return IssueUserNotFound, nil
	}
	if err != nil {
		return "", err
	}
	if verified {
		return IssueAlreadyVerified, nil
	}
	result, err := tx.Exec(ctx, `
 INSERT INTO identity.email_verifications AS ev (user_id, token_hash, expires_at, requested_at, window_started_at, sends_in_window)
 VALUES ($1, $2, now() + $3 * interval '1 second', now(), now(), 1)
 ON CONFLICT (user_id) DO UPDATE SET
   token_hash = EXCLUDED.token_hash,
   expires_at = EXCLUDED.expires_at,
   requested_at = now(),
   consumed_at = NULL,
   window_started_at = CASE WHEN ev.window_started_at <= now() - $5 * interval '1 second' THEN now() ELSE ev.window_started_at END,
   sends_in_window = CASE WHEN ev.window_started_at <= now() - $5 * interval '1 second' THEN 1 ELSE ev.sends_in_window + 1 END
 WHERE ev.requested_at <= now() - $4 * interval '1 second'
   AND (ev.window_started_at <= now() - $5 * interval '1 second' OR ev.sends_in_window < $6)`,
		userID, tokenHash, seconds(ttl), seconds(r.cooldown), seconds(r.window), r.maxPerWindow)
	if err != nil {
		return "", err
	}
	if result.RowsAffected() != 1 {
		return IssueThrottled, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return IssueIssued, nil
}

// ReleaseCooldown retrocede requested_at para permitir un reenvío inmediato
// tras un fallo de SMTP. El tope por hora no se devuelve: sigue limitando a
// quien provoque fallos repetidos.
func (r *EmailVerificationRepository) ReleaseCooldown(ctx context.Context, userID string, tokenHash []byte) error {
	ctx, cancel := context.WithTimeout(ctx, r.statementLimit)
	defer cancel()
	_, err := r.pool.Exec(ctx, `
 UPDATE identity.email_verifications
 SET requested_at = now() - $3 * interval '1 second'
 WHERE user_id = $1 AND token_hash = $2 AND consumed_at IS NULL`,
		userID, tokenHash, seconds(r.cooldown))
	return err
}

// Confirm canjea el token en una transacción: marca la cuenta como
// confirmada y consume el token a la vez. Con dos canjes simultáneos del
// mismo token, el segundo espera el bloqueo y encuentra consumed_at ya fijado.
func (r *EmailVerificationRepository) Confirm(ctx context.Context, tokenHash []byte) (ConfirmOutcome, string, error) {
	ctx, cancel := context.WithTimeout(ctx, r.statementLimit)
	defer cancel()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback(ctx)

	var userID string
	err = tx.QueryRow(ctx, `SELECT user_id FROM identity.email_verifications WHERE token_hash = $1`, tokenHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ConfirmInvalid, "", nil
	}
	if err != nil {
		return "", "", err
	}
	var verified bool
	err = tx.QueryRow(ctx, `SELECT email_verified_at IS NOT NULL FROM identity.users WHERE id = $1 FOR NO KEY UPDATE`, userID).Scan(&verified)
	if errors.Is(err, pgx.ErrNoRows) {
		return ConfirmInvalid, "", nil
	}
	if err != nil {
		return "", "", err
	}
	// Releer bajo bloqueo y comprobar que el token sigue siendo el vigente
	// (un reenvío concurrente pudo reemplazarlo) y que pertenece a userID.
	var consumed, expired bool
	err = tx.QueryRow(ctx, `
 SELECT consumed_at IS NOT NULL, expires_at <= now()
 FROM identity.email_verifications
 WHERE user_id = $1 AND token_hash = $2
 FOR UPDATE`, userID, tokenHash).Scan(&consumed, &expired)
	if errors.Is(err, pgx.ErrNoRows) {
		return ConfirmInvalid, "", nil
	}
	if err != nil {
		return "", "", err
	}
	switch {
	case consumed && verified:
		return ConfirmAlreadyVerified, userID, nil
	case consumed:
		return ConfirmUsed, userID, nil
	case expired:
		return ConfirmExpired, userID, nil
	}
	outcome := ConfirmAlreadyVerified
	if !verified {
		if _, err := tx.Exec(ctx, `UPDATE identity.users SET email_verified_at = now() WHERE id = $1`, userID); err != nil {
			return "", "", err
		}
		outcome = ConfirmVerified
	}
	if _, err := tx.Exec(ctx, `UPDATE identity.email_verifications SET consumed_at = now() WHERE user_id = $1`, userID); err != nil {
		return "", "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", err
	}
	return outcome, userID, nil
}

// EmailVerificationSchemaReady informa si la migración 007 está aplicada
// (tabla identity.email_verifications y columna users.email_verified_at). Al
// arrancar solo se advierte: sin ella el registro sigue funcionando y lo que
// falla es la emisión y el canje de tokens, que quedan registrados en el log.
func EmailVerificationSchemaReady(ctx context.Context, q SchemaQuerier) (bool, error) {
	var ok bool
	err := q.QueryRow(ctx, `
 SELECT to_regclass('identity.email_verifications') IS NOT NULL
    AND EXISTS (SELECT 1 FROM information_schema.columns
                WHERE table_schema = 'identity' AND table_name = 'users' AND column_name = 'email_verified_at')`).Scan(&ok)
	return ok, err
}

// IsEmailVerified implementa el gate de Login/Refresh
// (EMAIL_VERIFICATION_REQUIRED=true). Un usuario inexistente cuenta como no
// confirmado; Login y Refresh ya lo habrán rechazado antes por otros motivos.
func (r *EmailVerificationRepository) IsEmailVerified(ctx context.Context, userID string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, r.statementLimit)
	defer cancel()
	var verified bool
	err := r.pool.QueryRow(ctx, `SELECT email_verified_at IS NOT NULL FROM identity.users WHERE id = $1`, userID).Scan(&verified)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return verified, err
}

// EmailVerificationGateSchemaReady exige, para activar el gate, la migración
// 007 y el backfill 008 ya ejecutado (fila de control presente). Sin el
// backfill, el gate bloquearía a todas las cuentas antiguas.
func EmailVerificationGateSchemaReady(ctx context.Context, q SchemaQuerier) (bool, error) {
	ready, err := EmailVerificationSchemaReady(ctx, q)
	if err != nil || !ready {
		return false, err
	}
	var applied bool
	err = q.QueryRow(ctx, `SELECT to_regclass('identity.email_verification_backfill') IS NOT NULL`).Scan(&applied)
	if err != nil || !applied {
		return false, err
	}
	err = q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.email_verification_backfill)`).Scan(&applied)
	return applied, err
}
