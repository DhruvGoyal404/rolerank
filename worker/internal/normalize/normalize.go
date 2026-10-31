// Package normalize turns a source's raw posting into the fields we store and search.
// Everything here is deterministic. An LLM per posting is the wrong cost and latency trade
// (ARCHITECTURE.md). The rules are measured by the D6 eval, not assumed to be right.
package normalize

import (
	"crypto/sha256"
	"html"
	"regexp"
	"strings"
	"unicode/utf8"

	nethtml "golang.org/x/net/html"

	"github.com/DhruvGoyal404/rolerank/worker/internal/source"
)

// Limits match the corpus schema.
const (
	MaxBody    = 20 * 1024
	MaxExcerpt = 280
)

// Levels. "unknown" stays honest: a guess here would hide a role from a fresher.
const (
	LevelIntern  = "intern"
	LevelEntry   = "entry"
	LevelMid     = "mid"
	LevelSenior  = "senior"
	LevelUnknown = "unknown"
)

// Remote scopes, matching the schema's check constraint.
const (
	ScopeOnsite     = "onsite"
	ScopeHybrid     = "hybrid"
	ScopeGlobal     = "remote_global"
	ScopeRestricted = "remote_restricted"
	ScopeUnknown    = "unknown"
)

// Fields are what normalisation adds to a posting.
type Fields struct {
	Body          string
	Excerpt       string
	Level         string
	RemoteScope   string
	Countries     []string
	IsEngineering bool
	ContentHash   []byte
}

// Text reduces HTML to plain text. Scripts and styles are dropped, and raw HTML is never
// stored, so a posting can't carry markup into the product.
func Text(raw string) string {
	// Greenhouse sends HTML escaped once, so it must be unescaped before tokenising.
	if strings.Contains(raw, "&lt;") {
		raw = html.UnescapeString(raw)
	}
	var b strings.Builder
	tokens := nethtml.NewTokenizer(strings.NewReader(raw))
	skip := 0
	for {
		switch tokens.Next() {
		case nethtml.ErrorToken:
			return collapse(b.String())
		case nethtml.TextToken:
			if skip == 0 {
				b.Write(tokens.Text())
			}
		case nethtml.StartTagToken, nethtml.SelfClosingTagToken:
			name, _ := tokens.TagName()
			switch string(name) {
			case "script", "style":
				skip++
			case "br", "p", "li", "div", "h1", "h2", "h3", "h4", "h5", "h6", "tr", "ul", "ol":
				b.WriteByte('\n')
			}
		case nethtml.EndTagToken:
			name, _ := tokens.TagName()
			switch string(name) {
			case "script", "style":
				if skip > 0 {
					skip--
				}
			case "p", "li", "div", "h1", "h2", "h3", "h4", "h5", "h6", "tr", "ul", "ol":
				b.WriteByte('\n')
			}
		}
	}
}

var (
	spaces = regexp.MustCompile(`[ \t\x{00a0}\r]+`)
	blanks = regexp.MustCompile(`\n\s*\n+`)
)

func collapse(s string) string {
	s = spaces.ReplaceAllString(s, " ")
	s = blanks.ReplaceAllString(s, "\n")
	s = strings.ReplaceAll(s, "\n ", "\n")
	s = strings.ReplaceAll(s, "\x00", "")
	return strings.TrimSpace(s)
}

// clip cuts to at most max bytes on a rune boundary.
func clip(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

func excerpt(body string) string {
	flat := strings.ReplaceAll(body, "\n", " ")
	if len(flat) <= MaxExcerpt {
		return flat
	}
	cut := clip(flat, MaxExcerpt)
	if i := strings.LastIndexByte(cut, ' '); i > MaxExcerpt/2 {
		cut = cut[:i]
	}
	return cut + "..."
}

var (
	engineering = regexp.MustCompile(`(?i)\b(engineer|engineering|developer|programmer|sde|sdet|sre|devops|devsecops|data scientist|data analyst|machine learning|ml engineer|full[- ]?stack|back[- ]?end|front[- ]?end|software|android|ios developer|qa automation|mlops)\b`)
	notEngineer = regexp.MustCompile(`(?i)\b(sales engineer|account (executive|manager)|recruiter|talent acquisition|civil engineer|mechanical engineer|chemical engineer|electrical engineer|field service engineer|sales development)\b`)
	intern      = regexp.MustCompile(`(?i)\b(intern|internship|interns|apprentice|apprenticeship|trainee|co-?op)\b`)
	entry       = regexp.MustCompile(`(?i)\b(new grad|new graduate|graduate|grad|fresher|freshers|entry[- ]level|early career|junior|jr\.?|campus|associate (software|engineer|developer)|engineer i|developer i|sde[- ]?1|sde[- ]?i|software engineer 1)\b`)
	senior      = regexp.MustCompile(`(?i)\b(senior|sr\.?|staff|principal|lead|manager|director|head of|vice president|vp|architect|distinguished|fellow|engineer iii|engineer iv|sde[- ]?3|sde[- ]?iii)\b`)
	mid         = regexp.MustCompile(`(?i)\b(mid[- ]?level|engineer ii|developer ii|sde[- ]?2|sde[- ]?ii)\b`)
	years       = regexp.MustCompile(`(?i)\b(\d{1,2})\s*(?:\+|-\s*\d{1,2}|to\s*\d{1,2})?\s*\+?\s*(?:years?|yrs?)\b[^.\n]{0,24}\b(?:experience|exp)\b`)
)

// Level reads the title first, because titles are what employers set deliberately. A
// stated minimum in the body or the source's own seniority only decides when the title
// says nothing.
func Level(title, body, employment string, seniority []string) string {
	if intern.MatchString(title) || strings.EqualFold(employment, "intern") {
		return LevelIntern
	}
	switch {
	case senior.MatchString(title):
		return LevelSenior
	case entry.MatchString(title):
		return LevelEntry
	case mid.MatchString(title):
		return LevelMid
	}
	for _, s := range seniority {
		switch strings.ToLower(s) {
		case "entry-level":
			return LevelEntry
		case "mid-level":
			return LevelMid
		case "senior", "manager", "director", "executive":
			return LevelSenior
		}
	}
	if m := years.FindStringSubmatch(body); m != nil {
		n := 0
		for _, c := range m[1] {
			n = n*10 + int(c-'0')
		}
		switch {
		case n <= 1:
			return LevelEntry
		case n <= 4:
			return LevelMid
		default:
			return LevelSenior
		}
	}
	return LevelUnknown
}

// IsEngineering reports whether the title is in scope.
func IsEngineering(title string) bool {
	return engineering.MatchString(title) && !notEngineer.MatchString(title)
}

var (
	indiaPlace = regexp.MustCompile(`(?i)\b(india|bengaluru|bangalore|hyderabad|pune|mumbai|gurgaon|gurugram|noida|chennai|delhi|new delhi|kolkata|ahmedabad|kochi|jaipur|chandigarh|mohali|indore|coimbatore|thiruvananthapuram|trivandrum|visakhapatnam|nagpur|lucknow|bhubaneswar|mysuru|mysore)\b`)
	remoteWord = regexp.MustCompile(`(?i)\b(remote|work from home|wfh|anywhere|worldwide|distributed)\b`)
	global     = regexp.MustCompile(`(?i)\b(anywhere|worldwide|global|globally)\b`)
	region     = regexp.MustCompile(`(?i)\b(us|usa|u\.s\.|united states|canada|uk|united kingdom|europe|eu|emea|germany|france|spain|poland|portugal|ireland|netherlands|latam|brazil|mexico|americas|australia|japan|nyc|new york|california)\b`)
)

// Geography sets where a posting is open. A remote job with a named region elsewhere is
// restricted: "remote" alone isn't proof that someone in India can take it.
func Geography(p source.Posting) (string, []string) {
	var countries []string
	seen := map[string]bool{}
	add := func(c string) {
		if c != "" && !seen[c] {
			seen[c] = true
			countries = append(countries, c)
		}
	}
	for _, c := range p.Countries {
		switch strings.ToUpper(c) {
		case "IN", "INDIA":
			add("IN")
		default:
			add(strings.ToUpper(c))
		}
	}
	if indiaPlace.MatchString(p.Location) {
		add("IN")
	}

	remote := p.Remote == source.RemoteYes || remoteWord.MatchString(p.Location)
	switch {
	case p.Remote == source.RemoteHybrid:
		return ScopeHybrid, countries
	case remote && p.Source == "himalayas" && len(p.Countries) == 0:
		return ScopeGlobal, countries
	case remote && seen["IN"]:
		return ScopeRestricted, countries
	case remote && global.MatchString(p.Location) && !region.MatchString(p.Location):
		return ScopeGlobal, countries
	case remote && region.MatchString(p.Location):
		return ScopeRestricted, countries
	case remote && len(countries) > 0:
		return ScopeRestricted, countries
	case remote:
		// Bare "remote" with no region: unknown who it's open to.
		return ScopeUnknown, countries
	case p.Remote == source.RemoteOnsite || p.Location != "":
		return ScopeOnsite, countries
	}
	return ScopeUnknown, countries
}

// Apply computes every stored field for a posting.
func Apply(p source.Posting) Fields {
	body := p.Body
	if p.BodyIsHTML {
		body = Text(body)
	} else {
		body = collapse(body)
	}
	body = clip(body, MaxBody)
	scope, countries := Geography(p)
	hash := sha256.New()
	for _, part := range []string{p.Title, p.Company, p.Location, p.URL, p.ApplyURL, body} {
		hash.Write([]byte(part))
		hash.Write([]byte{0})
	}
	return Fields{
		Body:          body,
		Excerpt:       excerpt(body),
		Level:         Level(p.Title, body, p.EmploymentType, p.Seniority),
		RemoteScope:   scope,
		Countries:     countries,
		IsEngineering: IsEngineering(p.Title),
		ContentHash:   hash.Sum(nil),
	}
}
