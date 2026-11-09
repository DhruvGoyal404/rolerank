-- migrate:up
-- The ledger scope of a call that belongs to no user: 'ops' for rolerank-up.sh's model check.
-- NULL for user calls, whose scope is user_id. Expand only: the previous release ignores it.
ALTER TABLE llm_calls ADD COLUMN scope text;

-- migrate:down
ALTER TABLE llm_calls DROP COLUMN scope;
