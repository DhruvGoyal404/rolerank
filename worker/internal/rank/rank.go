// Package rank ranks jobs for a profile, deterministically, from stored features
// (ARCHITECTURE.md, "Search and ranking"). No LLM takes part. The same profile and corpus give
// the same order, and every score can be rebuilt from Features and Weights.
package rank

import (
	"math"
	"slices"
	"sort"
	"time"
)

// Profile is what a search runs on: the user's confirmed profile and preferences.
type Profile struct {
	Skills    []string  // canonical names from the taxonomy
	Titles    []string  // target title terms, lowercased (skills.TitleTerms)
	Level     string    // intern, entry, mid, senior
	Country   string    // ISO code of where the user can work on site, e.g. IN
	Remote    bool      // also show remote roles open to Country
	Embedding []float32 // unit vector of the profile text
}

// Levels a profile of each level may see. Unknown is always included: rules can't read a level
// from every posting (PHASE2.md, "Known v1 gaps"), and hiding those would hide real entry roles.
var window = map[string][]string{
	"intern": {"intern", "unknown"},
	"entry":  {"entry", "unknown"},
	"mid":    {"entry", "mid", "unknown"},
	"senior": {"mid", "senior", "unknown"},
}

// Levels returns the level window for a profile level; an unrecognised level sees everything.
func Levels(level string) []string {
	if w, ok := window[level]; ok {
		return w
	}
	return []string{"intern", "entry", "mid", "senior", "unknown"}
}

// Candidate is one job as the reranker sees it.
type Candidate struct {
	ID       int64
	Level    string
	Required []string
	Nice     []string
	Title    []string
	Onsite   bool // open on site in the profile's country (else it got in as remote)
	Posted   time.Time
	Cosine   float64
	VecRank  int // 1-based rank in the vector list, 0 if absent
	LexRank  int // 1-based rank in the lexical list, 0 if absent
}

// Features are the explainable numbers a score is built from, each in [0, 1].
type Features struct {
	RequiredCoverage float64
	NiceCoverage     float64
	Cosine           float64
	TitleOverlap     float64
	LevelFit         float64
	LocationFit      float64
	Freshness        float64
	Fusion           float64 // reciprocal rank fusion of the two retrievers, scaled to [0, 1]
}

// Weights are the reranker's linear weights, versioned with the eval report that fitted them.
type Weights struct {
	Version                                                       string
	Required, Nice, Cosine, Title, Level, Location, Fresh, Fusion float64
}

// Default weights, fitted on D3 by coordinate ascent (`rank fit`, 2026-11-05). Cross-validated
// nDCG@10 over held-out profiles is 0.799, against 0.710 for the unfitted v0 weights. Skill
// coverage and cosine get no weight of their own: fusion already carries both retrievers' ranks.
var Default = Weights{Version: "v1-d3-fit", Required: 0.177, Nice: 0.013, Cosine: 0.354, Title: 0.051, Level: 0.076,
	Location: 0.253, Fresh: 0.0, Fusion: 0.076}

// RRFK is the reciprocal rank fusion constant pgvector's README suggests.
const RRFK = 60

func overlap(a, b []string) int {
	n := 0
	for _, x := range a {
		if slices.Contains(b, x) {
			n++
		}
	}
	return n
}

// Score computes a candidate's features for a profile.
func Score(p Profile, c Candidate, now time.Time) Features {
	f := Features{Cosine: math.Max(0, c.Cosine)}
	if len(c.Required) > 0 {
		f.RequiredCoverage = float64(overlap(c.Required, p.Skills)) / float64(len(c.Required))
	}
	if len(c.Nice) > 0 {
		f.NiceCoverage = float64(overlap(c.Nice, p.Skills)) / float64(len(c.Nice))
	}
	if union := len(c.Title) + len(p.Titles) - overlap(c.Title, p.Titles); union > 0 {
		f.TitleOverlap = float64(overlap(c.Title, p.Titles)) / float64(union)
	}
	switch {
	case c.Level == p.Level:
		f.LevelFit = 1
	case c.Level == "unknown":
		f.LevelFit = 0.5
	default:
		f.LevelFit = 0.25 // inside the window, one step away
	}
	f.LocationFit = 0.8
	if c.Onsite {
		f.LocationFit = 1
	}
	if !c.Posted.IsZero() {
		f.Freshness = math.Exp(-now.Sub(c.Posted).Hours() / 24 / 30)
	}
	var rrf float64
	for _, r := range []int{c.VecRank, c.LexRank} {
		if r > 0 {
			rrf += 1 / float64(RRFK+r)
		}
	}
	f.Fusion = rrf / (2.0 / (RRFK + 1)) // first in both lists is 1
	return f
}

// Total is the weighted sum.
func (w Weights) Total(f Features) float64 {
	return w.Required*f.RequiredCoverage + w.Nice*f.NiceCoverage + w.Cosine*f.Cosine + w.Title*f.TitleOverlap +
		w.Level*f.LevelFit + w.Location*f.LocationFit + w.Fresh*f.Freshness + w.Fusion*f.Fusion
}

// Ranked is one result.
type Ranked struct {
	Candidate
	Features
	Score float64
	Band  string
}

// Bands map a score to Strong, Good or Stretch, with cut-offs fitted on D3 grades.
type Bands struct{ Strong, Good float64 }

// DefaultBands, fitted on D3 (2026-11-05): of 111 graded jobs called strong, 74 are grade 3 and
// none grade 0; of 113 called good, 80 are grade 2 or 3.
var DefaultBands = Bands{Strong: 0.505, Good: 0.46}

// Band names a score, held to the job's required skills (owner's walkthrough, 2026-11-08: a
// job showed Strong with 1 of 5 required skills). Strong also needs more than half of the
// required skills; three quarters or more missing is Stretch, whatever the score (on D3 these
// are the jobs least often graded a fit). A job with no required skills read from it can't be
// shown to fit strongly, so it stops at Good.
func (b Bands) Band(score, coverage float64, required int) string {
	band := "stretch"
	switch {
	case score >= b.Strong:
		band = "strong"
	case score >= b.Good:
		band = "good"
	}
	switch {
	case required > 0 && coverage < 0.25:
		return "stretch"
	case band == "strong" && (required == 0 || coverage <= 0.5):
		return "good"
	}
	return band
}

// Rerank orders candidates by score, ties broken by ID, so the order is reproducible.
func Rerank(p Profile, cs []Candidate, w Weights, b Bands, now time.Time) []Ranked {
	out := make([]Ranked, len(cs))
	for i, c := range cs {
		f := Score(p, c, now)
		s := w.Total(f)
		out[i] = Ranked{Candidate: c, Features: f, Score: s, Band: b.Band(s, f.RequiredCoverage, len(c.Required))}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Explain is the explanation that needs no LLM: matched and missing required skills.
func Explain(p Profile, c Candidate) (matched, missing []string) {
	for _, s := range c.Required {
		if slices.Contains(p.Skills, s) {
			matched = append(matched, s)
		} else {
			missing = append(missing, s)
		}
	}
	return matched, missing
}
