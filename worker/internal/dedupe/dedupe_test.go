package dedupe

import (
	"fmt"
	"strings"
	"testing"
)

func text(seed string, words int) string {
	var b strings.Builder
	for i := range words {
		fmt.Fprintf(&b, "%s%d ", seed, i)
	}
	return b.String()
}

func TestKey(t *testing.T) {
	same := [][2][2]string{
		{{"Acme Inc.", "Software Engineer"}, {"ACME", "software engineer"}},
		{{"Acme Technologies Pvt Ltd", "SDE-1"}, {"acme", "SDE 1"}},
		{{"Acme", "Backend Engineer (Remote)"}, {"Acme", "Backend Engineer"}},
		{{"Acme", "Backend Engineer - Remote"}, {"Acme", "Backend Engineer"}},
		{{"Acme", "Node Engineer - Latin America, Remote"}, {"Acme", "Node Engineer - Remote, Latin America"}},
		{{"Acme", "Sr. Back-end Engineer"}, {"Acme", "Senior Backend Engineer"}},
		{{"Acme", "Data Engineer - India"}, {"Acme", "Data Engineer"}},
		{{"Acme", "Data Engineer - Kochi"}, {"Acme", "Data Engineer"}}, // Kochi is the location below
	}
	for _, pair := range same {
		if Key(pair[0][0], pair[0][1], "Kochi") != Key(pair[1][0], pair[1][1], "Kochi") {
			t.Errorf("%v should share a key", pair)
		}
	}
	different := [][2]string{
		{"Backend Engineer", "Frontend Engineer"},
		{"Software Developer in Test (C#)", "Software Developer in Test (Python)"},
		{"CAD Intern (Spring 2027)", "CAD Intern (Summer 2027)"},
		{"Senior Engineer", "Junior Engineer"},
		{"Engineer", "Engineer (Active Clearance)"},
	}
	for _, pair := range different {
		if Key("Acme", pair[0], "") == Key("Acme", pair[1], "") {
			t.Errorf("%v must differ", pair)
		}
	}
	if Key("Acme", "Engineer", "") == Key("Globex", "Engineer", "") {
		t.Error("different companies must differ")
	}
}

func TestSimilarityOnKnownOverlaps(t *testing.T) {
	base := text("w", 400)
	if got := Similarity(Signature(base), Signature(base)); got != 1 {
		t.Fatalf("identical text scored %v", got)
	}
	if got := Similarity(Signature(base), Signature(text("other", 400))); got > 0.1 {
		t.Fatalf("unrelated text scored %v", got)
	}
	nearly := base + text("tail", 4)
	if got := Similarity(Signature(base), Signature(nearly)); got < Threshold {
		t.Fatalf("a small edit scored %v", got)
	}
	half := text("w", 200) + text("other", 200)
	if got := Similarity(Signature(base), Signature(half)); got >= Threshold {
		t.Fatalf("half-different text scored %v, which would merge", got)
	}
}

func TestSignatureIgnoresCaseAndPunctuation(t *testing.T) {
	a := Signature(text("w", 100))
	b := Signature(strings.ToUpper(strings.ReplaceAll(text("w", 100), " ", ", ")))
	if Similarity(a, b) != 1 {
		t.Fatalf("scored %v", Similarity(a, b))
	}
}

func TestShortTextNeverMerges(t *testing.T) {
	if Signature("Apply now") != nil {
		t.Fatal("too short to sign")
	}
	if Signature("") != nil {
		t.Fatal("empty text can't be signed")
	}
	if Similarity(nil, nil) != 0 {
		t.Fatal("two short postings must not look identical")
	}
	if Similarity([]byte{1, 2}, []byte{1, 2}) != 0 {
		t.Fatal("malformed signatures must not merge")
	}
}

func TestSameJob(t *testing.T) {
	sig := Signature(text("w", 200))
	other := Signature(text("x", 200))
	job := func(location, body string, signature []byte) Posting {
		return Posting{Location: location, Signature: signature, Facts: Read(body)}
	}
	tests := []struct {
		name string
		a, b Posting
		want bool
	}{
		{"same text, same city", job("pune", "", sig), job("pune", "", sig), true},
		{"a place never splits a job", job("pune", "", sig), job("london", "", sig), true},
		{"different text", job("pune", "", sig), job("pune", "", other), false},
		{"no text", job("pune", "", nil), job("pune", "", nil), false},
		{"years", job("x", "5+ years of Go", sig), job("x", "8+ years of Go", sig), false},
		{"years left out of one", job("x", "5+ years of Go", sig), job("x", "Go", sig), true},
		{"shift", job("x", "Sunday - Thursday", sig), job("x", "Tuesday - Saturday", sig), false},
		{"clearance", job("x", "Active Secret clearance", sig), job("x", "", sig), false},
		{"language", job("x", "fluent Japanese", sig), job("x", "", sig), false},
		{"pay in one city", job("x", "$100,000", sig), job("x", "$120,000", sig), false},
		{"pay across cities", job("x", "$100,000", sig), job("y", "$120,000", sig), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SameJob(tt.a, tt.b); got != tt.want {
				t.Fatalf("got %v", got)
			}
		})
	}
}
