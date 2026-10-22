-- Keep refresh writes available while PostgreSQL builds the lookup index.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_refresh_tokens_fingerprint
    ON identity.refresh_tokens(token_fingerprint)
    WHERE token_fingerprint IS NOT NULL;
