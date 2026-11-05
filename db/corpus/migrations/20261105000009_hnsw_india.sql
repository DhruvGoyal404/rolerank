-- migrate:up
-- A partial HNSW index over jobs open to India, so a filtered vector search walks only rows it
-- can return. On the 5 lakh bench (2026-11-05) the whole-table index gave filtered recall@20 of
-- 0.759 and p95 937 ms at 20 users; this one gives 0.961 and 188 ms. The predicate must match
-- rank.Eligible's text exactly for the planner to use it.
CREATE INDEX search_postings_hnsw_in_idx ON search_postings
  USING hnsw (embedding halfvec_cosine_ops) WITH (m = 16, ef_construction = 64)
  WHERE 'IN' = ANY(countries) OR 'remote_global' = ANY(remote_scopes);

-- migrate:down
DROP INDEX search_postings_hnsw_in_idx;
