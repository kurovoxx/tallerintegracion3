-- Existing bcrypt hashes cannot be converted without the raw refresh token.
-- New sessions will populate this nullable digest; active legacy rows continue
-- through the bounded bcrypt fallback until they rotate or expire.
ALTER TABLE identity.refresh_tokens
    ADD COLUMN IF NOT EXISTS token_fingerprint BYTEA;
