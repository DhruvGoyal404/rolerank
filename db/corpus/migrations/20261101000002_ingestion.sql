-- migrate:up
-- Boards are the unit of fetching. One row per (source, token).
CREATE TABLE boards (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_id smallint NOT NULL REFERENCES sources (id),
  token text NOT NULL,
  company_name text,
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'retired')),
  denylisted boolean NOT NULL DEFAULT false,
  etag text,
  last_status smallint,
  last_checked_at timestamptz,
  next_due_at timestamptz NOT NULL DEFAULT now(),
  fail_count integer NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (source_id, token)
);
CREATE INDEX boards_due_idx ON boards (next_due_at) WHERE status = 'active' AND NOT denylisted;

CREATE TABLE clusters (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  canonical_posting_id bigint,
  minhash bytea
);

CREATE TABLE postings (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_id smallint NOT NULL REFERENCES sources (id),
  board_id bigint REFERENCES boards (id),
  source_job_id text NOT NULL,
  cluster_id bigint REFERENCES clusters (id),
  title text NOT NULL,
  company text NOT NULL,
  location_raw text,
  countries text[] NOT NULL DEFAULT '{}',
  remote_scope text NOT NULL DEFAULT 'unknown'
    CHECK (remote_scope IN ('onsite', 'hybrid', 'remote_global', 'remote_restricted', 'unknown')),
  level text NOT NULL DEFAULT 'unknown'
    CHECK (level IN ('intern', 'entry', 'mid', 'senior', 'unknown')),
  employment_type text,
  is_engineering boolean NOT NULL DEFAULT false,
  language text,
  body text NOT NULL DEFAULT '' CHECK (octet_length(body) <= 20480),
  excerpt text NOT NULL DEFAULT '',
  content_hash bytea NOT NULL,
  url text NOT NULL CHECK (url LIKE 'https://%'),
  apply_url text CHECK (apply_url LIKE 'https://%'),
  posted_at timestamptz,
  source_updated_at timestamptz,
  first_seen_at timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  closed_at timestamptz,
  salary_min numeric,
  salary_max numeric,
  salary_currency text,
  salary_period text,
  skills_required text[] NOT NULL DEFAULT '{}',
  skills_nice text[] NOT NULL DEFAULT '{}',
  tsv tsvector,
  embed_model text,
  UNIQUE (source_id, source_job_id)
);
CREATE INDEX postings_board_idx ON postings (board_id) WHERE closed_at IS NULL;
CREATE INDEX postings_cluster_idx ON postings (cluster_id);
CREATE INDEX postings_tsv_idx ON postings USING gin (tsv);

-- Narrow copy of active, in-scope rows, so a scan doesn't drag text through memory.
CREATE TABLE search_postings (
  posting_id bigint PRIMARY KEY REFERENCES postings (id) ON DELETE CASCADE,
  level text NOT NULL,
  remote_scope text NOT NULL,
  countries text[] NOT NULL DEFAULT '{}',
  employment_type text,
  skills_required text[] NOT NULL DEFAULT '{}',
  embedding halfvec(384)
);
CREATE INDEX search_postings_filter_idx ON search_postings (level, remote_scope);
CREATE INDEX search_postings_countries_idx ON search_postings USING gin (countries);
CREATE INDEX search_postings_skills_idx ON search_postings USING gin (skills_required);
CREATE INDEX search_postings_hnsw_idx ON search_postings
  USING hnsw (embedding halfvec_cosine_ops) WITH (m = 16, ef_construction = 64);

CREATE TABLE skills (
  id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name text NOT NULL UNIQUE,
  aliases text[] NOT NULL DEFAULT '{}',
  category text
);

CREATE TABLE robots_cache (
  host text PRIMARY KEY,
  status smallint,
  body text,
  fetched_at timestamptz NOT NULL
);

CREATE TABLE fetch_log (
  host text NOT NULL,
  status smallint,
  bytes integer,
  ms integer,
  at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX fetch_log_at_idx ON fetch_log (at);

-- migrate:down
