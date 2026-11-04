// Package normalize turns a source's raw posting into the fields we store and search.
// Everything here is deterministic. An LLM per posting is the wrong cost and latency trade
// (ARCHITECTURE.md). The rules are measured by the D6 eval, not assumed to be right.
package normalize

import (
	"crypto/sha256"
	"html"
	"regexp"
	"strconv"
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

// linkOnly sources are shown as a title and a link, never their text (docs/SOURCES.md). No
// excerpt is made for them, so none can reach a page.
var linkOnly = map[string]bool{"greenhouse": true, "lever": true}

func excerpt(sourceName, body string) string {
	if linkOnly[sourceName] {
		return ""
	}
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
	notEngineer = regexp.MustCompile(`(?i)\b(sales engineer|solutions? engineer|solutions? engineering|solutions? architect|account (executive|manager)|recruiter|talent acquisition|business developer|product manager|civil|mechanical|chemical|electrical|structural|geotechnical|environmental|manufacturing|process engineer|production|tooling|harness|avionics|pcb|photonics?|rf|fpga|rtl|vlsi|asic|chip|hardware|propulsion|combustion|thermal|launch|broadcast|usability|field (service|inspector|process)|project engineer|plan check|sales development)\b`)
	intern      = regexp.MustCompile(`(?i)\b(intern|internship|interns|apprentice|apprenticeship|trainee|co-?op)\b`)
	entry       = regexp.MustCompile(`(?i)\b(new grad|new graduate|graduate|grad|fresher|freshers|entry[- ]level|early career|junior|jr\.?|campus|associate (software|engineer|developer)|engineer i|developer i|sde[- ]?1|sde[- ]?i|software engineer 1)\b`)
	senior      = regexp.MustCompile(`(?i)\b(senior|sr\.?|staff|principal|lead|manager|director|head of|vice president|vp|architect|distinguished|fellow|engineer iii|engineer iv|sde[- ]?3|sde[- ]?iii)\b`)
	mid         = regexp.MustCompile(`(?i)\b(mid[- ]?level|engineer ii|developer ii|sde[- ]?2|sde[- ]?ii)\b`)
	years       = regexp.MustCompile(`(?i)\b(\d{1,2}(?:\.\d)?)\s*(?:\+|[-–]\s*(\d{1,2}(?:\.\d)?)|to\s*(\d{1,2}(?:\.\d)?))?\s*\+?\s*(?:years?|yrs?)\b[^.\n]{0,60}\b(?:experience|exp|background)\b`)
)

// Level reads the title first, because titles are what employers set deliberately. A
// stated minimum in the body or the source's own seniority only decides when the title
// says nothing.
func Level(title, body, employment string, seniority []string) string {
	if intern.MatchString(title) || strings.EqualFold(employment, "intern") {
		return LevelIntern
	}
	if senior.MatchString(title) {
		return LevelSenior
	}
	// A graduate or campus programme is entry whatever years it mentions.
	if graduate.MatchString(title) {
		return LevelEntry
	}
	// Stated years beat a "Junior", "I" or "II" title: the rule judges what the posting asks.
	if level := fromYears(body); level != "" {
		return level
	}
	if forGraduates.MatchString(body) {
		return LevelEntry
	}
	switch {
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
	return LevelUnknown
}

// fromYears reads the first stated experience; a range counts by its middle, so "1-3 years"
// is mid and "0-1 years" is entry.
var (
	graduate = regexp.MustCompile(`(?i)\b(new grad|new graduate|recent grads?|recent graduates?|graduate|university|campus|fresher|freshers|early[- ]career)\b`)
	// In the text, only a phrase aimed at graduates counts: "graduate" alone is often a degree.
	forGraduates = regexp.MustCompile(`(?i)\b(new grads?|new graduates?|recent grads?|recent graduates?|early[- ]career|freshers?)\b`)
	// "two years" and "three (3) years" are read as numbers.
	spelled = regexp.MustCompile(`(?i)\b(one|two|three|four|five|six|seven|eight|nine|ten)\b(?:\s*\(\s*\d{1,2}\s*\))?(\s*\+?\s*(?:years?|yrs?)\b)`)
	numbers = map[string]string{"one": "1", "two": "2", "three": "3", "four": "4", "five": "5", "six": "6", "seven": "7", "eight": "8", "nine": "9", "ten": "10"}
	// "a Bachelor's degree or 3+ years" offers years in place of a degree, so a graduate qualifies.
	inLieu = regexp.MustCompile(`(?i)\bdegree\b[^.\n]{0,120}\bor\s*$`)
)

func fromYears(body string) string {
	body = spelled.ReplaceAllStringFunc(body, func(s string) string {
		m := spelled.FindStringSubmatch(s)
		return numbers[strings.ToLower(m[1])] + m[2]
	})
	var m []string
	for _, at := range years.FindAllStringSubmatchIndex(body, -1) {
		start := max(at[0]-140, 0)
		after := strings.ToLower(body[at[1]:min(at[1]+30, len(body))])
		if inLieu.MatchString(body[start:at[0]]) || strings.Contains(after, "in lieu") || strings.Contains(after, "substitut") {
			continue
		}
		m = make([]string, 0, 4)
		for g := 0; g < len(at); g += 2 {
			if at[g] < 0 {
				m = append(m, "")
			} else {
				m = append(m, body[at[g]:at[g+1]])
			}
		}
		break
	}
	if m == nil {
		if inLieu.MatchString(body) || years.MatchString(body) {
			return LevelEntry // the only years stated stand in for a degree
		}
		return ""
	}
	low, _ := strconv.ParseFloat(m[1], 64)
	n := low
	for _, upper := range m[2:] {
		if high, err := strconv.ParseFloat(upper, 64); err == nil && high >= low {
			n = (low + high) / 2
		}
	}
	switch {
	case n <= 1.5: // "up to about 1 year" takes in "1 to 1.5" and "1-2 years"
		return LevelEntry
	case n <= 4:
		return LevelMid
	}
	return LevelSenior
}

// softwareTitle names software work outright. A title with only the bare word "engineer"
// says nothing, so its text decides: hardware and field roles rarely list a software stack.
var (
	softwareTitle = regexp.MustCompile(`(?i)\b(software|developer|programmer|sde|sdet|sre|site reliability|devops|devsecops|data engineer|analytics engineer|data scientist|machine learning|ml|ai|mlops|full[- ]?stack|back[- ]?end|front[- ]?end|android|ios|qa|embedded|firmware|platform engineer|cloud|security engineer|infrastructure|technical support|support engineer)\b`)
	stackWord     = regexp.MustCompile(`(?i)\b(python|java|javascript|typescript|golang|kubernetes|docker|aws|azure|gcp|sql|linux|rest apis?|microservices|terraform|ci/cd|git)\b`)
	sqlWord       = regexp.MustCompile(`(?i)\bsql\b`)
	codeWord      = regexp.MustCompile(`(?i)\b(python|r programming)\b`)
	analyst       = regexp.MustCompile(`(?i)\bdata analyst\b`)
)

// IsEngineering applies the owner's scope rules to a title and its text.
func IsEngineering(title, body string) bool {
	if !engineering.MatchString(title) || notEngineer.MatchString(title) {
		return false
	}
	if analyst.MatchString(title) {
		return sqlWord.MatchString(body) && codeWord.MatchString(body)
	}
	if softwareTitle.MatchString(title) {
		return true
	}
	distinct := map[string]bool{}
	for _, m := range stackWord.FindAllString(body, -1) {
		distinct[strings.ToLower(m)] = true
	}
	return len(distinct) >= 3
}

var (
	indiaPlace = regexp.MustCompile(`(?i)\b(india|bengaluru|bangalore|hyderabad|pune|mumbai|gurgaon|gurugram|noida|chennai|delhi|new delhi|kolkata|ahmedabad|kochi|jaipur|chandigarh|mohali|indore|coimbatore|thiruvananthapuram|trivandrum|visakhapatnam|nagpur|lucknow|bhubaneswar|mysuru|mysore)\b`)
	remoteWord = regexp.MustCompile(`(?i)\b(remote|work from home|wfh|anywhere|worldwide|distributed)\b`)
	global     = regexp.MustCompile(`(?i)\b(anywhere|worldwide|global|globally)\b`)
	// A blank location isn't proof of anything: open to the world needs words saying so.
	openWorld = regexp.MustCompile(`(?i)\b(worldwide|anywhere in the world|work from anywhere|remote[- ]first,? globally|from anywhere|globally remote|open to (candidates )?(in |from )?india|based in india|remote[ ,(-]+india)\b`)
	region    = regexp.MustCompile(`(?i)\b(us|usa|u\.s\.|united states|canada|uk|united kingdom|europe|eu|emea|germany|france|spain|poland|portugal|ireland|netherlands|latam|brazil|mexico|americas|australia|japan|nyc|new york|california)\b`)
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
		if openWorld.MatchString(p.Body) {
			return ScopeGlobal, countries
		}
		return ScopeUnknown, countries
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
		Excerpt:       excerpt(p.Source, body),
		Level:         Level(p.Title, body, p.EmploymentType, p.Seniority),
		RemoteScope:   scope,
		Countries:     countries,
		IsEngineering: IsEngineering(p.Title, body),
		ContentHash:   hash.Sum(nil),
	}
}
