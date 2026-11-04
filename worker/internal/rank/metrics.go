package rank

import (
	"math"
	"sort"
)

// Metrics are EVALS.md's matching metrics for one profile. Grades are 0 to 3; a job without a
// grade is skipped (condensed lists), not counted as irrelevant.
type Metrics struct {
	NDCG10       float64
	RR           float64 // reciprocal rank of the first grade 3; -1 when the profile has none
	Recall       float64 // share of grade >= 2 jobs the pipeline returned at all; -1 when none exist
	LevelViolate int     // senior-only jobs in the top 10 of an intern or entry profile
	GeoViolate   int     // jobs not open to the profile in the top 10
	Judged10     float64 // share of the top 10 that has a grade
}

// Measure scores one ranked list. grades holds every graded job for the profile; open says
// whether a job is open to the profile; senior says whether a job is senior-only.
func Measure(level string, ranked []int64, grades map[int64]int, open, senior func(int64) bool) Metrics {
	var m Metrics
	gain := func(g int) float64 { return math.Pow(2, float64(g)) - 1 }
	var dcg float64
	pos, judged := 0, 0
	for i, id := range ranked {
		g, ok := grades[id]
		if i < 10 {
			if ok {
				judged++
			}
			if !open(id) {
				m.GeoViolate++
			}
			if (level == "intern" || level == "entry") && senior(id) {
				m.LevelViolate++
			}
		}
		if !ok {
			continue
		}
		if pos < 10 {
			dcg += gain(g) / math.Log2(float64(pos)+2)
		}
		pos++
	}
	if top := min(10, len(ranked)); top > 0 {
		m.Judged10 = float64(judged) / float64(top)
	}
	var all []int
	for _, g := range grades {
		all = append(all, g)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(all)))
	var ideal float64
	for i, g := range all[:min(10, len(all))] {
		ideal += gain(g) / math.Log2(float64(i)+2)
	}
	if ideal > 0 {
		m.NDCG10 = dcg / ideal
	}
	m.RR = -1
	for _, g := range grades {
		if g == 3 {
			m.RR = 0
			break
		}
	}
	if m.RR == 0 {
		for i, id := range ranked {
			if grades[id] == 3 {
				m.RR = 1 / float64(i+1)
				break
			}
		}
	}
	want, got := 0, 0
	returned := map[int64]bool{}
	for _, id := range ranked {
		returned[id] = true
	}
	for id, g := range grades {
		if g >= 2 {
			want++
			if returned[id] {
				got++
			}
		}
	}
	m.Recall = -1
	if want > 0 {
		m.Recall = float64(got) / float64(want)
	}
	return m
}

// Summary averages per-profile metrics.
type Summary struct {
	Profiles                   int
	NDCG10, MRR, Recall        float64
	LevelViolation, GeoViolate float64
	Judged10                   float64
}

// Summarise averages per-profile metrics. MRR and recall average only over profiles where they
// are defined. Violations are rates: slots10 counts every top-10 slot filled, earlySlots only
// those of intern and entry profiles.
func Summarise(ms []Metrics, slots10, earlySlots int) Summary {
	s := Summary{Profiles: len(ms)}
	var rr, rc, lv, gv int
	for _, m := range ms {
		s.NDCG10 += m.NDCG10
		s.Judged10 += m.Judged10
		if m.RR >= 0 {
			s.MRR += m.RR
			rr++
		}
		if m.Recall >= 0 {
			s.Recall += m.Recall
			rc++
		}
		lv += m.LevelViolate
		gv += m.GeoViolate
	}
	if n := float64(len(ms)); n > 0 {
		s.NDCG10 /= n
		s.Judged10 /= n
	}
	if rr > 0 {
		s.MRR /= float64(rr)
	}
	if rc > 0 {
		s.Recall /= float64(rc)
	}
	if earlySlots > 0 {
		s.LevelViolation = float64(lv) / float64(earlySlots)
	}
	if slots10 > 0 {
		s.GeoViolate = float64(gv) / float64(slots10)
	}
	return s
}
