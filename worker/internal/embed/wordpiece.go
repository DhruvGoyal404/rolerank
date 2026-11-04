// Package embed turns text into the vectors search ranks by. This file is the BERT WordPiece
// tokenizer the model expects, in pure Go, checked token for token against the reference
// tokenizer (testdata/tokens.json).
package embed

import (
	"bufio"
	_ "embed"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// vocab.txt is BAAI/bge-small-en-v1.5's vocabulary (MIT licence), revision in model.go.
//
//go:embed vocab.txt
var vocabText string

// Tokenizer maps text to model input IDs.
type Tokenizer struct {
	ids                map[string]int64
	cls, sep, unk, pad int64
}

// NewTokenizer reads the embedded vocabulary.
func NewTokenizer() *Tokenizer {
	t := &Tokenizer{ids: map[string]int64{}}
	sc := bufio.NewScanner(strings.NewReader(vocabText))
	for n := int64(0); sc.Scan(); n++ {
		t.ids[sc.Text()] = n
	}
	t.cls, t.sep, t.unk, t.pad = t.ids["[CLS]"], t.ids["[SEP]"], t.ids["[UNK]"], t.ids["[PAD]"]
	return t
}

const maxWordChars = 100

// Encode returns [CLS] tokens [SEP], cut to at most max IDs.
func (t *Tokenizer) Encode(text string, max int) []int64 {
	out := []int64{t.cls}
	for _, word := range basic(text) {
		for _, id := range t.wordpiece(word) {
			if len(out) == max-1 {
				return append(out, t.sep)
			}
			out = append(out, id)
		}
	}
	return append(out, t.sep)
}

// basic is BERT's basic tokenizer: clean, space out CJK, lowercase, strip accents, split on
// whitespace and punctuation.
func basic(text string) []string {
	var b strings.Builder
	for _, r := range text {
		switch {
		case r == 0 || r == unicode.ReplacementChar || isControl(r):
			continue
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		case isCJK(r):
			b.WriteRune(' ')
			b.WriteRune(r)
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	var words []string
	for _, w := range strings.Fields(b.String()) {
		w = stripAccents(strings.ToLower(w))
		start := 0
		for i, r := range w {
			if isPunct(r) {
				if start < i {
					words = append(words, w[start:i])
				}
				words = append(words, string(r))
				start = i + len(string(r))
			}
		}
		if start < len(w) {
			words = append(words, w[start:])
		}
	}
	return words
}

func (t *Tokenizer) wordpiece(word string) []int64 {
	runes := []rune(word)
	if len(runes) > maxWordChars {
		return []int64{t.unk}
	}
	var out []int64
	for start := 0; start < len(runes); {
		end := len(runes)
		found := int64(-1)
		for ; end > start; end-- {
			piece := string(runes[start:end])
			if start > 0 {
				piece = "##" + piece
			}
			if id, ok := t.ids[piece]; ok {
				found = id
				break
			}
		}
		if found < 0 {
			return []int64{t.unk}
		}
		out = append(out, found)
		start = end
	}
	return out
}

func stripAccents(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isControl(r rune) bool {
	if r == '\t' || r == '\n' || r == '\r' {
		return false
	}
	return unicode.In(r, unicode.Cc, unicode.Cf)
}

func isPunct(r rune) bool {
	if (r >= 33 && r <= 47) || (r >= 58 && r <= 64) || (r >= 91 && r <= 96) || (r >= 123 && r <= 126) {
		return true
	}
	return unicode.IsPunct(r)
}

func isCJK(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0x3400 && r <= 0x4DBF) || (r >= 0x20000 && r <= 0x2A6DF) ||
		(r >= 0x2A700 && r <= 0x2B73F) || (r >= 0x2B740 && r <= 0x2B81F) || (r >= 0x2B820 && r <= 0x2CEAF) ||
		(r >= 0xF900 && r <= 0xFAFF) || (r >= 0x2F800 && r <= 0x2FA1F)
}
