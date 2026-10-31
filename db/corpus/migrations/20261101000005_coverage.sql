-- migrate:up
-- One row per source per measurement, so a growing reliance on one source shows as a
-- trend. The admin metrics page reads this table.
CREATE TABLE coverage_snapshots (
  taken_at timestamptz NOT NULL,
  source text NOT NULL,
  postings integer NOT NULL,
  in_scope integer NOT NULL,
  india_engineering integer NOT NULL,
  india_early integer NOT NULL,
  india_intern integer NOT NULL,
  remote_open integer NOT NULL,
  level_unknown integer NOT NULL,
  bare_remote integer NOT NULL,
  PRIMARY KEY (taken_at, source)
);

-- migrate:down
