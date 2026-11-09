package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/DhruvGoyal404/rolerank/worker/internal/rank"
)

func loadGrades(file string) (map[string]map[int64]int, error) {
	grades := map[string]map[int64]int{}
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var g struct {
			Profile string `json:"profile"`
			Posting int64  `json:"posting"`
			Grade   int    `json:"grade"`
		}
		if err := json.Unmarshal(sc.Bytes(), &g); err != nil {
			return nil, err
		}
		if grades[g.Profile] == nil {
			grades[g.Profile] = map[int64]int{}
		}
		grades[g.Profile][g.Posting] = g.Grade
	}
	return grades, sc.Err()
}

type example struct {
	id     string
	cands  []rank.Ranked
	grades map[int64]int
}

func ndcg(w rank.Weights, ex []example) float64 {
	var sum float64
	for _, e := range ex {
		r := make([]rank.Ranked, len(e.cands))
		copy(r, e.cands)
		for i := range r {
			r[i].Score = w.Total(r[i].Features)
		}
		sort.SliceStable(r, func(i, j int) bool {
			if r[i].Score != r[j].Score {
				return r[i].Score > r[j].Score
			}
			return r[i].ID < r[j].ID
		})
		ids := make([]int64, len(r))
		for i := range r {
			ids[i] = r[i].ID
		}
		sum += rank.Measure("", ids, e.grades, func(int64) bool { return true }, func(int64) bool { return false }).NDCG10
	}
	return sum / float64(len(ex))
}

// minRequired keeps a floor under the required-skill weight. D3 alone drives it to zero, which
// fails D2 (skill coverage is what D2 grades), and the weights must pass both.
var minRequired = func() float64 {
	var v float64
	_, _ = fmt.Sscan(os.Getenv("ROLERANK_FIT_MIN_REQUIRED"), &v)
	return v
}()

// ascend is coordinate ascent over the weights on a fixed grid, from the defaults. Deterministic.
func ascend(ex []example) rank.Weights {
	w := rank.Default
	w.Required = max(w.Required, minRequired)
	fields := []*float64{&w.Required, &w.Nice, &w.Cosine, &w.Title, &w.Level, &w.Location, &w.Fresh, &w.Fusion}
	grid := []float64{0, 0.025, 0.05, 0.1, 0.15, 0.2, 0.3, 0.4, 0.5, 0.7, 1}
	best := ndcg(w, ex)
	for range 4 {
		improved := false
		for _, f := range fields {
			keep := *f
			for _, v := range grid {
				if f == &w.Required && v < minRequired {
					continue
				}
				*f = v
				if got := ndcg(w, ex); got > best+1e-9 {
					best, keep, improved = got, v, true
				}
			}
			*f = keep
		}
		if !improved {
			break
		}
	}
	return w
}

func fit(ctx context.Context, s *rank.Searcher, ps []loaded, gradesFile string) error {
	grades, err := loadGrades(gradesFile)
	if err != nil {
		return err
	}
	var ex []example
	for _, p := range ps {
		c, err := s.Search(ctx, p.Profile, rank.Hybrid, 1000) // every candidate, with its features
		if err != nil {
			return err
		}
		ex = append(ex, example{p.ID, c, grades[p.ID]})
	}
	// 5 folds of 4 profiles: weights fitted on 16 profiles are scored on the 4 held out.
	const folds = 5
	var cvFit, cvDefault float64
	for k := range folds {
		var train, test []example
		for i, e := range ex {
			if i%folds == k {
				test = append(test, e)
			} else {
				train = append(train, e)
			}
		}
		w := ascend(train)
		cvFit += ndcg(w, test) * float64(len(test))
		cvDefault += ndcg(rank.Default, test) * float64(len(test))
	}
	n := float64(len(ex))
	fmt.Printf("cross-validated nDCG@10 (5 folds, held-out profiles): default %.3f, fitted %.3f\n", cvDefault/n, cvFit/n)
	w := ascend(ex)
	w.Version = "v1-d3-fit"
	fmt.Printf("weights fitted on all %d profiles (in-sample nDCG@10 %.3f): %+v\n", len(ex), ndcg(w, ex), w)
	fitBands(w, ex)
	return nil
}

// fitBands picks the Strong and Good cut-offs that best separate grade 3, and grade 2 or more,
// among graded jobs, and prints what each band holds.
func fitBands(w rank.Weights, ex []example) {
	type pt struct {
		score float64
		grade int
	}
	var pts []pt
	for _, e := range ex {
		for _, c := range e.cands {
			if g, ok := e.grades[c.ID]; ok {
				pts = append(pts, pt{w.Total(c.Features), g})
			}
		}
	}
	best := func(min int) (float64, float64) {
		var bt, ba float64
		for t := 0.0; t <= 1.0; t += 0.005 {
			right := 0
			for _, p := range pts {
				if (p.score >= t) == (p.grade >= min) {
					right++
				}
			}
			if a := float64(right) / float64(len(pts)); a > ba {
				bt, ba = t, a
			}
		}
		return bt, ba
	}
	strong, sa := best(3)
	good, ga := best(2)
	fmt.Printf("bands from %d graded jobs: strong >= %.3f (accuracy %.2f for grade 3), good >= %.3f (accuracy %.2f for grade >= 2)\n",
		len(pts), strong, sa, good, ga)
	for _, b := range []struct {
		name   string
		lo, hi float64
	}{{"strong", strong, 9}, {"good", good, strong}, {"stretch", -9, good}} {
		n, g := 0, map[int]int{}
		for _, p := range pts {
			if p.score >= b.lo && p.score < b.hi {
				n++
				g[p.grade]++
			}
		}
		fmt.Printf("  %-7s %3d jobs, grades 3/2/1/0: %d/%d/%d/%d\n", b.name, n, g[3], g[2], g[1], g[0])
	}
}

// bands prints what each band holds among D3's graded jobs, under the score-only rule of P3 and
// under the rule that also holds bands to required-skill coverage.
func bands(ctx context.Context, s *rank.Searcher, ps []loaded, gradesFile string) error {
	grades, err := loadGrades(gradesFile)
	if err != nil {
		return err
	}
	type tally struct{ n, g3, g2, g1, g0 int }
	old, now := map[string]*tally{}, map[string]*tally{}
	for _, b := range []string{"strong", "good", "stretch"} {
		old[b], now[b] = &tally{}, &tally{}
	}
	add := func(t *tally, g int) {
		t.n++
		switch g {
		case 3:
			t.g3++
		case 2:
			t.g2++
		case 1:
			t.g1++
		default:
			t.g0++
		}
	}
	cov := map[string]*tally{}
	b := rank.DefaultBands
	for _, p := range ps {
		cs, err := s.Search(ctx, p.Profile, rank.Hybrid, 1000)
		if err != nil {
			return err
		}
		for _, c := range cs {
			g, ok := grades[p.ID][c.ID]
			if !ok {
				continue
			}
			scoreOnly := b.Band(c.Score, 1, 1) // full coverage: only the score decides, as in P3
			add(old[scoreOnly], g)
			add(now[c.Band], g)
			bucket := "none read"
			switch {
			case len(c.Required) == 0:
			case c.RequiredCoverage < 0.25:
				bucket = "under 1/4"
			case c.RequiredCoverage < 0.5:
				bucket = "1/4 to 1/2"
			case c.RequiredCoverage <= 0.5:
				bucket = "exactly 1/2"
			default:
				bucket = "over 1/2"
			}
			if cov[bucket] == nil {
				cov[bucket] = &tally{}
			}
			add(cov[bucket], g)
		}
	}
	fmt.Println("by required-skill coverage:")
	for _, name := range []string{"none read", "under 1/4", "1/4 to 1/2", "exactly 1/2", "over 1/2"} {
		if t := cov[name]; t != nil {
			fmt.Printf("  %-11s %3d jobs, grades 3/2/1/0: %d/%d/%d/%d\n", name, t.n, t.g3, t.g2, t.g1, t.g0)
		}
	}
	for _, rule := range []struct {
		name string
		t    map[string]*tally
	}{{"score only (P3)", old}, {"score and required coverage", now}} {
		fmt.Println(rule.name + ":")
		for _, name := range []string{"strong", "good", "stretch"} {
			t := rule.t[name]
			fmt.Printf("  %-7s %3d jobs, grades 3/2/1/0: %d/%d/%d/%d\n", name, t.n, t.g3, t.g2, t.g1, t.g0)
		}
	}
	return nil
}
