// Package dedupe decides whether two postings are the same job. Precision comes first: a
// wrong merge hides a real job from someone, and a missed merge only shows it twice. The
// thresholds here are measured by the D5 eval.
package dedupe

import (
	"encoding/binary"
	"hash/fnv"
	"regexp"
	"strings"
)

const (
	permutations = 64
	shingleWords = 5
	// MinWords is the shortest text worth comparing. Below it, similarity means little.
	MinWords = 30
	// Threshold is the estimated Jaccard similarity needed to merge.
	Threshold = 0.9
)

var (
	nonWord  = regexp.MustCompile(`[^\pL\pN]+`)
	suffixes = regexp.MustCompile(`\b(inc|llc|ltd|limited|pvt|private|corp|corporation|co|company|gmbh|technologies|technology|labs|hq)\b`)
	noise    = regexp.MustCompile(`\b(remote|hybrid|onsite|on site|urgent|hiring|immediate joiner)\b`)
	bracket  = regexp.MustCompile(`[(\[].*?[)\]]`)
)

func normal(s string) string {
	return strings.Join(strings.Fields(nonWord.ReplaceAllString(strings.ToLower(s), " ")), " ")
}

// Key groups postings that could be the same job: one company, one title.
func Key(company, title string) string {
	c := normal(suffixes.ReplaceAllString(normal(company), " "))
	t := normal(noise.ReplaceAllString(normal(bracket.ReplaceAllString(title, " ")), " "))
	return c + "|" + t
}

// Location is compared as text. An aggregator's location list won't match an ATS's city,
// which is why the caller treats aggregators specially.
func Location(raw string) string {
	return normal(raw)
}

// Signature is a MinHash of the text's 5-word shingles, or nil if the text is too short
// to compare.
func Signature(body string) []byte {
	words := strings.Fields(normal(body))
	if len(words) < MinWords {
		return nil
	}
	mins := make([]uint64, permutations)
	for i := range mins {
		mins[i] = ^uint64(0)
	}
	for i := 0; i+shingleWords <= len(words); i++ {
		h := fnv.New64a()
		for _, w := range words[i : i+shingleWords] {
			_, _ = h.Write([]byte(w))
			_, _ = h.Write([]byte{0})
		}
		value := h.Sum64()
		for p := range mins {
			if hashed := mix(value, uint64(p)); hashed < mins[p] {
				mins[p] = hashed
			}
		}
	}
	out := make([]byte, permutations*8)
	for i, m := range mins {
		binary.LittleEndian.PutUint64(out[i*8:], m)
	}
	return out
}

// mix is the SplitMix64 finaliser, seeded per permutation.
func mix(value, seed uint64) uint64 {
	z := value + (seed+1)*0x9e3779b97f4a7c15
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// Similarity estimates the Jaccard similarity of two signatures. Anything missing or
// malformed scores 0, so it can never merge.
func Similarity(a, b []byte) float64 {
	if len(a) != permutations*8 || len(b) != permutations*8 {
		return 0
	}
	same := 0
	for i := 0; i < len(a); i += 8 {
		if binary.LittleEndian.Uint64(a[i:]) == binary.LittleEndian.Uint64(b[i:]) {
			same++
		}
	}
	return float64(same) / permutations
}

// IsAggregator reports a source that re-lists jobs from elsewhere.
func IsAggregator(source string) bool {
	return source == "himalayas" || source == "remoteok"
}

// SameJob decides a merge. The same role in two cities stays two jobs, because a fresher
// in Pune mustn't lose a Pune posting to a London one. An aggregator states regions, not
// cities, so it can match any location.
func SameJob(aLocation string, aAggregator bool, aSig []byte, bLocation string, bAggregator bool, bSig []byte) bool {
	if !aAggregator && !bAggregator && aLocation != bLocation {
		return false
	}
	return Similarity(aSig, bSig) >= Threshold
}
