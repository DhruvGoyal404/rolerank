package ingest

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/DhruvGoyal404/rolerank/worker/internal/normalize"
	"github.com/DhruvGoyal404/rolerank/worker/internal/source"
)

// Reclassify reapplies the classifiers to stored postings, because a board answering 304
// never reaches them. Only a blank Himalayas location can change scope from stored fields,
// and Himalayas keeps its level, which rests on a seniority field that isn't stored.
func (s *Store) Reclassify(ctx context.Context) (int, error) {
	type row struct {
		id                  int64
		title, body, source string
		scope, level, kind  string
		engineering         bool
		blank               bool
	}
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.title, p.body, src.name, p.remote_scope, p.level, COALESCE(p.employment_type, ''), p.is_engineering,
		  COALESCE(p.location_raw, '') = '' AND cardinality(p.countries) = 0
		FROM postings p JOIN sources src ON src.id = p.source_id`)
	if err != nil {
		return 0, fmt.Errorf("reclassify: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
		var x row
		err := r.Scan(&x.id, &x.title, &x.body, &x.source, &x.scope, &x.level, &x.kind, &x.engineering, &x.blank)
		return x, err
	})
	if err != nil {
		return 0, fmt.Errorf("reclassify: %w", err)
	}
	changed := 0
	for _, x := range list {
		engineering := normalize.IsEngineering(x.title, x.body)
		scope := x.scope
		if x.source == "himalayas" && x.blank {
			scope, _ = normalize.Geography(source.Posting{Source: x.source, Remote: source.RemoteYes, Body: x.body})
		}
		level := x.level
		if x.source != "himalayas" {
			level = normalize.Level(x.title, x.body, x.kind, nil)
		}
		if engineering == x.engineering && scope == x.scope && level == x.level {
			continue
		}
		if _, err := s.pool.Exec(ctx, `UPDATE postings SET is_engineering = $2, remote_scope = $3, level = $4 WHERE id = $1`,
			x.id, engineering, scope, level); err != nil {
			return changed, fmt.Errorf("reclassify: %w", err)
		}
		changed++
	}
	return changed, nil
}
