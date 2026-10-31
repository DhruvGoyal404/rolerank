-- migrate:up
CREATE TABLE invites (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code_sha256 bytea NOT NULL UNIQUE CHECK (octet_length(code_sha256) = 32),
  uses_left integer NOT NULL CHECK (uses_left >= 0),
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  age_confirmed_at timestamptz NOT NULL,
  invite_id uuid REFERENCES invites (id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

-- Login keys on (provider, subject), never on email.
CREATE TABLE identities (
  provider text NOT NULL CHECK (provider IN ('google', 'github')),
  subject text NOT NULL,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  email text,
  email_verified boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (provider, subject)
);
CREATE INDEX identities_user_id_idx ON identities (user_id);

-- The cookie holds the token. Only its SHA-256 is stored.
CREATE TABLE sessions (
  token_sha256 bytea PRIMARY KEY CHECK (octet_length(token_sha256) = 32),
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL
);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

-- No foreign key: the record of a deletion outlives the user. Holds no content.
CREATE TABLE audit_log (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id uuid,
  event text NOT NULL,
  at timestamptz NOT NULL DEFAULT now()
);

-- Re-applied after any restore, so a deleted user can't come back.
CREATE TABLE deletion_tombstones (
  user_id uuid PRIMARY KEY,
  deleted_at timestamptz NOT NULL DEFAULT now()
);

-- migrate:down
