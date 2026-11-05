package rank

import (
	"math"
	"slices"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)

func TestLevelWindowAlwaysKeepsUnknown(t *testing.T) {
	for _, l := range []string{"intern", "entry", "mid", "senior", "anything"} {
		if !slices.Contains(Levels(l), "unknown") {
			t.Errorf("%s hides unknown-level jobs", l)
		}
	}
	if slices.Contains(Levels("entry"), "senior") {
		t.Error("a fresher must not see senior-only jobs")
	}
}

func TestScoreFeatures(t *testing.T) {
	p := Profile{Skills: []string{"Go", "PostgreSQL"}, Titles: []string{"backend", "engineer"}, Level: "entry"}
	c := Candidate{Required: []string{"Go", "PostgreSQL", "Kubernetes", "AWS"}, Nice: []string{"Go"},
		Title: []string{"backend", "engineer", "ii"}, Level: "entry", Onsite: true, Posted: now.Add(-30 * 24 * time.Hour),
		Cosine: 0.8, VecRank: 1, LexRank: 1}
	f := Score(p, c, now)
	if f.RequiredCoverage != 0.5 || f.NiceCoverage != 1 || f.LevelFit != 1 || f.LocationFit != 1 || f.Fusion != 1 {
		t.Fatalf("%+v", f)
	}
	if math.Abs(f.TitleOverlap-2.0/3) > 1e-9 || math.Abs(f.Freshness-math.Exp(-1)) > 1e-9 {
		t.Fatalf("%+v", f)
	}
	if got := Score(p, Candidate{Level: "unknown"}, now); got.LevelFit != 0.5 || got.LocationFit != 0.8 || got.Fusion != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestRerankIsDeterministic(t *testing.T) {
	p := Profile{Skills: []string{"Go"}, Level: "mid"}
	cs := []Candidate{{ID: 3, Level: "mid"}, {ID: 1, Level: "mid"}, {ID: 2, Level: "mid", Required: []string{"Go"}}}
	got := Rerank(p, cs, Default, DefaultBands, now)
	if got[0].ID != 2 || got[1].ID != 1 || got[2].ID != 3 {
		t.Fatalf("order %d %d %d", got[0].ID, got[1].ID, got[2].ID)
	}
}

func TestExplainSplitsRequiredSkills(t *testing.T) {
	m, x := Explain(Profile{Skills: []string{"Go"}}, Candidate{Required: []string{"Go", "Kubernetes"}})
	if !slices.Equal(m, []string{"Go"}) || !slices.Equal(x, []string{"Kubernetes"}) {
		t.Fatal(m, x)
	}
}

func TestBands(t *testing.T) {
	if DefaultBands.Band(0.6) != "strong" || DefaultBands.Band(0.48) != "good" || DefaultBands.Band(0.1) != "stretch" {
		t.Fatal("bands")
	}
}

func TestEligibleWritesOnlyAnISOCode(t *testing.T) {
	if _, err := Eligible(Profile{Country: "IN') OR true --"}); err == nil {
		t.Fatal("a country that isn't an ISO code must be refused")
	}
	got, err := Eligible(Profile{Country: "IN", Remote: true})
	if err != nil || got != "level = ANY($1) AND ('IN' = ANY(countries) OR 'remote_global' = ANY(remote_scopes))" {
		t.Fatalf("%q %v", got, err)
	}
}
