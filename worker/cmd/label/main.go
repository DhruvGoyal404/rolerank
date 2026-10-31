// Command label prepares and collects the hand labels for D5 and D6.
//
//	label select <root> <seed>     draw the items into evals/private
//	label serve <root> d5|d6       open http://127.0.0.1:8765, keyboard only
package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DhruvGoyal404/rolerank/worker/internal/label"
)

const address = "127.0.0.1:8765" // loopback only: this page shows real posting text

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "label:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 3 && args[0] == "select" {
		return choose(args[1], args[2])
	}
	if len(args) == 3 && args[0] == "serve" {
		return serve(args[1], args[2])
	}
	return fmt.Errorf("usage: label select <root> <seed> | label serve <root> d5|d6")
}

func report(name string, counts map[string]int) {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %s %s: %d\n", name, k, counts[k])
	}
}

func choose(root, seed string) error {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("ROLERANK_CORPUS_URL"))
	if err != nil {
		return fmt.Errorf("ROLERANK_CORPUS_URL is missing or invalid")
	}
	defer pool.Close()
	d6, counts, err := label.SelectD6(ctx, pool, seed)
	if err != nil {
		return err
	}
	report("d6", counts)
	if err := label.WriteItems(root, "d6", d6); err != nil {
		return err
	}
	d5, counts, err := label.SelectD5(ctx, pool, seed)
	if err != nil {
		return err
	}
	report("d5", counts)
	if err := label.WriteItems(root, "d5", d5); err != nil {
		return err
	}
	fmt.Printf("selected %d D5 pairs and %d D6 postings\n", len(d5), len(d6))
	return nil
}

func serve(root, dataset string) error {
	session, err := label.Open(root, dataset)
	if err != nil {
		return err
	}
	defer session.Close()
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	fmt.Printf("open http://%s  (Ctrl-C when done; every key is saved as you press it)\n", address)
	server := &http.Server{Handler: label.Handler(session), ReadHeaderTimeout: 5 * time.Second}
	return server.Serve(listener)
}
