package ingest

import (
	"context"
	"crypto/sha256"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/DhruvGoyal404/rolerank/worker/internal/skills"
)

// Enrich extracts skills for every visible in-scope posting and keeps search_postings in step:
// one row per job, carrying the cluster's union of countries and remote scopes. A row whose
// text changed loses its embedding, so the embed step redoes only those. Rows for jobs that
// closed or left scope are deleted.
func (s *Store) Enrich(ctx context.Context) (upserted, removed int, err error) {
	rows, err := s.pool.Query(ctx, `
		WITH visible AS (
		  SELECT DISTINCT ON (COALESCE(p.cluster_id, -p.id)) p.id, p.title, p.body, p.level, p.remote_scope,
		    p.employment_type, p.posted_at, COALESCE(r.countries, p.countries) AS countries,
		    COALESCE(r.remote_scopes, ARRAY[p.remote_scope]) AS scopes, p.is_engineering
		  FROM postings p
		  LEFT JOIN clusters c ON c.id = p.cluster_id
		  LEFT JOIN cluster_reach r ON r.cluster_id = p.cluster_id
		  WHERE p.closed_at IS NULL
		  ORDER BY COALESCE(p.cluster_id, -p.id), c.canonical_posting_id = p.id DESC NULLS LAST, p.id
		)
		SELECT v.id, v.title, v.body, v.level, v.remote_scope, v.employment_type, v.posted_at, v.countries, v.scopes,
		  sp.embed_hash
		FROM visible v LEFT JOIN search_postings sp ON sp.posting_id = v.id
		WHERE v.is_engineering`)
	if err != nil {
		return 0, 0, fmt.Errorf("enrich: %w", err)
	}
	type row struct {
		id                        int64
		title, body, level, scope string
		kind                      *string
		posted                    *time.Time
		countries, scopes         []string
		hash                      []byte
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
		var x row
		err := r.Scan(&x.id, &x.title, &x.body, &x.level, &x.scope, &x.kind, &x.posted, &x.countries, &x.scopes, &x.hash)
		return x, err
	})
	if err != nil {
		return 0, 0, fmt.Errorf("enrich: %w", err)
	}
	keep := make([]int64, 0, len(list))
	batch := &pgx.Batch{}
	flush := func() error {
		if batch.Len() == 0 {
			return nil
		}
		err := s.pool.SendBatch(ctx, batch).Close()
		batch = &pgx.Batch{}
		return err
	}
	for _, x := range list {
		keep = append(keep, x.id)
		req, nice := skills.Default.Extract(x.body)
		for _, t := range skills.Default.Find(x.title) {
			if !slices.Contains(req, t) {
				req = append(req, t)
			}
		}
		nice = slices.DeleteFunc(nice, func(n string) bool { return slices.Contains(req, n) })
		slices.Sort(req)
		req, nice = append([]string{}, req...), append([]string{}, nice...) // NOT NULL columns: never send nil
		sum := sha256.Sum256([]byte(x.title + "\n" + x.body))
		batch.Queue(`UPDATE postings SET skills_required = $2, skills_nice = $3 WHERE id = $1`, x.id, req, nice)
		batch.Queue(`
			INSERT INTO search_postings (posting_id, level, remote_scope, countries, employment_type, skills_required,
			  skills_nice, title_terms, remote_scopes, posted_at, embed_hash)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (posting_id) DO UPDATE SET level = $2, remote_scope = $3, countries = $4, employment_type = $5,
			  skills_required = $6, skills_nice = $7, title_terms = $8, remote_scopes = $9, posted_at = $10, embed_hash = $11,
			  embedding = CASE WHEN search_postings.embed_hash = $11 THEN search_postings.embedding END,
			  embed_model = CASE WHEN search_postings.embed_hash = $11 THEN search_postings.embed_model END`,
			x.id, x.level, x.scope, x.countries, x.kind, req, nice, append([]string{}, skills.TitleTerms(x.title)...), x.scopes, x.posted, sum[:])
		if batch.Len() >= 1000 {
			if err := flush(); err != nil {
				return upserted, 0, fmt.Errorf("enrich: %w", err)
			}
		}
		upserted++
	}
	if err := flush(); err != nil {
		return upserted, 0, fmt.Errorf("enrich: %w", err)
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM search_postings WHERE NOT (posting_id = ANY($1))`, keep)
	if err != nil {
		return upserted, 0, fmt.Errorf("enrich: %w", err)
	}
	return upserted, int(tag.RowsAffected()), nil
}
