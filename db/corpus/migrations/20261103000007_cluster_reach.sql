-- migrate:up
-- A job listed in several places is one cluster. Search and filters read this union of its
-- open members, so an India listing on any member keeps the job findable from India.
CREATE VIEW cluster_reach AS
SELECT p.cluster_id,
  COALESCE(array_agg(DISTINCT u.country) FILTER (WHERE u.country IS NOT NULL), '{}') AS countries,
  array_agg(DISTINCT p.remote_scope) AS remote_scopes,
  COALESCE(array_agg(DISTINCT p.location_raw) FILTER (WHERE p.location_raw <> ''), '{}') AS locations
FROM postings p
LEFT JOIN LATERAL unnest(p.countries) AS u(country) ON true
WHERE p.cluster_id IS NOT NULL AND p.closed_at IS NULL
GROUP BY p.cluster_id;

-- migrate:down
