// Package ingest runs the fetch pipeline. Postgres is the record of progress: each page's
// postings, the position after it and the ETag commit in one transaction, and the queue
// message is acknowledged only after that. Stopping at any instant loses at most the page
// in flight, and repeating a page is harmless because writes are upserts.
package ingest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/valkey-io/valkey-go"

	"github.com/DhruvGoyal404/rolerank/worker/internal/fetch"
	"github.com/DhruvGoyal404/rolerank/worker/internal/source"
)

const (
	group       = "ingest"
	reclaimIdle = 5 * time.Minute
	readBlock   = 2 * time.Second
	field       = "board"
	retryDelay  = 15 * time.Second
)

// Stream names the queue for a source. One per source, so a slow host can't hold up the
// others.
func Stream(sourceName string) string {
	return "ingest:fetch:" + sourceName
}

// Worker consumes one source's stream.
type Worker struct {
	Store    *Store
	Fetch    *fetch.Client
	Valkey   valkey.Client
	Adapter  source.Adapter
	Consumer string
	Idle     time.Duration // reclaim threshold, for tests
	current  string        // the stream entry in hand
	OnPage   func()        // called after each committed page, for tests
}

// Setup creates a stream and its consumer group if needed.
func Setup(ctx context.Context, client valkey.Client, sourceName string) error {
	err := client.Do(ctx, client.B().XgroupCreate().Key(Stream(sourceName)).Group(group).Id("0").Mkstream().Build()).Error()
	if err != nil && !valkey.IsValkeyBusyGroup(err) {
		return fmt.Errorf("setup %s: %w", sourceName, err)
	}
	return nil
}

// Enqueue moves due boards onto their streams. A board claimed but not queued because of a
// crash here is claimed again after requeueAfter.
func Enqueue(ctx context.Context, store *Store, client valkey.Client, limit int) (int, error) {
	due, err := store.ClaimDue(ctx, limit)
	if err != nil {
		return 0, err
	}
	for _, d := range due {
		cmd := client.B().Xadd().Key(Stream(d.Source)).Id("*").FieldValue().FieldValue(field, strconv.FormatInt(d.ID, 10)).Build()
		if err := client.Do(ctx, cmd).Error(); err != nil {
			return 0, fmt.Errorf("enqueue: %w", err)
		}
	}
	return len(due), nil
}

// Run processes messages until ctx ends.
func (w *Worker) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		if err := w.Step(ctx); err != nil && ctx.Err() == nil {
			// A laptop waking from sleep, or a restarting container, lands here. Progress is
			// saved, so wait and carry on rather than exit.
			fmt.Fprintf(os.Stderr, "%s: will retry: %v\n", w.Consumer, err)
			select {
			case <-ctx.Done():
			case <-time.After(retryDelay):
			}
		}
	}
	return nil
}

// Step reclaims abandoned messages, then handles at most one new one.
func (w *Worker) Step(ctx context.Context) error {
	idle := w.Idle
	if idle == 0 {
		idle = reclaimIdle
	}
	claim := w.Valkey.B().Xautoclaim().Key(Stream(w.Adapter.Name())).Group(group).Consumer(w.Consumer).
		MinIdleTime(strconv.FormatInt(idle.Milliseconds(), 10)).Start("0").Count(1).Build()
	reply, err := w.Valkey.Do(ctx, claim).ToArray()
	if err != nil {
		return fmt.Errorf("reclaim: %w", err)
	}
	if len(reply) >= 2 {
		entries, err := reply[1].AsXRange()
		if err != nil {
			return fmt.Errorf("reclaim: %w", err)
		}
		if len(entries) > 0 {
			return w.handle(ctx, entries[0])
		}
	}
	read := w.Valkey.B().Xreadgroup().Group(group, w.Consumer).Count(1).Block(readBlock.Milliseconds()).
		Streams().Key(Stream(w.Adapter.Name())).Id(">").Build()
	streams, err := w.Valkey.Do(ctx, read).AsXRead()
	if valkey.IsValkeyNil(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	for _, entries := range streams {
		for _, entry := range entries {
			if err := w.handle(ctx, entry); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *Worker) handle(ctx context.Context, entry valkey.XRangeEntry) error {
	w.current = entry.ID
	id, err := strconv.ParseInt(entry.FieldValues[field], 10, 64)
	if err == nil {
		err = w.process(ctx, id)
	}
	if errors.Is(err, ErrStale) {
		// The new owner holds this message now.
		return nil
	}
	if err != nil {
		// Not acknowledged: another worker reclaims it.
		return err
	}
	return w.Valkey.Do(ctx, w.Valkey.B().Xack().Key(Stream(w.Adapter.Name())).Group(group).Id(entry.ID).Build()).Error()
}

// keepAlive tells the stream this worker is still busy, so a long run isn't reclaimed.
func (w *Worker) keepAlive(ctx context.Context) {
	if w.current == "" {
		return
	}
	cmd := w.Valkey.B().Xclaim().Key(Stream(w.Adapter.Name())).Group(group).Consumer(w.Consumer).
		MinIdleTime("0").Id(w.current).Justid().Build()
	// Best effort: if this fails, the lease still keeps a takeover safe.
	_ = w.Valkey.Do(ctx, cmd).Error()
}

func (w *Worker) process(ctx context.Context, id int64) error {
	attempt, err := w.Store.Acquire(ctx, id)
	if err != nil {
		return err
	}
	board, err := w.Store.LoadBoard(ctx, id)
	if err != nil {
		return err
	}
	// Already finished by an earlier delivery: the crash was after the commit, before the ack.
	if board == nil || (board.NextDueAt.After(time.Now()) && board.NextURL == "" && !board.RunStarted) {
		return nil
	}

	target := board.NextURL
	if target == "" {
		target = w.Adapter.FirstURL(board.Token)
	}
	for {
		etag := ""
		if board.NextURL == "" && !board.RunStarted {
			etag = board.ETag
		}
		started := time.Now()
		res, err := w.Fetch.Get(ctx, target, etag)
		took := time.Since(started)
		host := hostOf(target)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			w.Store.LogFetch(ctx, host, id, 0, 0, took, false)
			return w.failed(ctx, id, attempt, err)
		}
		w.Store.LogFetch(ctx, host, id, res.Status, len(res.Body), took, res.NotModified)

		switch {
		case res.NotModified:
			return w.Store.NotModified(ctx, id, attempt, etag)
		case res.Status != http.StatusOK:
			return w.Store.Fail(ctx, id, attempt, res.Status)
		}
		page, err := w.Adapter.Parse(board.Token, res.Body)
		if err != nil {
			// A bad body proves nothing about the jobs, so nothing is closed.
			fmt.Fprintf(os.Stderr, "%s: board %d: unreadable response: %v\n", w.Consumer, id, err)
			return w.Store.Fail(ctx, id, attempt, res.Status)
		}
		// An ETag is only meaningful for a board that fits in one response.
		saveTag := res.ETag
		if page.NextURL != "" || board.RunStarted {
			saveTag = ""
		}
		if err := w.Store.SavePage(ctx, board, attempt, page.Postings, page.NextURL, saveTag, res.Status); err != nil {
			return err
		}
		w.keepAlive(ctx)
		if w.OnPage != nil {
			w.OnPage()
		}
		if page.NextURL == "" {
			return nil
		}
		target = page.NextURL
		board.NextURL = target
		board.RunStarted = true
	}
}

func (w *Worker) failed(ctx context.Context, id int64, attempt int, err error) error {
	// A failure must be visible. A silent one left the biggest boards out of a whole run.
	fmt.Fprintf(os.Stderr, "%s: board %d failed: %v\n", w.Consumer, id, err)
	switch {
	case errors.Is(err, fetch.ErrBackoff):
		// The fetcher already blocked the host for every worker.
		return w.Store.Defer(ctx, id, attempt, 15*time.Minute, http.StatusTooManyRequests)
	case errors.Is(err, fetch.ErrDisallowed):
		return w.Store.Defer(ctx, id, attempt, fetch.RobotsTTL, 0)
	case errors.Is(err, fetch.ErrRobotsUnavailable):
		return w.Store.Defer(ctx, id, attempt, 10*time.Minute, 0)
	default:
		return w.Store.Fail(ctx, id, attempt, 0)
	}
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}
