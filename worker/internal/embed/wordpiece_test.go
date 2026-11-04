package embed

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"slices"
	"testing"
)

func TestVocabularyIsThePinnedOne(t *testing.T) {
	sum := sha256.Sum256([]byte(vocabText))
	if got := hex.EncodeToString(sum[:]); got != VocabSHA256 {
		t.Fatalf("vocab.txt is %s, want %s", got, VocabSHA256)
	}
}

// testdata/tokens.json comes from the reference tokenizer (Hugging Face tokenizers 0.22.1)
// loading the pinned tokenizer.json, on synthetic text only.
func TestEncodeMatchesTheReferenceTokenizer(t *testing.T) {
	raw, err := os.ReadFile("testdata/tokens.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Text string  `json:"text"`
		IDs  []int64 `json:"ids"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	tok := NewTokenizer()
	for _, c := range cases {
		if got := tok.Encode(c.Text, 512); !slices.Equal(got, c.IDs) {
			t.Errorf("%q:\n got %v\nwant %v", c.Text, got, c.IDs)
		}
	}
}

func TestEncodeCutsAtTheLimit(t *testing.T) {
	tok := NewTokenizer()
	got := tok.Encode("one two three four five six", 4)
	if len(got) != 4 || got[0] != tok.cls || got[3] != tok.sep {
		t.Fatalf("got %v", got)
	}
}
