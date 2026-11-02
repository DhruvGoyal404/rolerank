package ingest

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/DhruvGoyal404/rolerank/worker/internal/dedupe"
)

type undeduped struct {
	id                           int64
	source, company, title, body string
	location                     string
}

// Dedupe assigns in-scope postings to clusters until none are left. Each posting commits
// on its own, so it's safe to stop and rerun, and a finished posting is never redone. Run
// one at a time. A posting whose text changes loses its cluster and comes back here.
func (s *Store) Dedupe(ctx context.Context) (done, merged int, err error) {
	const batch = 1000
	for {
		rows, err := s.pool.Query(ctx, `
			SELECT p.id, src.name, p.company, p.title, p.body, COALESCE(p.location_raw, '')
			FROM postings p JOIN sources src ON src.id = p.source_id
			WHERE p.cluster_id IS NULL AND p.closed_at IS NULL AND p.is_engineering
			ORDER BY p.id LIMIT $1`, batch)
		if err != nil {
			return done, merged, fmt.Errorf("dedupe: %w", err)
		}
		list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (undeduped, error) {
			var u undeduped
			err := row.Scan(&u.id, &u.source, &u.company, &u.title, &u.body, &u.location)
			return u, err
		})
		if err != nil {
			return done, merged, fmt.Errorf("dedupe: %w", err)
		}
		if len(list) == 0 {
			return done, merged, nil
		}
		for _, u := range list {
			joined, err := s.place(ctx, u)
			if err != nil {
				return done, merged, err
			}
			done++
			if joined {
				merged++
			}
		}
	}
}

// place puts one posting in a matching cluster or starts its own. It returns true on a merge.
func (s *Store) place(ctx context.Context, u undeduped) (bool, error) {
	key := dedupe.Key(u.company, u.title, u.location)
	location := dedupe.Location(u.location)
	aggregator := dedupe.IsAggregator(u.source)
	signature := dedupe.Signature(u.body)
	me := dedupe.Posting{Location: location, Signature: signature, Facts: dedupe.Read(u.body)}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT c.id, c.location_key, c.minhash, src.name, p.body
		FROM clusters c
		JOIN postings p ON p.id = c.canonical_posting_id
		JOIN sources src ON src.id = p.source_id
		WHERE c.dedupe_key = $1`, key)
	if err != nil {
		return false, err
	}
	type cluster struct {
		id        int64
		location  string
		signature []byte
		source    string
		body      string
	}
	clusters, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (cluster, error) {
		var c cluster
		err := row.Scan(&c.id, &c.location, &c.signature, &c.source, &c.body)
		return c, err
	})
	if err != nil {
		return false, err
	}

	for _, c := range clusters {
		clusterAggregator := dedupe.IsAggregator(c.source)
		if !dedupe.SameJob(me, dedupe.Posting{Location: c.location, Signature: c.signature, Facts: dedupe.Read(c.body)}) {
			continue
		}
		if _, err := tx.Exec(ctx, `UPDATE postings SET cluster_id = $2 WHERE id = $1`, u.id, c.id); err != nil {
			return false, err
		}
		// Prefer the employer's own ATS over a re-listing.
		if clusterAggregator && !aggregator {
			if _, err := tx.Exec(ctx, `UPDATE clusters SET canonical_posting_id = $2, location_key = $3 WHERE id = $1`,
				c.id, u.id, location); err != nil {
				return false, err
			}
		}
		return true, tx.Commit(ctx)
	}

	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO clusters (canonical_posting_id, minhash, dedupe_key, location_key)
		VALUES ($1, $2, $3, $4) RETURNING id`, u.id, signature, key, location).Scan(&id)
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE postings SET cluster_id = $2 WHERE id = $1`, u.id, id); err != nil {
		return false, err
	}
	return false, tx.Commit(ctx)
}
