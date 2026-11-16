-- Confirmación de cuenta por correo.
--
-- identity.users.email_verified_at: NULL = correo sin confirmar. Esta
-- migración NO rellena usuarios existentes ni bloquea el login: ambas cosas
-- corresponden a tareas posteriores (backfill y gate de login).
--
-- identity.email_verifications: un token pendiente por usuario (PK user_id).
-- Emitir uno nuevo reemplaza el anterior, que deja de servir al instante.
--   token_hash         SHA-256 (32 bytes) del token; el token en claro nunca
--                      se guarda. UNIQUE para buscarlo por índice.
--   expires_at         vencimiento del token vigente.
--   requested_at       último envío (enfriamiento entre envíos).
--   window_started_at  inicio de la ventana de 1 hora del límite de envíos.
--   sends_in_window    envíos dentro de esa ventana.
--   consumed_at        cuándo se usó; un token consumido no vuelve a servir.
-- La fila se conserva tras la confirmación para mantener el límite de envíos
-- y responder «ya confirmado» si el enlace se abre otra vez.
--
-- Idempotente (IF NOT EXISTS): init.sql ya crea ambas cosas en una base nueva
-- y validate-migrations.sh aplica init.sql antes que esta carpeta.

ALTER TABLE identity.users
    ADD COLUMN IF NOT EXISTS email_verified_at timestamptz;

CREATE TABLE IF NOT EXISTS identity.email_verifications (
    user_id uuid PRIMARY KEY REFERENCES identity.users(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL
        CONSTRAINT email_verifications_token_hash_key UNIQUE
        CONSTRAINT email_verifications_token_hash_len CHECK (octet_length(token_hash) = 32),
    expires_at timestamptz NOT NULL,
    requested_at timestamptz NOT NULL DEFAULT now(),
    window_started_at timestamptz NOT NULL DEFAULT now(),
    sends_in_window integer NOT NULL DEFAULT 1 CHECK (sends_in_window >= 0),
    consumed_at timestamptz
);
