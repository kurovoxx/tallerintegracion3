-- H-05: identificador público de usuario (12 caracteres Crockford Base32 en
-- minúsculas, 60 bits). Vive en identity.users y no en identity.profiles
-- porque no todos los usuarios tienen fila de perfil.
--
-- La migración es idempotente: init.sql ya crea la columna/funciones en una
-- base nueva y validate-migrations.sh aplica init.sql antes que esta carpeta.
-- Sobre una base ya inicializada, añade la columna, rellena los usuarios
-- existentes y deja el DEFAULT para los nuevos.

CREATE OR REPLACE FUNCTION identity.generate_public_id()
RETURNS text
LANGUAGE plpgsql
VOLATILE
AS $$
DECLARE
    alphabet CONSTANT text := '0123456789abcdefghjkmnpqrstvwxyz';
    raw bytea := gen_random_bytes(8);
    value bigint := 0;
    result text := '';
BEGIN
    -- 8 bytes aleatorios = 64 bits; se descartan los 4 bits bajos para usar
    -- exactamente 60 bits (12 símbolos de 5 bits).
    FOR i IN 0..6 LOOP
        value := (value << 8) | get_byte(raw, i);
    END LOOP;
    value := (value << 4) | (get_byte(raw, 7) >> 4);

    FOR i IN 0..11 LOOP
        result := result || substr(
            alphabet,
            (((value >> (55 - i * 5)) & 31) + 1)::int,
            1
        );
    END LOOP;
    RETURN result;
END;
$$;

-- Generador con reintento ante colisión. El DEFAULT de la columna lo usa para
-- que cualquier INSERT (incluido el registro desde Flutter) reciba su ID sin
-- que el cliente tenga que enviarlo. La UNIQUE sigue siendo la garantía final:
-- dos inserts concurrentes pueden colisionar y el repositorio reintenta.
CREATE OR REPLACE FUNCTION identity.generate_unique_public_id()
RETURNS text
LANGUAGE plpgsql
VOLATILE
AS $$
DECLARE
    candidate text;
BEGIN
    FOR attempt IN 1..10 LOOP
        candidate := identity.generate_public_id();
        IF NOT EXISTS (
            SELECT 1 FROM identity.users WHERE public_id = candidate
        ) THEN
            RETURN candidate;
        END IF;
    END LOOP;
    RAISE EXCEPTION 'no se pudo generar un public_id único tras 10 intentos';
END;
$$;

ALTER TABLE identity.users
    ADD COLUMN IF NOT EXISTS public_id text;

-- La UNIQUE permite varios NULL mientras se rellena la columna.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'users_public_id_key'
          AND conrelid = 'identity.users'::regclass
    ) THEN
        ALTER TABLE identity.users
            ADD CONSTRAINT users_public_id_key UNIQUE (public_id);
    END IF;
END;
$$;

-- Backfill de usuarios existentes (idempotente: solo filas NULL).
UPDATE identity.users
SET public_id = identity.generate_unique_public_id()
WHERE public_id IS NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'users_public_id_format'
          AND conrelid = 'identity.users'::regclass
    ) THEN
        ALTER TABLE identity.users
            ADD CONSTRAINT users_public_id_format
            CHECK (public_id ~ '^[0-9a-hj-km-np-tv-z]{12}$');
    END IF;
END;
$$;

ALTER TABLE identity.users
    ALTER COLUMN public_id SET DEFAULT identity.generate_unique_public_id();

ALTER TABLE identity.users
    ALTER COLUMN public_id SET NOT NULL;
