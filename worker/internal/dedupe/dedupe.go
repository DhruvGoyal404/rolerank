// Package dedupe decides whether two postings are the same job. Precision comes first: a
// wrong merge hides a real job from someone, and a missed merge only shows it twice. The
// thresholds here are measured by the D5 eval.
package dedupe

import (
	"encoding/binary"
	"hash/fnv"
	"regexp"
	"sort"
	"strings"
)

const (
	permutations = 64
	shingleWords = 5
	// MinWords is the shortest text worth comparing. Below it, similarity means little.
	MinWords = 30
	// Threshold is the estimated Jaccard similarity needed to merge.
	Threshold = 0.95
)

var (
	nonWord  = regexp.MustCompile(`[^\pL\pN]+`)
	suffixes = regexp.MustCompile(`\b(inc|llc|ltd|limited|pvt|private|corp|corporation|co|company|gmbh|technologies|technology|labs|hq)\b`)
	noise    = regexp.MustCompile(`\b(remote|hybrid|onsite|on site|urgent|hiring|immediate joiner)\b`)
	// Places a title may carry as a suffix. A place never makes a different job.
	places = regexp.MustCompile(`\b(india|usa|us|uk|united states|united kingdom|canada|mexico|brazil|argentina|colombia|chile|peru|germany|france|spain|portugal|poland|ireland|netherlands|singapore|japan|australia|europe|emea|apac|latam|latin america|north america|americas|bengaluru|bangalore|hyderabad|pune|mumbai|gurgaon|gurugram|noida|chennai|delhi|kochi|ahmedabad|london|berlin|new york|san francisco|seattle|austin|boston|atlanta|chicago|toronto|dublin|amsterdam|paris|tokyo|sydney|georgia)\b`)
	// A trailing " - City, Region" and requisition numbers are cosmetic too.
	placeTail   = regexp.MustCompile(`\s+[-–]\s+[^-–,]+,[^-–]+$`)
	requisition = regexp.MustCompile(`^r?\d{3,}$`)
	cosmetic    = strings.NewReplacer("back end", "backend", "front end", "frontend", "full stack", "fullstack", "sr ", "senior ", "jr ", "junior ")
)

func normal(s string) string {
	return strings.Join(strings.Fields(nonWord.ReplaceAllString(strings.ToLower(s), " ")), " ")
}

// Key groups postings that could be the same job: one company, one title.
func Key(company, title, location string) string {
	c := normal(suffixes.ReplaceAllString(normal(company), " "))
	t := cosmetic.Replace(normal(placeTail.ReplaceAllString(title, "")) + " ")
	t = noise.ReplaceAllString(places.ReplaceAllString(t, " "), " ")
	here := map[string]bool{}
	for _, w := range strings.Fields(normal(location)) {
		here[w] = true
	}
	var words []string
	for _, w := range strings.Fields(t) {
		if !here[w] && !requisition.MatchString(w) {
			words = append(words, w)
		}
	}
	sort.Strings(words) // word order is cosmetic
	return c + "|" + strings.Join(words, " ")
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

var (
	years     = regexp.MustCompile(`(?i)\b\d{1,2}\s*(?:\+|-|–|to)?\s*\d{0,2}\s*\+?\s*(?:years|yrs)\b`)
	pay       = regexp.MustCompile(`[$€£₹]\s?\d[\d,.]*[kK]?`)
	shift     = regexp.MustCompile(`(?i)\b(?:mon|tues|wednes|thurs|fri|satur|sun)day\s*(?:-|–|to|through)\s*(?:mon|tues|wednes|thurs|fri|satur|sun)day\b|\b\d{1,2}(?::\d\d)?\s*[ap]\.?m\.?\s*(?:-|–|to)\s*\d{1,2}(?::\d\d)?\s*[ap]\.?m\b`)
	clearance = regexp.MustCompile(`(?i)\b(?:active|current|obtain|not required)\b[^.\n]{0,40}\bclearance\b|\bclearance\b[^.\n]{0,40}\bnot required\b|\bts/sci\b|\bpolygraph\b|\bpoly\b|\bcitizenship\b`)
	language  = regexp.MustCompile(`(?i)\b(?:mandarin|cantonese|japanese|korean|german|french|spanish|portuguese|italian|dutch|hebrew|arabic|hindi|turkish|polish|russian|nordic|swedish|danish|norwegian|finnish)\b`)
)

func set(re *regexp.Regexp, body string) string {
	found := map[string]bool{}
	for _, m := range re.FindAllString(body, -1) {
		found[strings.Join(strings.Fields(strings.ToLower(m)), " ")] = true
	}
	list := make([]string, 0, len(found))
	for m := range found {
		list = append(list, m)
	}
	sort.Strings(list)
	return strings.Join(list, ";")
}

// Facts are the things a posting can state that make it a different job even when the
// text is otherwise the same.
type Facts struct{ Years, Pay, Shift, Clearance, Language string }

// Read extracts a posting's facts.
func Read(body string) Facts {
	return Facts{set(years, body), set(pay, body), set(shift, body), set(clearance, body), set(language, body)}
}

// Posting is what a merge decision looks at.
type Posting struct {
	Location  string
	Signature []byte
	Facts     Facts
}

// SameJob decides a merge: same text, and no stated difference in years, shift, clearance,
// language or pay. A place never splits a job, because the cluster keeps every member's
// location. Pay is compared only within one location, as pay bands vary by metro and
// currency. A fact one side leaves out doesn't count, except clearance and language.
func SameJob(a, b Posting) bool {
	stated := func(x, y string) bool { return x != "" && y != "" && x != y }
	if stated(a.Facts.Years, b.Facts.Years) || stated(a.Facts.Shift, b.Facts.Shift) ||
		a.Facts.Clearance != b.Facts.Clearance || a.Facts.Language != b.Facts.Language {
		return false
	}
	if a.Location == b.Location && stated(a.Facts.Pay, b.Facts.Pay) {
		return false
	}
	return Similarity(a.Signature, b.Signature) >= Threshold
}
