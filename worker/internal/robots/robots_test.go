package robots

import (
	"strings"
	"testing"
	"time"
)

const bot = "RoleRankBot"

func TestRealFiles(t *testing.T) {
	// Each body is reduced from what the host served on 2026-10-30.
	tests := []struct {
		name    string
		body    string
		path    string
		allowed bool
		delay   time.Duration
	}{
		{
			name:    "SmartRecruiters allows only LinkedInBot",
			body:    "User-agent: LinkedInBot\nAllow: /v1/companies/\nUser-agent: *\nDisallow: /\n",
			path:    "/v1/companies/acme/postings",
			allowed: false,
		},
		{
			name:    "Lever API with a crawl delay",
			body:    "User-agent: *\nAllow: /\nCrawl-delay: 1\n",
			path:    "/v0/postings/acme?mode=json",
			allowed: true,
			delay:   time.Second,
		},
		{
			name:    "Greenhouse disallows only embed",
			body:    "# comment\n\nUser-agent: *\nDisallow: /embed/\n",
			path:    "/v1/boards/acme/jobs",
			allowed: true,
		},
		{
			name:    "Greenhouse embed path",
			body:    "User-agent: *\nDisallow: /embed/\n",
			path:    "/embed/job_board",
			allowed: false,
		},
		{
			name:    "Workable has an empty Disallow and a content signal",
			body:    "User-agent: *\nContent-Signal: search=yes, ai-input=yes, ai-train=no\nDisallow: \n",
			path:    "/api/v1/widget/accounts/acme",
			allowed: true,
		},
		{
			name: "RemoteOK merges split star groups",
			body: "User-agent: *\nContent-Signal: search=yes\nAllow: /\n\nUser-agent: GPTBot\nDisallow: /\n\n" +
				"User-agent: *\nCrawl-delay: 1\nAllow: /\n\nUser-agent: AhrefsBot\nDisallow: /\n\n# AJAX\nDisallow: /*?action=get_jobs\n",
			path:    "/api",
			allowed: true,
			delay:   time.Second,
		},
		{
			name:    "several agents share one group",
			body:    "User-agent: AwarioSmartBot\nUser-agent: AwarioRssBot\nDisallow: /\nUser-agent: *\nAllow: /\n",
			path:    "/x",
			allowed: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := Parse(tt.body, bot)
			if got := policy.Allowed(tt.path); got != tt.allowed {
				t.Fatalf("Allowed(%q) = %v, want %v", tt.path, got, tt.allowed)
			}
			if got := policy.CrawlDelay(); got != tt.delay {
				t.Fatalf("CrawlDelay = %v, want %v", got, tt.delay)
			}
		})
	}
}

// Himalayas' real rules, as served on 2026-10-31. Its search pages with `&page=`, which
// these disallow, so ingestion walks the browse feed by cursor.
func TestHimalayasPagingRules(t *testing.T) {
	body := "User-Agent: *\nAllow: /\nDisallow: /apply\nDisallow: /jobs?page=\nDisallow: /jobs*&page=\nDisallow: /jobs/*?page=\n"
	policy := Parse(body, bot)
	for path, want := range map[string]bool{
		"/jobs/api?limit=20":                 true,
		"/jobs/api?limit=20&cursor=abc":      true,
		"/jobs/api/search?country=IN":        true,
		"/jobs/api/search?country=IN&page=2": false,
		"/jobs/api/search?page=2":            false,
	} {
		if got := policy.Allowed(path); got != want {
			t.Errorf("Allowed(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestRFC9309Precedence(t *testing.T) {
	body := "User-agent: *\nDisallow: /a\nAllow: /a/b\nDisallow: /a/b/c\nAllow: /tie\nDisallow: /tie\n"
	policy := Parse(body, bot)
	for path, want := range map[string]bool{
		"/a":      false,
		"/a/b":    true,
		"/a/b/c":  false,
		"/other":  true,
		"/tie":    true,
		"/a/bcde": true,
	} {
		if got := policy.Allowed(path); got != want {
			t.Errorf("Allowed(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestWildcards(t *testing.T) {
	policy := Parse("User-agent: *\nDisallow: /*.pdf$\nDisallow: /p*q\n", bot)
	for path, want := range map[string]bool{
		"/file.pdf":     false,
		"/file.pdf?x=1": true,
		"/pxxq":         false,
		"/pxx":          true,
	} {
		if got := policy.Allowed(path); got != want {
			t.Errorf("Allowed(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestSpecificGroupReplacesStar(t *testing.T) {
	body := "User-agent: *\nDisallow: /\n\nUser-agent: rolerankbot\nAllow: /jobs\nDisallow: /private\n"
	policy := Parse(body, bot)
	if !policy.Allowed("/jobs") || !policy.Allowed("/anything") || policy.Allowed("/private") {
		t.Fatal("our named group must replace the star group, matched case-insensitively")
	}
}

func TestStatusHandling(t *testing.T) {
	blocked := "User-agent: *\nDisallow: /\n"
	tests := []struct {
		status  int
		allowed bool
	}{
		{200, false}, // the body applies
		{401, true},  // some API hosts answer this
		{404, true},
		{500, false},
		{503, false},
		{0, false}, // network failure
	}
	for _, tt := range tests {
		if got := ForStatus(tt.status, blocked, bot).Allowed("/x"); got != tt.allowed {
			t.Errorf("status %d: Allowed = %v, want %v", tt.status, got, tt.allowed)
		}
	}
}

func TestHostileInput(t *testing.T) {
	if !Parse("", bot).Allowed("/x") {
		t.Fatal("an empty file allows everything")
	}
	if !Parse("not robots at all\n\x00\xff:::", bot).Allowed("/x") {
		t.Fatal("garbage allows everything")
	}
	if !Parse("Disallow: /\n", bot).Allowed("/x") {
		t.Fatal("a rule outside any group applies to nobody")
	}
	huge := "User-agent: *\n" + strings.Repeat("Disallow: /aaaaaaaa\n", 200_000) + "Disallow: /late\n"
	policy := Parse(huge, bot)
	if !policy.Allowed("/late") {
		t.Fatal("rules past MaxBytes must be ignored")
	}
	if policy.Allowed("/aaaaaaaa") {
		t.Fatal("rules before MaxBytes must apply")
	}
}
