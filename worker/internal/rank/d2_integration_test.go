//go:build integration

package rank

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DhruvGoyal404/rolerank/worker/internal/d2"
	"github.com/DhruvGoyal404/rolerank/worker/internal/embed"
	"github.com/DhruvGoyal404/rolerank/worker/internal/skills"
)

// testDB creates an isolated corpus database named *_test, with every migration applied.
func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	admin := os.Getenv("ROLERANK_TEST_ADMIN_URL")
	if admin == "" {
		t.Fatal("ROLERANK_TEST_ADMIN_URL is required")
	}
	b := make([]byte, 5)
	_, _ = rand.Read(b)
	name := "rrrank" + hex.EncodeToString(b) + "_corpus_test"
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		_ = conn.Close(context.Background())
	})
	u, _ := url.Parse(admin)
	u.Path = "/" + name
	db, err := pgx.Connect(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "CREATE EXTENSION vector"); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob("../../../db/corpus/migrations/*.sql")
	sort.Strings(files)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		up, _, _ := strings.Cut(string(data), "-- migrate:down")
		if _, err := db.Exec(ctx, up); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}
	_ = db.Close(ctx)
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func vectors(t *testing.T) map[string][]float32 {
	t.Helper()
	f, err := os.Open("../d2/testdata/vectors.jsonl.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]float32{}
	sc := bufio.NewScanner(z)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var x struct {
			ID string    `json:"id"`
			V  []float32 `json:"v"`
		}
		if err := json.Unmarshal(sc.Bytes(), &x); err != nil {
			t.Fatal(err)
		}
		out[x.ID] = x.V
	}
	return out
}

// TestD2 runs EVALS.md's D2 through the real SQL and reports every mode. It gates the hybrid.
func TestD2(t *testing.T) {
	ctx := context.Background()
	pool := testDB(t)
	vec := vectors(t)
	ps, js := d2.Generate(d2.Seed, d2.Profiles, d2.Jobs)
	posted := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `INSERT INTO sources (name) VALUES ('d2')`); err != nil {
		t.Fatal(err)
	}
	byID := map[int64]d2.Job{}
	for _, j := range js {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO postings (source_id, source_job_id, title, company, is_engineering, body,
			content_hash, url) SELECT id, $1, $2, 'D2', true, $3, '\x00', 'https://d2.example/' || $1 FROM sources
			WHERE name = 'd2' RETURNING id`, j.ID, j.Title, j.Body).Scan(&id); err != nil {
			t.Fatal(err)
		}
		byID[id] = j
		req, nice := skills.Default.Extract(j.Body)
		countries, scopes := []string{j.Place}, []string{"onsite"}
		if j.Place == "remote" {
			countries, scopes = []string{}, []string{"remote_global"}
		}
		if _, err := pool.Exec(ctx, `INSERT INTO search_postings (posting_id, level, remote_scope, countries, skills_required,
			skills_nice, title_terms, remote_scopes, posted_at, embedding, embed_model)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::halfvec, $11)`, id, j.Level, scopes[0], countries,
			append([]string{}, req...), append([]string{}, nice...), skills.TitleTerms(j.Title), scopes, posted,
			embed.Literal(vec[j.ID]), embed.ModelTag); err != nil {
			t.Fatal(err)
		}
	}
	s := &Searcher{Pool: pool, Weights: Default, Bands: DefaultBands, Now: func() time.Time { return posted }}
	run := func(mode Mode, prefix bool) Summary {
		var ms []Metrics
		slots, early := 0, 0
		for _, p := range ps {
			key := p.ID
			if prefix {
				key = "q" + p.ID
			}
			title := strings.Split(p.Text, ".")[0]
			prof := Profile{Skills: p.Skills, Titles: skills.TitleTerms(title), Level: p.Level, Country: d2.Country,
				Remote: p.Remote, Embedding: vec[key]}
			got, err := s.Search(ctx, prof, mode, 400)
			if err != nil {
				t.Fatal(err)
			}
			ids := make([]int64, len(got))
			for i, r := range got {
				ids[i] = r.ID
			}
			grades := map[int64]int{}
			for id, j := range byID {
				grades[id] = d2.Grade(p, j)
			}
			m := Measure(p.Level, ids, grades, func(id int64) bool { return d2.Open(p, byID[id]) },
				func(id int64) bool { return byID[id].Level == "senior" })
			ms = append(ms, m)
			slots += min(10, len(ids))
			if p.Level == "intern" || p.Level == "entry" {
				early += min(10, len(ids))
			}
		}
		return Summarise(ms, slots, early)
	}
	results := map[string]Summary{}
	for _, mode := range []Mode{Lexical, Vector, Hybrid} {
		for _, prefix := range []bool{false, true} {
			if mode == Lexical && prefix {
				continue
			}
			sum := run(mode, prefix)
			name := fmt.Sprintf("%s prefix=%v", mode, prefix)
			results[name] = sum
			t.Logf("D2 %-22s nDCG@10 %.3f  MRR %.3f  Recall %.3f  level-viol %.3f  geo-viol %.3f  judged@10 %.2f",
				name, sum.NDCG10, sum.MRR, sum.Recall, sum.LevelViolation, sum.GeoViolate, sum.Judged10)
		}
	}
	// A rank is reproducible from stored features: the same search twice gives the same order
	// and the same scores.
	p := ps[0]
	prof := Profile{Skills: p.Skills, Titles: skills.TitleTerms(strings.Split(p.Text, ".")[0]), Level: p.Level,
		Country: d2.Country, Remote: p.Remote, Embedding: vec["q"+p.ID]}
	first, err := s.Search(ctx, prof, Hybrid, 50)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Search(ctx, prof, Hybrid, 50)
	if err != nil {
		t.Fatal(err)
	}
	for i := range first {
		if first[i].ID != again[i].ID || first[i].Score != again[i].Score {
			t.Fatalf("rank %d differs between runs: %d %.6f vs %d %.6f", i, first[i].ID, first[i].Score, again[i].ID, again[i].Score)
		}
	}

	// Profiles are embedded with bge's query prefix: the P3 bake-off chose it on D3.
	h := results["hybrid prefix=true"]
	if h.NDCG10 < 0.85 || h.MRR < 0.90 || h.Recall < 0.98 || h.LevelViolation > 0.01 || h.GeoViolate != 0 {
		t.Errorf("D2 thresholds missed by the hybrid: %+v", h)
	}
}
