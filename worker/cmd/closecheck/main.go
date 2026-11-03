// Command closecheck measures closed-posting accuracy: it refetches a seeded random sample of
// boards without conditional GET, through the real fetcher and limiter, and compares each with
// the active set in the corpus. It prints counts only.
//
//	ROLERANK_CORPUS_URL=... ROLERANK_VALKEY_URL=... closecheck <boards> <seed>
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/valkey-io/valkey-go"

	"github.com/DhruvGoyal404/rolerank/worker/internal/fetch"
	"github.com/DhruvGoyal404/rolerank/worker/internal/limiter"
	"github.com/DhruvGoyal404/rolerank/worker/internal/source"
	"github.com/DhruvGoyal404/rolerank/worker/internal/useragent"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "closecheck:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: closecheck <boards> <seed>")
	}
	count, err := strconv.Atoi(args[0])
	if err != nil {
		return err
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("ROLERANK_CORPUS_URL"))
	if err != nil {
		return fmt.Errorf("ROLERANK_CORPUS_URL is missing or invalid")
	}
	defer pool.Close()
	options, err := valkey.ParseURL(os.Getenv("ROLERANK_VALKEY_URL"))
	if err != nil {
		return fmt.Errorf("ROLERANK_VALKEY_URL is missing or invalid")
	}
	client, err := valkey.NewClient(options)
	if err != nil {
		return err
	}
	defer client.Close()

	adapters := map[string]source.Adapter{}
	var hosts []string
	for _, a := range source.All() {
		adapters[a.Name()] = a
		hosts = append(hosts, a.Hosts()...)
	}
	fetcher := fetch.New(fetch.Options{
		UserAgent: useragent.String("0.1.0"), Agent: "RoleRankBot", Hosts: hosts,
		Limiter: limiter.New(client, "rolerank:"),
	})

	rows, err := pool.Query(ctx, `
		SELECT b.id, s.name, b.token FROM boards b JOIN sources s ON s.id = b.source_id
		WHERE b.status = 'active' AND NOT b.denylisted AND s.name IN ('greenhouse', 'lever')
		  AND b.last_checked_at > now() - interval '6 hours'
		ORDER BY md5(b.token || $1) LIMIT $2`, args[1], count)
	if err != nil {
		return err
	}
	type board struct {
		id            int64
		source, token string
	}
	boards, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (board, error) {
		var b board
		err := r.Scan(&b.id, &b.source, &b.token)
		return b, err
	})
	if err != nil {
		return err
	}

	var fresh, active, staleOpen, wrongClosed, failed int
	for _, b := range boards {
		adapter := adapters[b.source]
		live := map[string]bool{}
		next := adapter.FirstURL(b.token)
		ok := true
		for next != "" {
			result, err := fetcher.Get(ctx, next, "")
			if err != nil {
				ok = false
				break
			}
			page, err := adapter.Parse(b.token, result.Body)
			if err != nil {
				ok = false
				break
			}
			for _, p := range page.Postings {
				live[b.token+":"+p.SourceJobID] = true
			}
			next = page.NextURL
		}
		if !ok {
			failed++
			continue
		}
		rows, err := pool.Query(ctx, `SELECT source_job_id FROM postings WHERE board_id = $1 AND closed_at IS NULL`, b.id)
		if err != nil {
			return err
		}
		held, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		have := map[string]bool{}
		for _, id := range held {
			have[id] = true
			if !live[id] {
				staleOpen++
			}
		}
		for id := range live {
			if !have[id] {
				wrongClosed++
			}
		}
		fresh += len(live)
		active += len(held)
	}
	fmt.Printf("boards %d, failed %d, fresh postings %d, active in corpus %d\n", len(boards), failed, fresh, active)
	fmt.Printf("active but gone from the board: %d (%.3f%%)\n", staleOpen, 100*float64(staleOpen)/float64(max(active, 1)))
	fmt.Printf("on the board but not active: %d (%.3f%%)\n", wrongClosed, 100*float64(wrongClosed)/float64(max(fresh, 1)))
	return nil
}
