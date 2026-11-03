package normalize

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/DhruvGoyal404/rolerank/worker/internal/source"
)

func TestTextRemovesMarkup(t *testing.T) {
	tests := map[string]string{
		"<p>Hello <b>world</b></p><ul><li>Go</li><li>SQL</li></ul>": "Hello world\nGo\nSQL",
		"&lt;p&gt;Escaped once &amp;amp; again&lt;/p&gt;":           "Escaped once & again",
		`<script>alert(1)</script>Safe<style>p{color:red}</style>`:  "Safe",
		`<img src=x onerror=alert(1)>Text`:                          "Text",
		"Plain   text\r\n\r\n\r\nwith gaps":                         "Plain text\nwith gaps",
		"&lt;script&gt;alert(1)&lt;/script&gt;Safe":                 "Safe",
		"a&nbsp;b":                   "a b",
		"<p>One</p>\n\n\n<p>Two</p>": "One\nTwo",
		"":                           "",
	}
	for in, want := range tests {
		if got := Text(in); got != want {
			t.Errorf("Text(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLevel(t *testing.T) {
	tests := []struct {
		title, body, employment string
		seniority               []string
		want                    string
	}{
		{title: "Software Engineering Intern", want: LevelIntern},
		{title: "Summer Internship - Backend", want: LevelIntern},
		{title: "Backend Engineer", employment: "Intern", want: LevelIntern},
		{title: "Graduate Software Engineer", want: LevelEntry},
		{title: "SDE-1", want: LevelEntry},
		{title: "Associate Software Engineer", want: LevelEntry},
		{title: "Software Engineer I", want: LevelEntry},
		{title: "Software Engineer II", want: LevelMid},
		{title: "SDE 2", want: LevelMid},
		{title: "Senior Backend Engineer", want: LevelSenior},
		{title: "Staff Engineer", want: LevelSenior},
		{title: "Engineering Manager", want: LevelSenior},
		{title: "Internal Tools Engineer", want: LevelUnknown},
		{title: "Backend Engineer", body: "You have 0-1 years of experience.", want: LevelEntry},
		{title: "Backend Engineer", body: "Requires 3+ years of experience.", want: LevelMid},
		{title: "Backend Engineer", body: "At least 8 years experience.", want: LevelSenior},
		{title: "Backend Engineer", body: "We were founded 10 years ago", want: LevelUnknown},
		{title: "Backend Engineer", seniority: []string{"Entry-level"}, want: LevelEntry},
		{title: "Backend Engineer", seniority: []string{"Mid-level"}, want: LevelMid},
		{title: "Backend Engineer", want: LevelUnknown},
		{title: "Senior Backend Engineer", body: "1 year of experience", want: LevelSenior},
		{title: "Junior Software Engineer", body: "2 - 4 years of experience in C++", want: LevelMid},
		{title: "Software Engineer II", body: "Minimum 5+ years of engineering experience", want: LevelSenior},
		{title: "Data Scientist", body: "1-3 years of experience", want: LevelMid},
		{title: "Junior Developer", body: "0-1 years of experience", want: LevelEntry},
		{title: "Full Stack Engineer - Junior", body: "2+ years of professional software engineering experience", want: LevelMid},
	}
	for _, tt := range tests {
		if got := Level(tt.title, tt.body, tt.employment, tt.seniority); got != tt.want {
			t.Errorf("Level(%q, %q) = %q, want %q", tt.title, tt.body, got, tt.want)
		}
	}
}

func TestIsEngineering(t *testing.T) {
	yes := []string{"Software Engineer", "Backend Developer", "SDE-2", "Data Scientist", "DevOps Engineer", "ML Engineer", "Full-Stack Developer", "Embedded Firmware Engineer"}
	no := []string{"Account Executive", "Sales Engineer", "Recruiter", "Civil Engineer", "Marketing Manager", "Mechanical Engineer", "PCB Layout Engineer", "Solutions Engineer", "FPGA Engineer"}
	for _, title := range yes {
		if !IsEngineering(title, "") {
			t.Errorf("%q should be in scope", title)
		}
	}
	for _, title := range no {
		if IsEngineering(title, "Python, Java, AWS, Linux and SQL") {
			t.Errorf("%q should be out of scope", title)
		}
	}
	if IsEngineering("Test Engineer", "Test robotic hardware") || !IsEngineering("Test Engineer", "Python, Linux, AWS") {
		t.Error("a bare engineer title must go by its text")
	}
	if IsEngineering("Data Analyst", "Power BI dashboards, some SQL") || !IsEngineering("Data Analyst", "SQL and Python daily") {
		t.Error("a data analyst is in only if the role is mainly SQL or code")
	}
}

func TestBlankHimalayasLocationNeedsWordsSayingOpen(t *testing.T) {
	blank := source.Posting{Source: "himalayas", Remote: source.RemoteYes, Body: "Remote (CET)"}
	if scope, _ := Geography(blank); scope != ScopeUnknown {
		t.Fatalf("got %s", scope)
	}
	blank.Body = "Work from anywhere in the world."
	if scope, _ := Geography(blank); scope != ScopeGlobal {
		t.Fatalf("got %s", scope)
	}
}

func TestGeography(t *testing.T) {
	tests := []struct {
		name  string
		p     source.Posting
		scope string
		india bool
	}{
		{"Indian city", source.Posting{Location: "Bengaluru, Karnataka"}, ScopeOnsite, true},
		{"remote India", source.Posting{Location: "Remote - India"}, ScopeRestricted, true},
		{"remote US", source.Posting{Location: "Remote, United States"}, ScopeRestricted, false},
		{"remote EMEA", source.Posting{Location: "Remote (EMEA)", Remote: source.RemoteYes}, ScopeRestricted, false},
		{"anywhere", source.Posting{Location: "Remote - Anywhere"}, ScopeGlobal, false},
		{"bare remote is unknown", source.Posting{Location: "Remote"}, ScopeUnknown, false},
		{"hybrid", source.Posting{Location: "Pune", Remote: source.RemoteHybrid}, ScopeHybrid, true},
		{"country code", source.Posting{Countries: []string{"IN"}, Remote: source.RemoteYes}, ScopeRestricted, true},
		{"Lever lower case country", source.Posting{Countries: []string{"India"}, Location: "Hyderabad"}, ScopeOnsite, true},
		{"Himalayas no restriction", source.Posting{Source: "himalayas", Remote: source.RemoteYes, Body: "Hire from anywhere in the world"}, ScopeGlobal, false},
		{"Himalayas India list", source.Posting{Source: "himalayas", Remote: source.RemoteYes, Countries: []string{"India", "Canada"}}, ScopeRestricted, true},
		{"nothing stated", source.Posting{}, ScopeUnknown, false},
		{"Indiana is not India", source.Posting{Location: "Indianapolis, Indiana"}, ScopeOnsite, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scope, countries := Geography(tt.p)
			india := false
			for _, c := range countries {
				india = india || c == "IN"
			}
			if scope != tt.scope || india != tt.india {
				t.Fatalf("got %s %v, want %s india=%t", scope, countries, tt.scope, tt.india)
			}
		})
	}
}

func TestBodyIsClippedOnARuneBoundary(t *testing.T) {
	body := strings.Repeat("é", MaxBody)
	f := Apply(source.Posting{Title: "T", Body: body})
	if len(f.Body) > MaxBody || !utf8.ValidString(f.Body) {
		t.Fatalf("%d bytes, valid=%t", len(f.Body), utf8.ValidString(f.Body))
	}
	if len(f.Excerpt) > MaxExcerpt+3 || !utf8.ValidString(f.Excerpt) {
		t.Fatalf("excerpt %d bytes, valid=%t", len(f.Excerpt), utf8.ValidString(f.Excerpt))
	}
}

func TestLinkOnlySourcesGetNoExcerpt(t *testing.T) {
	for name, want := range map[string]bool{"greenhouse": false, "lever": false, "himalayas": true, "remoteok": true} {
		f := Apply(source.Posting{Source: name, Title: "T", Body: "Build things"})
		if (f.Excerpt != "") != want || f.Body == "" {
			t.Errorf("%s: excerpt %q, body %q", name, f.Excerpt, f.Body)
		}
	}
}

func TestHashChangesWithContentOnly(t *testing.T) {
	base := source.Posting{Title: "T", Company: "C", URL: "https://x.example/1", Body: "<p>one</p>", BodyIsHTML: true}
	same := base
	same.SalaryMax = 9
	changed := base
	changed.Body = "<p>two</p>"
	a, b, c := Apply(base), Apply(same), Apply(changed)
	if string(a.ContentHash) != string(b.ContentHash) {
		t.Fatal("identical text must hash the same")
	}
	if string(a.ContentHash) == string(c.ContentHash) {
		t.Fatal("changed text must hash differently")
	}
}

func TestHashSeparatesFields(t *testing.T) {
	a := Apply(source.Posting{Title: "AB", Company: "C", URL: "u"})
	b := Apply(source.Posting{Title: "A", Company: "BC", URL: "u"})
	if string(a.ContentHash) == string(b.ContentHash) {
		t.Fatal("field boundaries must affect the hash")
	}
}

func TestNullBytesNeverReachTheDatabase(t *testing.T) {
	if strings.Contains(Apply(source.Posting{Title: "T", Body: "a\x00b"}).Body, "\x00") {
		t.Fatal("Postgres text can't hold a NUL")
	}
}
