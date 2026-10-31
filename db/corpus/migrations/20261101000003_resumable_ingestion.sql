-- migrate:up
-- Progress lives on the board row, committed with each page of postings, so a run can
-- stop at any instant and continue exactly where it was.
ALTER TABLE boards
  ADD COLUMN enqueued_at timestamptz,
  ADD COLUMN next_url text,
  ADD COLUMN run_started_at timestamptz,
  ADD COLUMN hold_count integer NOT NULL DEFAULT 0,
  -- The lease: a worker's writes count only while this is still its number.
  ADD COLUMN attempt integer NOT NULL DEFAULT 0;

ALTER TABLE postings
  ADD COLUMN group_key text,
  ADD COLUMN tags text[] NOT NULL DEFAULT '{}';

ALTER TABLE fetch_log
  ADD COLUMN board_id bigint,
  ADD COLUMN not_modified boolean NOT NULL DEFAULT false;

-- migrate:down
