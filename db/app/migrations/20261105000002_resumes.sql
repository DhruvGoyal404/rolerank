-- migrate:up
-- Resumes, profiles, per-user keys and the LLM spend ledger (P4).
--
-- Resume text, the parsed profile and the stripped contact details are encrypted in the
-- application with a per-user key before they reach a disk (CLAUDE.md, privacy invariants).
-- The uploaded file waits in file_ct, encrypted, only until extraction; then it is set to
-- NULL (Q10).

-- Each user's data key, wrapped by the key-encryption key held outside the database.
CREATE TABLE user_keys (
  user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
  wrapped bytea NOT NULL,
  kek_version integer NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE resumes (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  status text NOT NULL CHECK (status IN ('queued', 'extracting', 'parsing', 'ready', 'failed')),
  error text, -- a fixed code, never resume content
  kind text NOT NULL CHECK (kind IN ('pdf', 'docx')),
  file_ct bytea,
  lines_ct bytea,
  contacts_ct bytea,
  hidden_lines integer NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX resumes_user_idx ON resumes (user_id);

-- A profile version is a draft until the user confirms it. Skills and level are in the clear
-- because matching queries them; the database disk is encrypted as well.
CREATE TABLE profiles (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  resume_id uuid REFERENCES resumes (id) ON DELETE SET NULL,
  version integer NOT NULL,
  data_ct bytea NOT NULL,
  skills text[] NOT NULL DEFAULT '{}',
  level text,
  embedding real[],
  embed_model text,
  confirmed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_id, version)
);

-- Spend per UTC day: one 'global' row and one row per user (scope = the user's id).
-- A call reserves its worst case first and settles to its real cost after.
CREATE TABLE llm_ledger (
  day date NOT NULL,
  scope text NOT NULL,
  reserved_usd numeric(12, 6) NOT NULL DEFAULT 0 CHECK (reserved_usd >= 0),
  spent_usd numeric(12, 6) NOT NULL DEFAULT 0 CHECK (spent_usd >= 0),
  PRIMARY KEY (day, scope)
);

-- One row per LLM call. Never the prompt, the resume or the output: only what the call cost.
CREATE TABLE llm_calls (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  day date NOT NULL,
  user_id uuid REFERENCES users (id) ON DELETE SET NULL,
  task text NOT NULL,
  model text NOT NULL,
  prompt_version text NOT NULL,
  reserved_usd numeric(12, 6) NOT NULL,
  cost_usd numeric(12, 6),
  input_tokens integer,
  output_tokens integer,
  latency_ms integer,
  outcome text NOT NULL DEFAULT 'reserved',
  input_hash bytea,
  created_at timestamptz NOT NULL DEFAULT now(),
  settled_at timestamptz
);
CREATE INDEX llm_calls_open_idx ON llm_calls (created_at) WHERE settled_at IS NULL;

-- migrate:down
DROP TABLE llm_calls;
DROP TABLE llm_ledger;
DROP TABLE profiles;
DROP TABLE resumes;
DROP TABLE user_keys;
