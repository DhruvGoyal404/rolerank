// Package skills finds canonical skills in job text with a deterministic matcher over our own
// taxonomy (ARCHITECTURE.md, "Skills"). It splits them into required and nice to have by the
// section a line sits in.
package skills

import (
	_ "embed"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

//go:embed taxonomy.txt
var taxonomyText string

// Taxonomy maps token sequences to canonical skill names.
type Taxonomy struct {
	insensitive map[string]string // lowercased tokens joined by spaces
	sensitive   map[string]string // tokens in their own case
	longest     int
	names       []string
}

var token = regexp.MustCompile(`[A-Za-z0-9+#.]+|[/-]`)

func tokens(s string) []string {
	words, _ := tokenSpans(s)
	return words
}

// tokenSpans returns the tokens and where each sits in s.
func tokenSpans(s string) ([]string, [][2]int) {
	var words []string
	var spans [][2]int
	for _, at := range token.FindAllStringIndex(s, -1) {
		t := strings.TrimRight(s[at[0]:at[1]], ".") // a sentence's full stop isn't part of "Node.js."
		if t != "" {
			words = append(words, t)
			spans = append(spans, [2]int{at[0], at[0] + len(t)})
		}
	}
	return words, spans
}

// Parse reads the taxonomy format described at the top of taxonomy.txt.
func Parse(text string) (*Taxonomy, error) {
	t := &Taxonomy{insensitive: map[string]string{}, sensitive: map[string]string{}}
	seen := map[string]bool{}
	for n, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) != 3 {
			return nil, fmt.Errorf("taxonomy line %d: want 3 fields", n+1)
		}
		name := strings.TrimSpace(parts[0])
		forms := []string{name}
		for _, a := range strings.Split(parts[2], ",") {
			if a = strings.TrimSpace(a); a != "" {
				forms = append(forms, a)
			}
		}
		name = strings.TrimPrefix(name, "=")
		if seen[name] {
			return nil, fmt.Errorf("taxonomy line %d: %q listed twice", n+1, name)
		}
		seen[name] = true
		t.names = append(t.names, name)
		for _, f := range forms {
			exact := strings.HasPrefix(f, "=")
			words := tokens(strings.TrimPrefix(f, "="))
			if len(words) == 0 {
				return nil, fmt.Errorf("taxonomy line %d: empty form", n+1)
			}
			t.longest = max(t.longest, len(words))
			if exact {
				t.sensitive[strings.Join(words, " ")] = name
				continue
			}
			key := strings.ToLower(strings.Join(words, " "))
			if other, ok := t.insensitive[key]; ok && other != name {
				return nil, fmt.Errorf("taxonomy line %d: %q means both %q and %q", n+1, key, other, name)
			}
			t.insensitive[key] = name
		}
	}
	return t, nil
}

// Default is the embedded taxonomy. It panics at start-up if the file is malformed, which a
// test catches first.
var Default = func() *Taxonomy {
	t, err := Parse(taxonomyText)
	if err != nil {
		panic(err)
	}
	return t
}()

// Names lists every canonical skill.
func (t *Taxonomy) Names() []string { return t.names }

// Find returns the canonical skills in text, in order of first appearance, longest match first.
func (t *Taxonomy) Find(text string) []string {
	words, spans := tokenSpans(text)
	var out []string
	seen := map[string]bool{}
	for i := 0; i < len(words); {
		matched := 0
		for n := min(t.longest, len(words)-i); n > 0 && matched == 0; n-- {
			key := strings.Join(words[i:i+n], " ")
			name, ok := t.sensitive[key]
			// One letter is a skill (C, R) only standing alone: not "(C)", "P&C", "C-level" or "Series C".
			if ok && len(key) == 1 && !alone(text, spans[i][0], spans[i][1], i, words) {
				ok = false
			}
			if !ok {
				name, ok = t.insensitive[strings.ToLower(key)]
			}
			if ok {
				matched = n
				if !seen[name] {
					seen[name] = true
					out = append(out, name)
				}
			}
		}
		i += max(matched, 1)
	}
	return out
}

func alone(text string, start, end, i int, words []string) bool {
	if start > 0 && strings.ContainsRune("(&-'’", rune(text[start-1])) {
		return false
	}
	if end < len(text) && strings.ContainsRune(")&-'’", rune(text[end])) {
		return false
	}
	if i > 0 && (strings.EqualFold(words[i-1], "series") || strings.EqualFold(words[i-1], "grade") || strings.EqualFold(words[i-1], "level")) {
		return false
	}
	return true
}

var (
	niceMark = regexp.MustCompile(`(?i)\b(nice[- ]to[- ]haves?|preferred|preferably|bonus|good[- ]to[- ]have|a plus|plus points?|desired|desirable|ideally|extra credit|not required)\b`)
	required = regexp.MustCompile(`(?i)\b(requirements?|qualifications?|must[- ]haves?|what you('|’)?ll need|what we('|’)?re looking for|what you bring|about you|you have|you will have|skills|experience|responsibilities|what you('|’)?ll do)\b`)
)

// Extract splits a posting's skills into required and nice to have. A heading line that marks
// a nice-to-have section moves the lines under it there until a required heading; a nice marker
// inside a line moves that line only. A skill seen in both is required.
func (t *Taxonomy) Extract(body string) (req, nice []string) {
	inNice := false
	isReq, isNice := map[string]bool{}, map[string]bool{}
	for _, line := range strings.Split(body, "\n") {
		heading := len(strings.TrimSpace(line)) <= 80
		switch {
		case heading && niceMark.MatchString(line) && len(t.Find(line)) == 0:
			inNice = true
			continue
		case heading && required.MatchString(line) && !niceMark.MatchString(line) && len(t.Find(line)) == 0:
			inNice = false
			continue
		}
		nice := inNice || niceMark.MatchString(line)
		for _, s := range t.Find(line) {
			if nice {
				isNice[s] = true
			} else {
				isReq[s] = true
			}
		}
	}
	for s := range isReq {
		req = append(req, s)
	}
	for s := range isNice {
		if !isReq[s] {
			nice = append(nice, s)
		}
	}
	sort.Strings(req)
	sort.Strings(nice)
	return req, nice
}

var stop = map[string]bool{"and": true, "or": true, "the": true, "a": true, "an": true, "of": true, "for": true,
	"in": true, "at": true, "to": true, "with": true, "on": true, "&": true, "/": true, "-": true}

// TitleTerms are a title's lowercased words without stop words, for title overlap in ranking.
func TitleTerms(title string) []string {
	var out []string
	seen := map[string]bool{}
	for _, w := range tokens(title) {
		w = strings.ToLower(w)
		if !stop[w] && !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}
