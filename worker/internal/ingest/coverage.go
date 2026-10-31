package ingest

import (
	"context"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5"
)

// SourceCoverage is one source's share of what users can actually see. Counts are of
// canonical postings, so a job listed on two sources counts once, for its employer's ATS.
type SourceCoverage struct {
	Source           string
	Postings         int
	InScope          int
	IndiaEngineering int
	IndiaEarly       int
	IndiaIntern      int
	RemoteOpen       int
	LevelUnknown     int
	BareRemote       int
}

// Coverage measures the corpus and records a dated snapshot.
func (s *Store) Coverage(ctx context.Context) ([]SourceCoverage, error) {
	rows, err := s.pool.Query(ctx, `
		WITH visible AS (
		  SELECT p.*, 'IN' = ANY(p.countries) AS india
		  FROM postings p
		  WHERE p.closed_at IS NULL
		    AND (p.cluster_id IS NULL OR EXISTS (
		      SELECT 1 FROM clusters c WHERE c.id = p.cluster_id AND c.canonical_posting_id = p.id))
		), counted AS (
		  SELECT s.name, count(v.id) AS postings,
		    count(v.id) FILTER (WHERE v.is_engineering) AS in_scope,
		    count(v.id) FILTER (WHERE v.is_engineering AND v.india) AS india_engineering,
		    count(v.id) FILTER (WHERE v.is_engineering AND v.india AND v.level IN ('intern', 'entry')) AS india_early,
		    count(v.id) FILTER (WHERE v.is_engineering AND v.india AND v.level = 'intern') AS india_intern,
		    count(v.id) FILTER (WHERE v.is_engineering AND (v.remote_scope = 'remote_global'
		      OR (v.remote_scope = 'remote_restricted' AND v.india))) AS remote_open,
		    count(v.id) FILTER (WHERE v.is_engineering AND v.level = 'unknown') AS level_unknown,
		    count(v.id) FILTER (WHERE v.is_engineering AND v.remote_scope = 'unknown'
		      AND v.location_raw ~* 'remote') AS bare_remote
		  FROM sources s LEFT JOIN visible v ON v.source_id = s.id
		  GROUP BY s.name
		)
		SELECT * FROM counted ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("coverage: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (SourceCoverage, error) {
		var c SourceCoverage
		err := row.Scan(&c.Source, &c.Postings, &c.InScope, &c.IndiaEngineering, &c.IndiaEarly,
			&c.IndiaIntern, &c.RemoteOpen, &c.LevelUnknown, &c.BareRemote)
		return c, err
	})
	if err != nil {
		return nil, fmt.Errorf("coverage: %w", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var at any
	if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&at); err != nil {
		return nil, err
	}
	for _, c := range list {
		_, err := tx.Exec(ctx, `INSERT INTO coverage_snapshots VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			at, c.Source, c.Postings, c.InScope, c.IndiaEngineering, c.IndiaEarly, c.IndiaIntern,
			c.RemoteOpen, c.LevelUnknown, c.BareRemote)
		if err != nil {
			return nil, fmt.Errorf("coverage snapshot: %w", err)
		}
	}
	return list, tx.Commit(ctx)
}

func share(part, whole int) string {
	if whole == 0 {
		return "-"
	}
	return fmt.Sprintf("%d%%", part*100/whole)
}

// WriteCoverage prints counts with each source's share, which is where concentration shows.
func WriteCoverage(out io.Writer, list []SourceCoverage) {
	var total SourceCoverage
	for _, c := range list {
		total.Postings += c.Postings
		total.InScope += c.InScope
		total.IndiaEngineering += c.IndiaEngineering
		total.IndiaEarly += c.IndiaEarly
		total.IndiaIntern += c.IndiaIntern
		total.RemoteOpen += c.RemoteOpen
		total.LevelUnknown += c.LevelUnknown
		total.BareRemote += c.BareRemote
	}
	fmt.Fprintf(out, "%-11s %8s %9s %11s %12s %7s %12s %8s %6s\n",
		"source", "postings", "in scope", "India eng", "India early", "intern", "remote open", "lvl ?", "rem ?")
	cell := func(n, whole int) string { return fmt.Sprintf("%d (%s)", n, share(n, whole)) }
	for _, c := range list {
		fmt.Fprintf(out, "%-11s %8d %9s %11s %12s %7d %12s %8d %6d\n", c.Source, c.Postings,
			cell(c.InScope, total.InScope), cell(c.IndiaEngineering, total.IndiaEngineering),
			cell(c.IndiaEarly, total.IndiaEarly), c.IndiaIntern, cell(c.RemoteOpen, total.RemoteOpen),
			c.LevelUnknown, c.BareRemote)
	}
	fmt.Fprintf(out, "%-11s %8d %9d %11d %12d %7d %12d %8d %6d\n", "total", total.Postings, total.InScope,
		total.IndiaEngineering, total.IndiaEarly, total.IndiaIntern, total.RemoteOpen, total.LevelUnknown, total.BareRemote)
}
