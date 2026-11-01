CREATE TABLE IF NOT EXISTS identity.password_resets (
    user_id uuid PRIMARY KEY REFERENCES identity.users(id) ON DELETE CASCADE,
    code_hash varchar(255) NOT NULL,
    expires_at timestamptz NOT NULL,
    requested_at timestamptz NOT NULL DEFAULT now(),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 5)
);
