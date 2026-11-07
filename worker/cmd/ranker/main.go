//go:build onnx

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DhruvGoyal404/rolerank/worker/internal/embed"
	"github.com/DhruvGoyal404/rolerank/worker/internal/rank"
)

// The embedder needs cgo and ONNX Runtime, so the binary builds only with -tags onnx, in the
// Linux container (compose service "ranker"). The handler is tested without it.
func main() {
	token := os.Getenv("ROLERANK_RANKER_TOKEN")
	if len(token) < 32 {
		fmt.Fprintln(os.Stderr, "ranker: ROLERANK_RANKER_TOKEN must be at least 32 characters")
		os.Exit(2)
	}
	e, err := embed.Open(os.Getenv("ROLERANK_ORT_LIB"), os.Getenv("ROLERANK_EMBED_MODEL"), 2)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ranker:", err)
		os.Exit(2)
	}
	defer e.Close()
	pool, err := pgxpool.New(context.Background(), os.Getenv("ROLERANK_CORPUS_URL"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "ranker: corpus:", err)
		os.Exit(2)
	}
	defer pool.Close()
	s := &rank.Searcher{Pool: pool, Weights: rank.Default, Bands: rank.DefaultBands, Now: time.Now}
	addr := os.Getenv("ROLERANK_RANKER_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8094"
	}
	srv := &http.Server{Addr: addr, Handler: Handler(e, s, token), ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second}
	slog.Info("ranker listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("ranker stopped", "err", err)
		os.Exit(1)
	}
}
