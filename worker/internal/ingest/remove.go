package ingest

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// removePostings deletes every posting whose column equals value, with its search row, and
// leaves no cluster pointing at it. A shared cluster passes to a surviving member.
// shortcut: the survivor is the lowest open ID, even if an aggregator; dedupe hands the
// cluster to the next ATS copy it places, so upgrade only if that proves too slow.
func removePostings(ctx context.Context, tx pgx.Tx, column string, value int64) (int64, error) {
	if _, err := tx.Exec(ctx, `
		UPDATE clusters c SET canonical_posting_id = (
		  SELECT o.id FROM postings o
		  WHERE o.cluster_id = c.id AND o.`+column+` IS DISTINCT FROM $1::bigint
		  ORDER BY o.closed_at IS NOT NULL, o.id LIMIT 1)
		WHERE c.canonical_posting_id IN (SELECT id FROM postings WHERE `+column+` = $1::bigint)`, value); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE postings SET cluster_id = NULL WHERE `+column+` = $1::bigint`, value); err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM postings WHERE `+column+` = $1::bigint`, value)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM clusters WHERE canonical_posting_id IS NULL`); err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// Deny honours an employer's removal request: the board is never fetched again, and what we
// hold from it goes now. The row stays, so rediscovering the board can't bring it back.
func (s *Store) Deny(ctx context.Context, sourceName, token string) (int64, error) {
	var removed int64
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var board int64
		err := tx.QueryRow(ctx, `
			UPDATE boards b SET denylisted = true FROM sources s
			WHERE s.id = b.source_id AND s.name = $1 AND b.token = $2 RETURNING b.id`,
			sourceName, token).Scan(&board)
		if err != nil {
			return fmt.Errorf("deny %s/%s: %w", sourceName, token, err)
		}
		removed, err = removePostings(ctx, tx, "board_id", board)
		return err
	})
	return removed, err
}

// Purge removes a source we may no longer use: its postings, boards, the fetch history of
// its host, and the source row. Dated coverage totals stay: they hold counts, not postings.
func (s *Store) Purge(ctx context.Context, sourceName, host string) (int64, error) {
	var removed int64
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var id int16
		if err := tx.QueryRow(ctx, `SELECT id FROM sources WHERE name = $1`, sourceName).Scan(&id); err != nil {
			return fmt.Errorf("purge %s: %w", sourceName, err)
		}
		var err error
		if removed, err = removePostings(ctx, tx, "source_id", int64(id)); err != nil {
			return err
		}
		for _, q := range []struct {
			sql string
			arg any
		}{
			{`DELETE FROM boards WHERE source_id = $1`, id},
			{`DELETE FROM fetch_log WHERE host = $1`, host},
			{`DELETE FROM robots_cache WHERE host = $1`, host},
			{`DELETE FROM sources WHERE id = $1`, id},
		} {
			if _, err := tx.Exec(ctx, q.sql, q.arg); err != nil {
				return err
			}
		}
		return nil
	})
	return removed, err
}
