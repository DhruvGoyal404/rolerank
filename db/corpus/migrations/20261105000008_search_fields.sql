-- migrate:up
-- What the ranker reads besides required skills: nice-to-have skills, title terms, the posting
-- date for freshness, and the union of the cluster's remote scopes. embed_hash marks the text
-- an embedding was made from, so a changed posting is re-embedded and an unchanged one isn't.
ALTER TABLE search_postings
  ADD COLUMN skills_nice text[] NOT NULL DEFAULT '{}',
  ADD COLUMN title_terms text[] NOT NULL DEFAULT '{}',
  ADD COLUMN remote_scopes text[] NOT NULL DEFAULT '{}',
  ADD COLUMN posted_at timestamptz,
  ADD COLUMN embed_model text,
  ADD COLUMN embed_hash bytea;
CREATE INDEX search_postings_title_idx ON search_postings USING gin (title_terms);
CREATE INDEX search_postings_scopes_idx ON search_postings USING gin (remote_scopes);

-- migrate:down
DROP INDEX search_postings_scopes_idx;
DROP INDEX search_postings_title_idx;
ALTER TABLE search_postings
  DROP COLUMN embed_hash,
  DROP COLUMN embed_model,
  DROP COLUMN posted_at,
  DROP COLUMN remote_scopes,
  DROP COLUMN title_terms,
  DROP COLUMN skills_nice;
