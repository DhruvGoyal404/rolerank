// Command label prepares and collects the hand labels for D5 and D6.
//
//	label select <root> <seed>     draw the items into evals/private
//	label serve <root> <dataset>   open http://127.0.0.1:8765, keyboard only
//	label recheck <root> <dataset> <ids file> <seed> [full]   draw a blind second pass
//	label redo <root> <dataset> <ids file> <seed> [name]   draw a blind redo, with full text
//	label agree <root> <dataset>   score a finished second pass
package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
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
	if len(args) == 5 && args[0] == "recheck" {
		return draw(args[1], args[2], "recheck", args[3], args[4], false)
	}
	if len(args) == 6 && args[0] == "recheck" && args[5] == "full" {
		return draw(args[1], args[2], "recheck", args[3], args[4], true)
	}
	if len(args) == 5 && args[0] == "redo" {
		return draw(args[1], args[2], "redo", args[3], args[4], true)
	}
	if len(args) == 6 && args[0] == "redo" {
		return draw(args[1], args[2], args[5], args[3], args[4], true)
	}
	if len(args) == 3 && args[0] == "agree" {
		return agree(args[1], args[2])
	}
	return fmt.Errorf("usage: label select <root> <seed> | label serve <root> <dataset> | " +
		"label recheck <root> <dataset> <ids file> <seed> [full] | " +
		"label redo <root> <dataset> <ids file> <seed> [name] | label agree <root> <dataset>")
}

func draw(root, dataset, pass, idsFile, seed string, full bool) error {
	data, err := os.ReadFile(idsFile)
	if err != nil {
		return err
	}
	ids := strings.Fields(string(data))
	var refresh func([]label.Item) error
	if full {
		ctx := context.Background()
		pool, err := pgxpool.New(ctx, os.Getenv("ROLERANK_CORPUS_URL"))
		if err != nil {
			return fmt.Errorf("ROLERANK_CORPUS_URL is missing or invalid")
		}
		defer pool.Close()
		refresh = func(items []label.Item) error { return label.FullText(ctx, pool, items) }
	}
	if err := label.Recheck(root, dataset, dataset+"-"+pass, ids, seed, refresh); err != nil {
		return err
	}
	fmt.Printf("wrote %s-%s with %d items\n", dataset, pass, len(ids))
	return nil
}

func agree(root, dataset string) error {
	results, err := label.Compare(root, dataset, dataset+"-recheck")
	if err != nil {
		return err
	}
	for _, got := range results {
		fmt.Printf("%s: agreed on %d of %d, Cohen's kappa %.3f (95%% interval %.3f to %.3f)\n",
			got.Question, got.Same, got.Pairs, got.Kappa, got.Low, got.High)
		keys := make([][2]string, 0, len(got.Table))
		for k := range got.Table {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i][0]+keys[i][1] < keys[j][0]+keys[j][1] })
		for _, k := range keys {
			fmt.Printf("  first %s, second %s: %d\n", k[0], k[1], got.Table[k])
		}
	}
	return nil
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
