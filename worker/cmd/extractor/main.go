// Command extractor serves resume extraction to aiworker on the internal network:
// POST /extract with the file as the body returns the sandbox's JSON. Every request needs the
// shared token in Authorization, and at most two extractions run at once.
package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/DhruvGoyal404/rolerank/worker/internal/extract"
)

func main() {
	token := os.Getenv("ROLERANK_EXTRACTOR_TOKEN")
	if len(token) < 32 {
		fmt.Fprintln(os.Stderr, "extractor: ROLERANK_EXTRACTOR_TOKEN must be at least 32 characters")
		os.Exit(2)
	}
	r := extract.Runner{Image: os.Getenv("ROLERANK_EXTRACTOR_IMAGE"), Runtime: os.Getenv("ROLERANK_SANDBOX_RUNTIME")}
	if r.Image == "" {
		fmt.Fprintln(os.Stderr, "extractor: ROLERANK_EXTRACTOR_IMAGE is required")
		os.Exit(2)
	}
	addr := os.Getenv("ROLERANK_EXTRACTOR_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8090"
	}
	srv := &http.Server{Addr: addr, Handler: Handler(r, token, 2), ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second}
	slog.Info("extractor listening", "addr", addr, "runtime", r.Runtime)
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("extractor stopped", "err", err)
		os.Exit(1)
	}
}

// failure names why a run failed, in a fixed word aiworker maps to a message for the user.
func failure(err error) string {
	switch {
	case errors.Is(err, extract.ErrStartup):
		return "startup"
	case errors.Is(err, extract.ErrTimeout):
		return "timeout"
	case errors.Is(err, extract.ErrOutput):
		return "output"
	}
	return "sandbox"
}

type runner interface {
	Run(context.Context, []byte) (extract.Result, error)
}

// Handler serves POST /extract and GET /health. Responses never echo resume content into logs.
func Handler(r runner, token string, concurrent int) http.Handler {
	slots := make(chan struct{}, concurrent)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("POST /extract", func(w http.ResponseWriter, req *http.Request) {
		if subtle.ConstantTimeCompare([]byte(req.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, extract.MaxBytes+1))
		if err != nil || len(body) > extract.MaxBytes {
			http.Error(w, "too large", http.StatusRequestEntityTooLarge)
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		case <-req.Context().Done():
			return
		}
		res, err := r.Run(req.Context(), body)
		w.Header().Set("Content-Type", "application/json")
		if errors.Is(err, extract.ErrRejected) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "rejected"})
			return
		}
		if err != nil {
			reason := failure(err)
			slog.Warn("extract failed", "reason", reason, "bytes", len(body)) // a fixed word: no content, no error text from the file
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed", "reason": reason})
			return
		}
		_ = json.NewEncoder(w).Encode(res)
	})
	return mux
}
