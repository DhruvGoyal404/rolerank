// Package robots parses and applies robots.txt under RFC 9309, plus Crawl-delay,
// which is non-standard but which we honour. Written here because the common Go
// parser's last release is from 2021, and the rules fit in one small, tested file.
package robots

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MaxBytes is the most of a robots.txt we read. RFC 9309 requires at least 500 KiB.
const MaxBytes = 512 * 1024

type rule struct {
	allow   bool
	length  int
	pattern *regexp.Regexp
}

// Policy is what one agent may do on one host.
type Policy struct {
	rules      []rule
	crawlDelay time.Duration
	blockAll   bool
}

// ForStatus applies RFC 9309's status handling. A 4xx means no rules apply. A 5xx, or
// no response at all (status 0), means assume complete disallow until a retry works.
func ForStatus(status int, body string, agent string) Policy {
	switch {
	case status >= 200 && status < 300:
		return Parse(body, agent)
	case status >= 400 && status < 500:
		return Policy{}
	default:
		return Policy{blockAll: true}
	}
}

type group struct {
	agents []string
	rules  []rule
	delay  time.Duration
}

// Parse returns the policy for agent. Groups naming the same agent are merged, as
// RFC 9309 requires, and a specific group replaces `*` entirely.
func Parse(body string, agent string) Policy {
	if len(body) > MaxBytes {
		body = body[:MaxBytes]
	}
	var groups []*group
	var current *group
	inAgents := false
	for _, raw := range strings.Split(body, "\n") {
		line := raw
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "user-agent":
			if !inAgents {
				current = &group{}
				groups = append(groups, current)
				inAgents = true
			}
			current.agents = append(current.agents, strings.ToLower(value))
		case "allow", "disallow":
			inAgents = false
			if current == nil || value == "" {
				continue
			}
			current.rules = append(current.rules, rule{
				allow:   key == "allow",
				length:  len(value),
				pattern: compile(value),
			})
		case "crawl-delay":
			inAgents = false
			if current == nil {
				continue
			}
			if seconds, err := strconv.ParseFloat(value, 64); err == nil && seconds > 0 {
				current.delay = time.Duration(seconds * float64(time.Second))
			}
		default:
			// Sitemap, Content-Signal and others sit outside the groups we match on.
			if key != "sitemap" && key != "content-signal" {
				inAgents = false
			}
		}
	}

	token := strings.ToLower(agent)
	policy := collect(groups, token)
	if policy == nil {
		policy = collect(groups, "*")
	}
	if policy == nil {
		return Policy{}
	}
	return *policy
}

func collect(groups []*group, token string) *Policy {
	var policy *Policy
	for _, g := range groups {
		for _, name := range g.agents {
			if name != token {
				continue
			}
			if policy == nil {
				policy = &Policy{}
			}
			policy.rules = append(policy.rules, g.rules...)
			if g.delay > policy.crawlDelay {
				policy.crawlDelay = g.delay
			}
			break
		}
	}
	return policy
}

func compile(path string) *regexp.Regexp {
	var b strings.Builder
	b.WriteByte('^')
	anchored := strings.HasSuffix(path, "$")
	path = strings.TrimSuffix(path, "$")
	for i, part := range strings.Split(path, "*") {
		if i > 0 {
			b.WriteString(".*")
		}
		b.WriteString(regexp.QuoteMeta(part))
	}
	if anchored {
		b.WriteByte('$')
	}
	return regexp.MustCompile(b.String())
}

// Allowed reports whether path (with any query) may be fetched. The longest matching
// rule wins, and Allow wins a tie. No match means allowed.
func (p Policy) Allowed(path string) bool {
	if p.blockAll {
		return false
	}
	best := -1
	allowed := true
	for _, r := range p.rules {
		if !r.pattern.MatchString(path) {
			continue
		}
		if r.length > best || (r.length == best && r.allow) {
			best = r.length
			allowed = r.allow
		}
	}
	return allowed
}

// CrawlDelay returns the host's requested delay between requests, or zero.
func (p Policy) CrawlDelay() time.Duration {
	return p.crawlDelay
}
