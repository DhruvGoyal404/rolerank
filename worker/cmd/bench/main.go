// Command bench measures search latency on a synthetic corpus (ARCHITECTURE.md, "Bench harness").
//
//	bench load <rows>   creates rolerank_bench_test with every migration and <rows> synthetic jobs
//	bench run           20 concurrent users run hybrid searches while a writer updates rows;
//	                    prints p50/p95/p99, a histogram, and EXPLAIN ANALYZE checks of the timer
//
// The corpus copies the real one's shares (level, India-open, remote) measured on 2026-11-05.
// Vectors are clustered around 64 centres, as real embeddings are, not uniform on the sphere.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DhruvGoyal404/rolerank/worker/internal/embed"
	"github.com/DhruvGoyal404/rolerank/worker/internal/rank"
	"github.com/DhruvGoyal404/rolerank/worker/internal/skills"
)

const dbName = "rolerank_bench_test" // ends in _test: never the dev corpus

func main() {
	users := flag.Int("users", 20, "concurrent users")
	queries := flag.Int("queries", 2000, "searches in total")
	writes := flag.Int("writes", 50, "row updates a second during the run (ingestion load)")
	flag.Parse()
	var err error
	switch flag.Arg(0) {
	case "load":
		var n int
		_, err = fmt.Sscan(flag.Arg(1), &n)
		if err == nil {
			err = load(context.Background(), n)
		}
	case "run":
		err = run(context.Background(), *users, *queries, *writes)
	default:
		err = errors.New("usage: bench load <rows> | bench run")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "bench:", err)
		os.Exit(1)
	}
}

func dbURL() (string, error) {
	u, err := url.Parse(os.Getenv("ROLERANK_TEST_ADMIN_URL"))
	if err != nil || u.Host == "" {
		return "", errors.New("ROLERANK_TEST_ADMIN_URL is required")
	}
	u.Path = "/" + dbName
	return u.String(), nil
}

// Shares of the real in-scope corpus.
var levelShare = []struct {
	level string
	p     float64
}{{"senior", 0.657}, {"mid", 0.156}, {"unknown", 0.126}, {"entry", 0.038}, {"intern", 0.024}}

const (
	centres     = 64
	indiaShare  = 0.17
	remoteShare = 0.006
)

func centre(r *rand.Rand) [][]float32 {
	cs := make([][]float32, centres)
	for i := range cs {
		cs[i] = unit(r, nil, 1)
	}
	return cs
}

// unit returns a unit vector near base (or uniform when base is nil), spread by noise.
func unit(r *rand.Rand, base []float32, noise float64) []float32 {
	v := make([]float32, embed.Dims)
	var sum float64
	for i := range v {
		x := r.NormFloat64() * noise
		if base != nil {
			x += float64(base[i])
		}
		v[i] = float32(x)
		sum += x * x
	}
	for i := range v {
		v[i] = float32(float64(v[i]) / math.Sqrt(sum))
	}
	return v
}

func load(ctx context.Context, n int) error {
	admin := os.Getenv("ROLERANK_TEST_ADMIN_URL")
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+dbName+" WITH (FORCE)"); err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+dbName); err != nil {
		return err
	}
	_ = conn.Close(ctx)
	target, err := dbURL()
	if err != nil {
		return err
	}
	db, err := pgx.Connect(ctx, target)
	if err != nil {
		return err
	}
	defer db.Close(ctx)
	if _, err := db.Exec(ctx, "CREATE EXTENSION vector"); err != nil {
		return err
	}
	files, _ := filepath.Glob("../db/corpus/migrations/*.sql")
	sort.Strings(files)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		up, _, _ := strings.Cut(string(data), "-- migrate:down")
		if _, err := db.Exec(ctx, up); err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
	}
	// Indexes are built after the copy, as a restore does; the timings are reported.
	if _, err := db.Exec(ctx, `DROP INDEX search_postings_hnsw_idx`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `INSERT INTO sources (name) VALUES ('bench')`); err != nil {
		return err
	}
	r := rand.New(rand.NewPCG(11, 11))
	cs := centre(r)
	names := skills.Default.Names()
	start := time.Now()
	pr, pw := io.Pipe()
	go func() {
		for i := 1; i <= n; i++ {
			fmt.Fprintf(pw, "%d\t1\tb%d\tSynthetic job %d\tBench\t\\\\x00\thttps://bench.example/%d\ttrue\n", i, i, i, i)
		}
		pw.Close()
	}()
	if _, err := db.PgConn().CopyFrom(ctx, pr, `COPY postings (id, source_id, source_job_id, title, company,
		content_hash, url, is_engineering) FROM STDIN`); err != nil {
		return fmt.Errorf("copy postings: %w", err)
	}
	pr, pw = io.Pipe()
	go func() {
		w := &strings.Builder{}
		for i := 1; i <= n; i++ {
			w.Reset()
			x, level := r.Float64(), "senior"
			for _, s := range levelShare {
				if x < s.p {
					level = s.level
					break
				}
				x -= s.p
			}
			countries, scopes := "{US}", "{onsite}"
			switch y := r.Float64(); {
			case y < indiaShare:
				countries = "{IN}"
			case y < indiaShare+remoteShare:
				countries, scopes = "{}", "{remote_global}"
			}
			// Zipf skills: a few common ones, a long tail.
			z := rand.NewZipf(r, 1.2, 4, uint64(len(names)-1))
			var req []string
			for range 6 + r.IntN(8) {
				req = append(req, `"`+strings.ReplaceAll(names[z.Uint64()], `"`, `\"`)+`"`)
			}
			v := unit(r, cs[r.IntN(centres)], 0.06)
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t{%s}\t{engineer}\t%s\t%s\t%s\n", i, level, strings.Trim(scopes, "{}"), countries,
				strings.Join(req, ","), scopes, time.Now().Add(-time.Duration(r.IntN(60*24))*time.Hour).Format(time.RFC3339),
				embed.Literal(v))
			_, _ = io.WriteString(pw, w.String())
		}
		pw.Close()
	}()
	if _, err := db.PgConn().CopyFrom(ctx, pr, `COPY search_postings (posting_id, level, remote_scope, countries,
		skills_required, title_terms, remote_scopes, posted_at, embedding) FROM STDIN`); err != nil {
		return fmt.Errorf("copy search_postings: %w", err)
	}
	fmt.Printf("copied %d rows in %s\n", n, time.Since(start).Round(time.Second))
	t := time.Now()
	if _, err := db.Exec(ctx, `CREATE INDEX search_postings_hnsw_idx ON search_postings
		USING hnsw (embedding halfvec_cosine_ops) WITH (m = 16, ef_construction = 64)`); err != nil {
		return err
	}
	fmt.Printf("HNSW built in %s\n", time.Since(t).Round(time.Second))
	t = time.Now()
	if _, err := db.Exec(ctx, `ANALYZE`); err != nil {
		return err
	}
	fmt.Printf("ANALYZE in %s\n", time.Since(t).Round(time.Second))
	return nil
}

func run(ctx context.Context, users, queries, writes int) error {
	target, err := dbURL()
	if err != nil {
		return err
	}
	cfg, err := pgxpool.ParseConfig(target)
	if err != nil {
		return err
	}
	cfg.MaxConns = int32(users + 2)
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	r := rand.New(rand.NewPCG(5, 5))
	cs := centre(rand.New(rand.NewPCG(11, 11))) // the corpus's own centres
	names := skills.Default.Names()
	levels := []string{"intern", "entry", "mid", "senior"}
	type q struct{ p rank.Profile }
	qs := make([]q, queries)
	for i := range qs {
		var sk []string
		for range 6 {
			sk = append(sk, names[r.IntN(80)])
		}
		qs[i] = q{rank.Profile{Skills: sk, Titles: []string{"engineer"}, Level: levels[i%4], Country: "IN", Remote: i%2 == 0,
			Embedding: unit(r, cs[r.IntN(centres)], 0.08)}}
	}
	s := &rank.Searcher{Pool: pool, Weights: rank.Default, Bands: rank.DefaultBands, Now: time.Now}

	// Ingestion load: a writer touching rows at the given rate for the whole run.
	stop := make(chan struct{})
	var wrote atomic.Int64
	go func() {
		if writes <= 0 {
			return
		}
		tick := time.NewTicker(time.Second / time.Duration(writes))
		defer tick.Stop()
		w := rand.New(rand.NewPCG(9, 9))
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				if _, err := pool.Exec(ctx, `UPDATE search_postings SET posted_at = now() WHERE posting_id = $1`,
					1+w.IntN(500000)); err == nil {
					wrote.Add(1)
				}
			}
		}
	}()

	lat := make([]time.Duration, len(qs))
	var next atomic.Int64
	var wg sync.WaitGroup
	var failed atomic.Int64
	start := time.Now()
	for range users {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1)) - 1
				if i >= len(qs) {
					return
				}
				t := time.Now()
				if _, err := s.Search(ctx, qs[i].p, rank.Hybrid, 50); err != nil {
					failed.Add(1)
					fmt.Fprintln(os.Stderr, err)
				}
				lat[i] = time.Since(t)
			}
		}()
	}
	wg.Wait()
	close(stop)
	wall := time.Since(start)
	sorted := append([]time.Duration{}, lat...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	p := func(f float64) time.Duration { return sorted[int(f*float64(len(sorted)-1))] }
	fmt.Printf("%d hybrid searches, %d users, %d failed, %d writes during the run, %.0f searches a second\n",
		len(qs), users, failed.Load(), wrote.Load(), float64(len(qs))/wall.Seconds())
	fmt.Printf("p50 %s  p95 %s  p99 %s  max %s\n", p(0.5).Round(time.Millisecond), p(0.95).Round(time.Millisecond),
		p(0.99).Round(time.Millisecond), sorted[len(sorted)-1].Round(time.Millisecond))
	edges := []time.Duration{25, 50, 100, 150, 200, 300, 500, 1000, 2000}
	counts := make([]int, len(edges)+1)
	for _, d := range sorted {
		k := sort.Search(len(edges), func(i int) bool { return d <= edges[i]*time.Millisecond })
		counts[k]++
	}
	prev := "0"
	for i, e := range edges {
		fmt.Printf("  %5s to %5d ms: %d\n", prev, e, counts[i])
		prev = fmt.Sprint(e)
	}
	fmt.Printf("  over %5s ms: %d\n", prev, counts[len(edges)])
	return explainCheck(ctx, pool, qs[0].p)
}

// explainCheck compares the client's timer with the server's own execution time for the
// vector query, on one connection with nothing else running. The Phase 0 harness overstated
// latency by up to 100x; this is the check that it doesn't here.
func explainCheck(ctx context.Context, pool *pgxpool.Pool, p rank.Profile) error {
	sql := `SELECT posting_id FROM search_postings WHERE embedding IS NOT NULL AND level = ANY($1)
		AND ($2 = ANY(countries) OR ($3 AND 'remote_global' = ANY(remote_scopes)))
		ORDER BY (embedding <=> $4::halfvec) + 0, posting_id LIMIT 200`
	args := []any{rank.Levels(p.Level), p.Country, p.Remote, embed.Literal(p.Embedding)}
	for range 3 {
		t := time.Now()
		rows, err := pool.Query(ctx, sql, args...)
		if err != nil {
			return err
		}
		n := 0
		for rows.Next() {
			n++
		}
		rows.Close()
		client := time.Since(t)
		var plan string
		lines, err := pool.Query(ctx, "EXPLAIN (ANALYZE, TIMING OFF) "+sql, args...)
		if err != nil {
			return err
		}
		var last string
		for lines.Next() {
			_ = lines.Scan(&plan)
			last = plan
		}
		lines.Close()
		fmt.Printf("vector query: client %s for %d rows; server %s\n", client.Round(time.Millisecond), n, strings.TrimSpace(last))
	}
	return nil
}
