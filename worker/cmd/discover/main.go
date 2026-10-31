// Command discover finds board names in Common Crawl's URL index and writes them to
// data/<source>.txt, one per line. Safe to stop and rerun: finished pages aren't repeated.
//
//	discover <data dir> [indexes]
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/DhruvGoyal404/rolerank/worker/internal/discover"
	"github.com/DhruvGoyal404/rolerank/worker/internal/useragent"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "discover:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: discover <data dir> [indexes]")
	}
	count := 2
	if len(args) > 1 {
		parsed, err := strconv.Atoi(args[1])
		if err != nil {
			return err
		}
		count = parsed
	}
	if err := os.MkdirAll(args[0], 0o700); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runner := &discover.Runner{
		Dir:       args[0],
		UserAgent: useragent.String("0.1.0"),
		Client:    &http.Client{Timeout: 3 * time.Minute},
		Base:      "https://index.commoncrawl.org",
		Sleep:     time.Sleep,
		Log: func(line string) {
			fmt.Println(time.Now().Format(time.TimeOnly), line)
		},
	}
	indexes, err := runner.LatestIndexes(ctx, count)
	if err != nil {
		return err
	}
	undone, err := runner.Run(ctx, indexes)
	if err != nil {
		return err
	}
	fmt.Printf("done; %d pages left for the next run\n", undone)
	return nil
}
