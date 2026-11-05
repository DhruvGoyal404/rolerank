//go:build onnx

// Command embed fills search_postings embeddings, or embeds JSONL text for evals and the bench.
//
//	embed jobs   embeds every search row that has no vector yet
//	embed text   reads {"id","text"} lines on stdin, writes {"id","vector"} lines on stdout
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DhruvGoyal404/rolerank/worker/internal/embed"
)

func main() {
	lib := flag.String("lib", os.Getenv("ROLERANK_ORT_LIB"), "path to libonnxruntime")
	model := flag.String("model", os.Getenv("ROLERANK_EMBED_MODEL"), "path to model.onnx")
	threads := flag.Int("threads", 4, "intra-op threads")
	batch := flag.Int("batch", 16, "texts per model call")
	prefix := flag.Bool("query-prefix", false, "prepend bge's query instruction (text mode)")
	// The mode comes first and the flags after it ("embed text -query-prefix"); Go's flag package
	// stops at the first non-flag, so the flags are parsed from after the mode.
	mode := ""
	if len(os.Args) > 1 {
		mode = os.Args[1]
		_ = flag.CommandLine.Parse(os.Args[2:])
	}
	if flag.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "embed: unexpected arguments", flag.Args())
		os.Exit(2)
	}
	if err := run(mode, *lib, *model, *threads, *batch, *prefix); err != nil {
		fmt.Fprintln(os.Stderr, "embed:", err)
		os.Exit(1)
	}
}

func run(mode, lib, model string, threads, batch int, prefix bool) error {
	e, err := embed.Open(lib, model, threads)
	if err != nil {
		return err
	}
	defer e.Close()
	switch mode {
	case "jobs":
		return jobs(context.Background(), e, batch)
	case "text":
		return text(e, batch, prefix)
	}
	return errors.New("usage: embed jobs|text")
}

func jobs(ctx context.Context, e *embed.Embedder, batch int) error {
	pool, err := pgxpool.New(ctx, os.Getenv("ROLERANK_CORPUS_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	start, done := time.Now(), 0
	for {
		rows, err := pool.Query(ctx, `
			SELECT sp.posting_id, sp.embed_hash, p.title, sp.skills_required, p.body
			FROM search_postings sp JOIN postings p ON p.id = sp.posting_id
			WHERE sp.embedding IS NULL ORDER BY sp.posting_id LIMIT 256`)
		if err != nil {
			return err
		}
		type job struct {
			id   int64
			hash []byte
			text string
		}
		list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (job, error) {
			var j job
			var title, body string
			var skills []string
			err := r.Scan(&j.id, &j.hash, &title, &skills, &body)
			j.text = embed.PostingText(title, skills, body)
			return j, err
		})
		if err != nil {
			return err
		}
		if len(list) == 0 {
			fmt.Printf("embedded %d jobs in %s\n", done, time.Since(start).Round(time.Second))
			return nil
		}
		for i := 0; i < len(list); i += batch {
			part := list[i:min(i+batch, len(list))]
			texts := make([]string, len(part))
			for k, j := range part {
				texts[k] = j.text
			}
			vectors, err := e.Embed(texts)
			if err != nil {
				return err
			}
			b := &pgx.Batch{}
			for k, j := range part {
				// The hash guard skips a row whose text changed while it was being embedded.
				b.Queue(`UPDATE search_postings SET embedding = $2::halfvec, embed_model = $3
					WHERE posting_id = $1 AND embed_hash = $4`, j.id, embed.Literal(vectors[k]), embed.ModelTag, j.hash)
			}
			if err := pool.SendBatch(ctx, b).Close(); err != nil {
				return err
			}
			done += len(part)
		}
		fmt.Fprintf(os.Stderr, "%d jobs, %.1f a second\n", done, float64(done)/time.Since(start).Seconds())
	}
}

func text(e *embed.Embedder, batch int, prefix bool) error {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 1<<24)
	out := json.NewEncoder(os.Stdout)
	type item struct {
		ID     string    `json:"id"`
		Text   string    `json:"text,omitempty"`
		Vector []float32 `json:"vector,omitempty"`
	}
	var pending []item
	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		texts := make([]string, len(pending))
		for i, p := range pending {
			texts[i] = p.Text
			if prefix {
				texts[i] = embed.QueryPrefix + p.Text
			}
		}
		vectors, err := e.Embed(texts)
		if err != nil {
			return err
		}
		for i, p := range pending {
			if err := out.Encode(item{ID: p.ID, Vector: vectors[i]}); err != nil {
				return err
			}
		}
		pending = pending[:0]
		return nil
	}
	for in.Scan() {
		var it item
		if err := json.Unmarshal(in.Bytes(), &it); err != nil {
			return err
		}
		pending = append(pending, it)
		if len(pending) == batch {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := in.Err(); err != nil {
		return err
	}
	return flush()
}
