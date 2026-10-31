-- migrate:up
-- Public job data only. PII never enters this database.
CREATE TABLE sources (
  id smallint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name text NOT NULL UNIQUE,
  attribution jsonb NOT NULL DEFAULT '{}',
  enabled boolean NOT NULL DEFAULT true
);

-- migrate:down
