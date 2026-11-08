-- migrate:up
-- P6: the simple tracker. A user saves a job and sets its status. The title, company and link
-- are copied from the public posting so the tracker still reads after the posting closes; they
-- are public job data, not resume content. Rows go with the user (ON DELETE CASCADE).
CREATE TABLE tracker_items (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  posting_id bigint NOT NULL,
  title text NOT NULL CHECK (char_length(title) <= 400),
  company text NOT NULL CHECK (char_length(company) <= 200),
  url text NOT NULL CHECK (url LIKE 'https://%'),
  status text NOT NULL DEFAULT 'saved' CHECK (status IN ('saved', 'applied', 'interviewing', 'offer', 'rejected')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_id, posting_id)
);
CREATE INDEX tracker_items_user_idx ON tracker_items (user_id, updated_at DESC);

-- migrate:down
DROP TABLE tracker_items;
