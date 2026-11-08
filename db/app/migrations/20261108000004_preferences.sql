-- migrate:up
-- P6: what the user wants from matches. Short public facts about the search, not resume content.
-- Rows go with the user (ON DELETE CASCADE).
CREATE TABLE user_preferences (
  user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
  country text NOT NULL DEFAULT 'IN' CHECK (country ~ '^[A-Z]{2}$'),
  remote boolean NOT NULL DEFAULT true,
  target_titles text NOT NULL DEFAULT '' CHECK (char_length(target_titles) <= 200),
  updated_at timestamptz NOT NULL DEFAULT now()
);

-- migrate:down
DROP TABLE user_preferences;
