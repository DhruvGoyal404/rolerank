-- migrate:up
ALTER TABLE clusters
  ADD COLUMN dedupe_key text NOT NULL DEFAULT '',
  ADD COLUMN location_key text NOT NULL DEFAULT '';
CREATE INDEX clusters_key_idx ON clusters (dedupe_key);
CREATE INDEX postings_undeduped_idx ON postings (id)
  WHERE cluster_id IS NULL AND closed_at IS NULL AND is_engineering;

-- migrate:down
