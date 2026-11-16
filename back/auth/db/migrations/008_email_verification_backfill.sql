-- Backfill único de la confirmación de correo: marca como confirmadas las
-- cuentas que ya existían antes de exigir la confirmación, para que el gate
-- de login (EMAIL_VERIFICATION_REQUIRED=true) no bloquee a usuarios antiguos.
--
-- Requiere 007_email_verification.sql. Orden de despliegue:
--   1. 007   2. 008 (con la versión ANTERIOR de Auth todavía en servicio)
--   3. Auth nuevo con EMAIL_VERIFICATION_REQUIRED=false   4. Flutter
--   5. EMAIL_VERIFICATION_REQUIRED=true
-- Las cuentas creadas después del corte quedan sin confirmar.
--
-- Una sola vez: identity.email_verification_backfill guarda una única fila
-- (corte y cuentas marcadas). Si ya existe, el bloque no hace nada, así que
-- volver a aplicar esta migración nunca confirma cuentas posteriores. En una
-- base nueva, init.sql ya inserta esa fila: ahí nunca hay backfill.
--
-- Atomicidad: el bloque DO es una sola sentencia y por lo tanto una sola
-- transacción (psql -f en autocommit, o dentro del BEGIN del llamador). LOCK
-- TABLE ... SHARE ROW EXCLUSIVE espera a los INSERT/UPDATE/DELETE en curso y
-- bloquea los nuevos hasta el COMMIT. El corte es exactamente «cuentas
-- confirmadas en la base al tomar el bloqueo». También serializa dos
-- ejecuciones simultáneas de esta migración. El bloqueo dura lo que tarda un
-- UPDATE sobre identity.users; los registros que lleguen mientras tanto
-- esperan y se crean después del corte.

CREATE TABLE IF NOT EXISTS identity.email_verification_backfill (
    id boolean PRIMARY KEY DEFAULT true
        CONSTRAINT email_verification_backfill_single_row CHECK (id),
    cutoff timestamptz NOT NULL,
    accounts_marked integer NOT NULL CHECK (accounts_marked >= 0),
    applied_at timestamptz NOT NULL DEFAULT clock_timestamp()
);

DO $$
DECLARE
    cut timestamptz;
    marked integer;
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'identity' AND table_name = 'users' AND column_name = 'email_verified_at'
    ) THEN
        RAISE EXCEPTION '008_email_verification_backfill requiere aplicar antes 007_email_verification.sql';
    END IF;

    LOCK TABLE identity.users IN SHARE ROW EXCLUSIVE MODE;

    IF EXISTS (SELECT 1 FROM identity.email_verification_backfill) THEN
        RAISE NOTICE 'email verification backfill: ya aplicado, sin cambios';
        RETURN;
    END IF;

    cut := clock_timestamp();
    -- Solo cuentas sin fecha: nunca sobrescribe una confirmación existente.
    UPDATE identity.users SET email_verified_at = cut WHERE email_verified_at IS NULL;
    GET DIAGNOSTICS marked = ROW_COUNT;
    INSERT INTO identity.email_verification_backfill (cutoff, accounts_marked) VALUES (cut, marked);
    RAISE NOTICE 'email verification backfill: % cuentas marcadas (corte %)', marked, cut;
END;
$$;
