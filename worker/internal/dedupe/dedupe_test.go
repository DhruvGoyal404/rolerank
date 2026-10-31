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
	}
	for _, pair := range same {
		if Key(pair[0][0], pair[0][1]) != Key(pair[1][0], pair[1][1]) {
			t.Errorf("%v should share a key", pair)
		}
	}
	if Key("Acme", "Backend Engineer") == Key("Acme", "Frontend Engineer") {
		t.Error("different titles must differ")
	}
	if Key("Acme", "Engineer") == Key("Globex", "Engineer") {
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
	tests := []struct {
		name string
		aLoc string
		aAgg bool
		bLoc string
		bAgg bool
		a, b []byte
		want bool
	}{
		{"same city", "pune", false, "pune", false, sig, sig, true},
		{"same text, two cities stays two jobs", "pune", false, "london", false, sig, sig, false},
		{"an aggregator matches an ATS city", "india", true, "pune", false, sig, sig, true},
		{"same city, different text", "pune", false, "pune", false, sig, other, false},
		{"no text", "pune", false, "pune", false, nil, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SameJob(tt.aLoc, tt.aAgg, tt.a, tt.bLoc, tt.bAgg, tt.b); got != tt.want {
				t.Fatalf("got %v", got)
			}
		})
	}
}
