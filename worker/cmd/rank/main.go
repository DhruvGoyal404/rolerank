// Command rank runs D3 (EVALS.md) against db-corpus.
//
//	rank pool  <profiles.json> <vectors.jsonl> <items.jsonl>   pool each profile's top 10 from
//	           every mode into grading items (posting text: write only under evals/private)
//	rank score <profiles.json> <vectors.jsonl> <grades.jsonl>  metrics per mode and segment, and
//	           the hybrid's paired bootstrap against each baseline
//	rank fit   <profiles.json> <vectors.jsonl> <grades.jsonl>  fits the rerank weights, reporting
//	           a cross-validated nDCG@10 over held-out profiles
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DhruvGoyal404/rolerank/worker/internal/rank"
	"github.com/DhruvGoyal404/rolerank/worker/internal/skills"
)

type profile struct {
	ID      string   `json:"id"`
	Segment string   `json:"segment"`
	Level   string   `json:"level"`
	Remote  bool     `json:"remote"`
	Titles  string   `json:"titles"`
	Skills  []string `json:"skills"`
	Text    string   `json:"text"`
}

const pooled = 10

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "rank:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 4 {
		return errors.New("usage: rank pool|score|fit|bands <profiles.json> <vectors.jsonl> <file>")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("ROLERANK_CORPUS_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	ps, err := load(args[1], args[2])
	if err != nil {
		return err
	}
	s := &rank.Searcher{Pool: pool, Weights: rank.Default, Bands: rank.DefaultBands, Now: time.Now}
	switch args[0] {
	case "pool":
		return poolItems(ctx, pool, s, ps, args[3])
	case "score":
		return score(ctx, s, ps, args[3])
	case "fit":
		return fit(ctx, s, ps, args[3])
	case "bands":
		return bands(ctx, s, ps, args[3])
	}
	return errors.New("usage: rank pool|score|fit|bands")
}

type loaded struct {
	profile
	rank.Profile
}

func load(profiles, vectors string) ([]loaded, error) {
	raw, err := os.ReadFile(profiles)
	if err != nil {
		return nil, err
	}
	var ps []profile
	if err := json.Unmarshal(raw, &ps); err != nil {
		return nil, err
	}
	vec := map[string][]float32{}
	f, err := os.Open(vectors)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<22)
	for sc.Scan() {
		var x struct {
			ID     string    `json:"id"`
			Vector []float32 `json:"vector"`
		}
		if err := json.Unmarshal(sc.Bytes(), &x); err != nil {
			return nil, err
		}
		vec[x.ID] = x.Vector
	}
	var out []loaded
	for _, p := range ps {
		if vec[p.ID] == nil {
			return nil, fmt.Errorf("no vector for %s", p.ID)
		}
		out = append(out, loaded{p, rank.Profile{Skills: p.Skills, Titles: skills.TitleTerms(p.Titles), Level: p.Level,
			Country: "IN", Remote: p.Remote, Embedding: vec[p.ID]}})
	}
	return out, nil
}

var modes = []rank.Mode{rank.Lexical, rank.Vector, rank.Hybrid}

func poolItems(ctx context.Context, db *pgxpool.Pool, s *rank.Searcher, ps []loaded, out string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	total := 0
	for _, p := range ps {
		var ids []int64
		for _, m := range modes {
			got, err := s.Search(ctx, p.Profile, m, pooled)
			if err != nil {
				return err
			}
			for _, r := range got {
				ids = append(ids, r.ID)
			}
		}
		slices.Sort(ids)
		ids = slices.Compact(ids)
		rows, err := db.Query(ctx, `
			SELECT p.id, src.name || ':' || p.source_job_id, p.title, p.company, COALESCE(p.location_raw, ''), p.body
			FROM postings p JOIN sources src ON src.id = p.source_id WHERE p.id = ANY($1) ORDER BY p.id`, ids)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id int64
			var key, title, company, location, body string
			if err := rows.Scan(&id, &key, &title, &company, &location, &body); err != nil {
				return err
			}
			if err := enc.Encode(map[string]any{"profile": p.ID, "posting": id, "key": key, "title": title,
				"company": company, "location": location, "body": body}); err != nil {
				return err
			}
			total++
		}
		if err := rows.Err(); err != nil {
			return err
		}
	}
	fmt.Printf("pooled %d items for %d profiles\n", total, len(ps))
	return nil
}

func score(ctx context.Context, s *rank.Searcher, ps []loaded, gradesFile string) error {
	grades, err := loadGrades(gradesFile)
	if err != nil {
		return err
	}
	per := map[rank.Mode][]rank.Metrics{}
	segs := map[string]map[rank.Mode][]rank.Metrics{}
	for _, p := range ps {
		for _, m := range modes {
			got, err := s.Search(ctx, p.Profile, m, 200)
			if err != nil {
				return err
			}
			ids := make([]int64, len(got))
			senior := map[int64]bool{}
			for i, r := range got {
				ids[i] = r.ID
				senior[r.ID] = r.Level == "senior"
			}
			// Geography is a hard filter, so every returned job lists the profile's country or is
			// remote for anyone; whether the text agrees is in the grade (0 = ineligible). Recall is
			// over the 200 returned, so it is the retrieval stage's.
			met := rank.Measure(p.profile.Level, ids, grades[p.ID], func(int64) bool { return true },
				func(id int64) bool { return senior[id] })
			per[m] = append(per[m], met)
			if segs[p.Segment] == nil {
				segs[p.Segment] = map[rank.Mode][]rank.Metrics{}
			}
			segs[p.Segment][m] = append(segs[p.Segment][m], met)
		}
	}
	for _, m := range modes {
		sum := rank.Summarise(per[m], 0, 0)
		slots, early := 0, 0
		for i, met := range per[m] {
			_ = met
			slots += 10
			if l := ps[i].profile.Level; l == "intern" || l == "entry" {
				early += 10
			}
		}
		sum = rank.Summarise(per[m], slots, early)
		fmt.Printf("%-8s nDCG@10 %.3f  MRR %.3f  Recall@200 %.3f  level-viol@10 %.3f  judged@10 %.2f\n", m, sum.NDCG10, sum.MRR,
			sum.Recall, sum.LevelViolation, sum.Judged10)
	}
	names := make([]string, 0, len(segs))
	for k := range segs {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Printf("segment %-12s", n)
		for _, m := range modes {
			fmt.Printf(" %s %.3f", m, rank.Summarise(segs[n][m], 0, 0).NDCG10)
		}
		fmt.Println()
	}
	for _, base := range []rank.Mode{rank.Lexical, rank.Vector} {
		lo, hi := bootstrap(per[rank.Hybrid], per[base])
		fmt.Printf("hybrid - %s nDCG@10: 95%% interval %.3f to %.3f\n", base, lo, hi)
	}
	return nil
}

// bootstrap resamples profiles with replacement and returns the 95% interval of the mean
// paired nDCG@10 difference, a minus b. Seeded, so a rerun prints the same interval.
func bootstrap(a, b []rank.Metrics) (float64, float64) {
	r := rand.New(rand.NewPCG(3, 3))
	var diffs []float64
	for range 10000 {
		var d float64
		for range a {
			i := r.IntN(len(a))
			d += a[i].NDCG10 - b[i].NDCG10
		}
		diffs = append(diffs, d/float64(len(a)))
	}
	sort.Float64s(diffs)
	return diffs[250], diffs[9750]
}
