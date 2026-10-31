package ingest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DhruvGoyal404/rolerank/worker/internal/normalize"
	"github.com/DhruvGoyal404/rolerank/worker/internal/source"
)

// ErrStale means another worker took this board over, so this one must stop and change
// nothing. Without it, a superseded worker could finish a run on a late page and close
// jobs it never read.
var ErrStale = errors.New("ingest: board taken over by another worker")

// Scheduling. A fresh cycle runs every RefreshEvery while up.
const (
	RefreshEvery  = 6 * time.Hour
	requeueAfter  = 30 * time.Minute
	retireAfter   = 3
	holdThreshold = 10
)

// Store is every database write the workers make.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore wraps a pool connected as corpus_rw.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Board is one unit of fetching.
type Board struct {
	ID         int64
	SourceID   int16
	Source     string
	Token      string
	ETag       string
	NextURL    string
	NextDueAt  time.Time
	RunStarted bool
}

// EnsureSources records the approved sources and their attribution.
func (s *Store) EnsureSources(ctx context.Context, adapters []source.Adapter) error {
	for _, a := range adapters {
		attribution := a.Attribution()
		_, err := s.pool.Exec(ctx, `
			INSERT INTO sources (name, attribution)
			VALUES ($1, jsonb_build_object('credit', $2::text, 'follow_link', $3::boolean))
			ON CONFLICT (name) DO UPDATE SET attribution = EXCLUDED.attribution`,
			a.Name(), attribution.Credit, attribution.FollowLink)
		if err != nil {
			return fmt.Errorf("ensure source %s: %w", a.Name(), err)
		}
	}
	return nil
}

// AddBoard registers a board, doing nothing if it's already known.
func (s *Store) AddBoard(ctx context.Context, sourceName, token string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO boards (source_id, token)
		SELECT id, $2 FROM sources WHERE name = $1
		ON CONFLICT (source_id, token) DO NOTHING`, sourceName, token)
	if err != nil {
		return false, fmt.Errorf("add board: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// Due is a board ready to be queued.
type Due struct {
	ID     int64
	Source string
}

// ClaimDue marks due boards as queued and returns them. A board queued long ago with no
// result is claimed again, which covers a message lost with a stopped Valkey.
func (s *Store) ClaimDue(ctx context.Context, limit int) ([]Due, error) {
	rows, err := s.pool.Query(ctx, `
		WITH due AS (
		  SELECT id FROM boards
		  WHERE status = 'active' AND NOT denylisted AND next_due_at <= now()
		    AND (enqueued_at IS NULL OR enqueued_at < now() - make_interval(secs => $2))
		  ORDER BY next_due_at
		  LIMIT $1
		  FOR UPDATE SKIP LOCKED
		)
		UPDATE boards b SET enqueued_at = now()
		FROM due, sources s
		WHERE b.id = due.id AND s.id = b.source_id
		RETURNING b.id, s.name`, limit, requeueAfter.Seconds())
	if err != nil {
		return nil, fmt.Errorf("claim due: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Due, error) {
		var d Due
		err := row.Scan(&d.ID, &d.Source)
		return d, err
	})
}

// Acquire takes the lease on a board, ending any earlier holder's right to write.
func (s *Store) Acquire(ctx context.Context, id int64) (int, error) {
	var attempt int
	err := s.pool.QueryRow(ctx, `UPDATE boards SET attempt = attempt + 1 WHERE id = $1 RETURNING attempt`, id).Scan(&attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("acquire: %w", err)
	}
	return attempt, nil
}

// fence runs inside a transaction, and holds the row lock until it ends.
func fence(ctx context.Context, tx pgx.Tx, id int64, attempt int) error {
	var current int
	if err := tx.QueryRow(ctx, `SELECT attempt FROM boards WHERE id = $1 FOR UPDATE`, id).Scan(&current); err != nil {
		return fmt.Errorf("fence: %w", err)
	}
	if current != attempt {
		return ErrStale
	}
	return nil
}

// LoadBoard reads a board's current state. Returns nil if it's gone or retired.
func (s *Store) LoadBoard(ctx context.Context, id int64) (*Board, error) {
	b := Board{ID: id}
	var etag, next *string
	var started *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT b.source_id, s.name, b.token, b.etag, b.next_url, b.next_due_at, b.run_started_at
		FROM boards b JOIN sources s ON s.id = b.source_id
		WHERE b.id = $1 AND b.status = 'active' AND NOT b.denylisted`, id).
		Scan(&b.SourceID, &b.Source, &b.Token, &etag, &next, &b.NextDueAt, &started)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load board: %w", err)
	}
	if etag != nil {
		b.ETag = *etag
	}
	if next != nil {
		b.NextURL = *next
	}
	b.RunStarted = started != nil
	return &b, nil
}

func clean(s string) string {
	return strings.ToValidUTF8(strings.ReplaceAll(s, "\x00", ""), "")
}

// list keeps a nil slice from reaching a NOT NULL array column.
func list(values []string) []string {
	if values == nil {
		return []string{}
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, clean(v))
	}
	return out
}

func nullable(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

const upsertPosting = `
	INSERT INTO postings (
	  source_id, board_id, source_job_id, group_key, title, company, location_raw, countries,
	  remote_scope, level, employment_type, is_engineering, language, body, excerpt,
	  content_hash, url, apply_url, posted_at, source_updated_at, closed_at,
	  salary_min, salary_max, salary_currency, salary_period, tags)
	VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,
	  CASE WHEN $21::timestamptz < now() THEN now() END,
	  $22,$23,$24,$25,$26)
	ON CONFLICT (source_id, source_job_id) DO UPDATE SET
	  cluster_id = CASE WHEN postings.content_hash = EXCLUDED.content_hash THEN postings.cluster_id END,
	  board_id = EXCLUDED.board_id, group_key = EXCLUDED.group_key, title = EXCLUDED.title,
	  company = EXCLUDED.company, location_raw = EXCLUDED.location_raw,
	  countries = EXCLUDED.countries, remote_scope = EXCLUDED.remote_scope,
	  level = EXCLUDED.level, employment_type = EXCLUDED.employment_type,
	  is_engineering = EXCLUDED.is_engineering, language = EXCLUDED.language,
	  body = EXCLUDED.body, excerpt = EXCLUDED.excerpt, content_hash = EXCLUDED.content_hash,
	  url = EXCLUDED.url, apply_url = EXCLUDED.apply_url, posted_at = EXCLUDED.posted_at,
	  source_updated_at = EXCLUDED.source_updated_at, closed_at = EXCLUDED.closed_at,
	  salary_min = EXCLUDED.salary_min, salary_max = EXCLUDED.salary_max,
	  salary_currency = EXCLUDED.salary_currency, salary_period = EXCLUDED.salary_period,
	  tags = EXCLUDED.tags, last_seen_at = now()`

func (b *Board) queuePostings(batch *pgx.Batch, postings []source.Posting) {
	for _, p := range postings {
		p.Source = b.Source
		f := normalize.Apply(p)
		company := clean(p.Company)
		if company == "" {
			company = b.Token
		}
		var apply *string
		if p.ApplyURL != "" {
			apply = &p.ApplyURL
		}
		var minimum, maximum *float64
		if p.SalaryMin > 0 || p.SalaryMax > 0 {
			minimum, maximum = &p.SalaryMin, &p.SalaryMax
		}
		// Prefixed so two boards can never collide on a source's own ID.
		id := b.Token + ":" + p.SourceJobID
		batch.Queue(upsertPosting,
			b.SourceID, b.ID, clean(id), clean(p.GroupID), clean(p.Title), company, clean(p.Location),
			list(f.Countries), f.RemoteScope, f.Level, clean(p.EmploymentType), f.IsEngineering,
			clean(p.Language), f.Body, f.Excerpt, f.ContentHash, p.URL, apply,
			nullable(p.PostedAt), nullable(p.UpdatedAt), nullable(p.ExpiresAt),
			minimum, maximum, clean(p.SalaryCurrency), clean(p.SalaryPeriod), list(p.Tags))
	}
}

// SavePage commits one page of postings and the position after it, atomically. If
// next is empty the board's run is finished, and postings it didn't see are closed.
func (s *Store) SavePage(ctx context.Context, b *Board, attempt int, postings []source.Posting, next, etag string, status int) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fence(ctx, tx, b.ID, attempt); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE boards SET run_started_at = COALESCE(run_started_at, now()) WHERE id = $1`, b.ID); err != nil {
		return err
	}
	batch := &pgx.Batch{}
	b.queuePostings(batch, postings)
	results := tx.SendBatch(ctx, batch)
	for range postings {
		if _, err := results.Exec(); err != nil {
			results.Close()
			return fmt.Errorf("save posting: %w", err)
		}
	}
	if err := results.Close(); err != nil {
		return err
	}

	if next != "" {
		_, err := tx.Exec(ctx, `UPDATE boards SET next_url = $2, last_status = $3, last_checked_at = now(), fail_count = 0
			WHERE id = $1`, b.ID, next, status)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if err := finishRun(ctx, tx, b.ID, etag, status); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// finishRun closes what the run didn't see. A board that suddenly loses over half its
// jobs is held for one retry, because that looks more like a source fault than reality.
func finishRun(ctx context.Context, tx pgx.Tx, id int64, etag string, status int) error {
	var active, unseen, hold int
	err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE p.closed_at IS NULL),
		       count(*) FILTER (WHERE p.closed_at IS NULL AND p.last_seen_at < b.run_started_at),
		       b.hold_count
		FROM boards b LEFT JOIN postings p ON p.board_id = b.id
		WHERE b.id = $1 GROUP BY b.hold_count`, id).Scan(&active, &unseen, &hold)
	if err != nil {
		return fmt.Errorf("finish run: %w", err)
	}
	if active >= holdThreshold && unseen*2 > active && hold == 0 {
		_, err := tx.Exec(ctx, `
			UPDATE boards SET hold_count = 1, next_url = NULL, run_started_at = NULL, enqueued_at = NULL,
			  etag = NULL, last_status = $2, last_checked_at = now(), next_due_at = now() + interval '1 hour'
			WHERE id = $1`, id, status)
		return err
	}
	if unseen > 0 {
		_, err := tx.Exec(ctx, `
			UPDATE postings p SET closed_at = now() FROM boards b
			WHERE p.board_id = b.id AND b.id = $1 AND p.closed_at IS NULL AND p.last_seen_at < b.run_started_at`, id)
		if err != nil {
			return err
		}
	}
	return schedule(ctx, tx, id, etag, status)
}

func schedule(ctx context.Context, tx pgx.Tx, id int64, etag string, status int) error {
	_, err := tx.Exec(ctx, `
		UPDATE boards SET etag = NULLIF($2, ''), last_status = $3, last_checked_at = now(), fail_count = 0,
		  hold_count = 0, next_url = NULL, run_started_at = NULL, enqueued_at = NULL,
		  next_due_at = now() + make_interval(secs => $4 * (0.95 + random() * 0.1))
		WHERE id = $1`, id, etag, status, RefreshEvery.Seconds())
	return err
}

// NotModified finishes a board whose ETag still holds.
func (s *Store) NotModified(ctx context.Context, id int64, attempt int, etag string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fence(ctx, tx, id, attempt); err != nil {
		return err
	}
	if err := schedule(ctx, tx, id, etag, 304); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Defer retries a board later, keeping any saved position.
func (s *Store) Defer(ctx context.Context, id int64, attempt int, wait time.Duration, status int) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE boards SET enqueued_at = NULL, last_status = $3, last_checked_at = now(),
		  next_due_at = now() + make_interval(secs => $2)
		WHERE id = $1 AND attempt = $4`, id, wait.Seconds(), status, attempt)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrStale
	}
	return err
}

// Fail records a failed attempt. After retireAfter failures in a row a board is retired,
// and its jobs close. The fail count resets on any success.
func (s *Store) Fail(ctx context.Context, id int64, attempt int, status int) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fence(ctx, tx, id, attempt); err != nil {
		return err
	}
	var fails int
	err = tx.QueryRow(ctx, `
		UPDATE boards SET fail_count = fail_count + 1, enqueued_at = NULL, last_status = $2,
		  last_checked_at = now(), next_due_at = now() + make_interval(mins => 5 * power(4, fail_count)::int)
		WHERE id = $1 RETURNING fail_count`, id, status).Scan(&fails)
	if err != nil {
		return err
	}
	if fails >= retireAfter {
		if _, err := tx.Exec(ctx, `UPDATE boards SET status = 'retired' WHERE id = $1`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE postings SET closed_at = now() WHERE board_id = $1 AND closed_at IS NULL`, id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// LogFetch records one request. It's for reporting, so a failure here never stops a run.
func (s *Store) LogFetch(ctx context.Context, host string, board int64, status, bytes int, took time.Duration, notModified bool) {
	_, _ = s.pool.Exec(ctx, `
		INSERT INTO fetch_log (host, board_id, status, bytes, ms, not_modified) VALUES ($1,$2,$3,$4,$5,$6)`,
		host, board, status, bytes, took.Milliseconds(), notModified)
}
