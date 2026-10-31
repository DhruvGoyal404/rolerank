// Command ingest runs the fetch pipeline.
//
//	ingest boards <source> <file>   register board tokens, one per line
//	ingest status                   print progress
//	ingest run                      fetch until stopped; safe to stop at any moment
//	ingest dedupe                   group duplicate postings; safe to stop and rerun
//	ingest coverage                 print and record in-scope coverage per source
//
// Needs ROLERANK_CORPUS_URL and ROLERANK_VALKEY_URL. HIMALAYAS_API is optional.
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/valkey-io/valkey-go"

	"github.com/DhruvGoyal404/rolerank/worker/internal/fetch"
	"github.com/DhruvGoyal404/rolerank/worker/internal/ingest"
	"github.com/DhruvGoyal404/rolerank/worker/internal/limiter"
	"github.com/DhruvGoyal404/rolerank/worker/internal/source"
	"github.com/DhruvGoyal404/rolerank/worker/internal/useragent"
)

var version = "dev"

// Concurrent fetches per source. Requests stay spaced by the limiter, so this only stops
// one slow response from wasting the next slot. It can never raise the rate.
const workersPerSource = 2

func main() {
	if err := run(os.Args[1:]); err != nil {
		// Errors here name a stage, never a credential: URLs come from the environment
		// and aren't formatted into messages.
		fmt.Fprintln(os.Stderr, "ingest:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ingest boards|status|run")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config, err := pgxpool.ParseConfig(os.Getenv("ROLERANK_CORPUS_URL"))
	if err != nil {
		return fmt.Errorf("ROLERANK_CORPUS_URL is missing or invalid")
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("connect to corpus failed")
	}
	defer pool.Close()
	store := ingest.NewStore(pool)
	adapters := source.All()
	if err := store.EnsureSources(ctx, adapters); err != nil {
		return err
	}
	// Feeds have no boards of their own, so each is one board.
	for _, feed := range []string{"himalayas", "remoteok"} {
		if _, err := store.AddBoard(ctx, feed, "feed"); err != nil {
			return err
		}
	}

	switch args[0] {
	case "boards":
		if len(args) != 3 {
			return fmt.Errorf("usage: ingest boards <source> <file>")
		}
		return addBoards(ctx, store, args[1], args[2])
	case "status":
		return status(ctx, pool)
	case "dedupe":
		done, merged, err := store.Dedupe(ctx)
		fmt.Printf("dedupe: %d postings placed, %d merged into an existing job\n", done, merged)
		return err
	case "coverage":
		list, err := store.Coverage(ctx)
		if err != nil {
			return err
		}
		ingest.WriteCoverage(os.Stdout, list)
		return nil
	case "run":
		return fetchUntilStopped(ctx, store, adapters)
	}
	return fmt.Errorf("unknown command %q", args[0])
}

func addBoards(ctx context.Context, store *ingest.Store, sourceName, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	added, known := 0, 0
	lines := bufio.NewScanner(file)
	for lines.Scan() {
		token := strings.TrimSpace(lines.Text())
		if token == "" || strings.HasPrefix(token, "#") {
			continue
		}
		fresh, err := store.AddBoard(ctx, sourceName, token)
		if err != nil {
			return err
		}
		if fresh {
			added++
		} else {
			known++
		}
	}
	fmt.Printf("%s: %d added, %d already known\n", sourceName, added, known)
	return lines.Err()
}

func status(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `
		SELECT s.name,
		  count(b.id) FILTER (WHERE b.status = 'active'),
		  count(b.id) FILTER (WHERE b.status = 'active' AND b.last_checked_at IS NOT NULL),
		  count(b.id) FILTER (WHERE b.status = 'active' AND b.next_due_at <= now()),
		  count(b.id) FILTER (WHERE b.status = 'retired'),
		  (SELECT count(*) FROM postings p WHERE p.source_id = s.id AND p.closed_at IS NULL)
		FROM sources s LEFT JOIN boards b ON b.source_id = s.id
		GROUP BY s.id, s.name ORDER BY s.name`)
	if err != nil {
		return err
	}
	defer rows.Close()
	fmt.Printf("%-12s %8s %8s %8s %8s %10s\n", "source", "boards", "checked", "due", "retired", "active")
	for rows.Next() {
		var name string
		var boards, checked, due, retired, active int
		if err := rows.Scan(&name, &boards, &checked, &due, &retired, &active); err != nil {
			return err
		}
		fmt.Printf("%-12s %8d %8d %8d %8d %10d\n", name, boards, checked, due, retired, active)
	}
	return rows.Err()
}

func fetchUntilStopped(ctx context.Context, store *ingest.Store, adapters []source.Adapter) error {
	options, err := valkey.ParseURL(os.Getenv("ROLERANK_VALKEY_URL"))
	if err != nil {
		return fmt.Errorf("ROLERANK_VALKEY_URL is missing or invalid")
	}
	client, err := valkey.NewClient(options)
	if err != nil {
		return fmt.Errorf("connect to valkey failed")
	}
	defer client.Close()

	var hosts []string
	for _, a := range adapters {
		hosts = append(hosts, a.Hosts()...)
		if err := ingest.Setup(ctx, client, a.Name()); err != nil {
			return err
		}
	}
	bearer := map[string]string{}
	if key := os.Getenv("HIMALAYAS_API"); key != "" {
		bearer["himalayas.app"] = key
	}
	fetcher := fetch.New(fetch.Options{
		UserAgent: useragent.String(version),
		Agent:     "RoleRankBot",
		Hosts:     hosts,
		Limiter:   limiter.New(client, "rolerank:"),
		Bearer:    bearer,
	})

	var wg sync.WaitGroup
	failures := make(chan error, len(adapters)*workersPerSource)
	for _, a := range adapters {
		for i := range workersPerSource {
			w := &ingest.Worker{
				Store: store, Fetch: fetcher, Valkey: client, Adapter: a,
				Consumer: a.Name() + "-" + strconv.Itoa(i),
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := w.Run(ctx); err != nil {
					failures <- err
				}
			}()
		}
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	fmt.Println("running; stop with Ctrl-C, and run again to continue where it stopped")
	for {
		n, err := ingest.Enqueue(ctx, store, client, 500)
		if err != nil && ctx.Err() == nil {
			return err
		}
		if n > 0 {
			fmt.Printf("%s queued %d boards\n", time.Now().Format(time.TimeOnly), n)
		}
		select {
		case <-ctx.Done():
			wg.Wait()
			fmt.Println("stopped; progress is saved")
			return nil
		case err := <-failures:
			return err
		case <-ticker.C:
		}
	}
}
