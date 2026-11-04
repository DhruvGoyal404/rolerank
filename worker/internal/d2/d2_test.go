package d2

import (
	"reflect"
	"slices"
	"testing"

	"github.com/DhruvGoyal404/rolerank/worker/internal/skills"
)

func TestGenerateIsSeeded(t *testing.T) {
	p1, j1 := Generate(7, 60, 400)
	p2, j2 := Generate(7, 60, 400)
	if !reflect.DeepEqual(p1, p2) || !reflect.DeepEqual(j1, j2) {
		t.Fatal("same seed, different set")
	}
}

func TestEverySkillIsInTheTaxonomy(t *testing.T) {
	names := skills.Default.Names()
	for f, list := range fields {
		for _, s := range list {
			if !slices.Contains(names, s) {
				t.Errorf("%s: %q isn't a canonical skill", f, s)
			}
		}
	}
}

func TestTheMatcherReadsTheGeneratedJobs(t *testing.T) {
	_, js := Generate(7, 0, 50)
	for _, j := range js {
		req, _ := skills.Default.Extract(j.Body)
		for _, s := range j.Required {
			if !slices.Contains(req, s) {
				t.Errorf("%s: %q not extracted as required (%v)", j.ID, s, req)
			}
		}
	}
}

func TestGrade(t *testing.T) {
	p := Profile{Field: "backend", Level: "entry", Skills: []string{"Go", "PostgreSQL"}}
	job := func(place, level string, req ...string) Job {
		return Job{Field: "backend", Level: level, Place: place, Required: req}
	}
	tests := []struct {
		j    Job
		want int
	}{
		{job("IN", "entry", "Go", "PostgreSQL", "Redis"), 3},
		{job("IN", "entry", "Go", "Redis", "Kafka"), 2},
		{job("IN", "entry", "Redis", "Kafka", "Java", "gRPC"), 1},
		{job("IN", "senior", "Go"), 1},
		{job("US", "entry", "Go"), 0},
		{job("remote", "entry", "Go"), 0}, // this profile doesn't accept remote
		{Job{Field: "ml", Level: "entry", Place: "IN", Required: []string{"Go"}}, 0},
	}
	for i, tt := range tests {
		if got := Grade(p, tt.j); got != tt.want {
			t.Errorf("case %d: got %d, want %d", i, got, tt.want)
		}
	}
}
