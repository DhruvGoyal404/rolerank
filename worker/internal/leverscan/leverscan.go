// Package leverscan finds Lever boards for a curated list of companies. A hit is never
// trusted on its own: Lever's response doesn't name the company, so a token can belong to
// someone else. A board counts only when the company's own site links to it.
//
// Names and official sites come from Wikipedia's ordinary pages, read through the fetcher
// so robots.txt applies. Wikidata's query service is disallowed by its robots.txt, so it
// isn't used. Names and URLs are facts, and the text is CC BY-SA, so the source is credited.
package leverscan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/DhruvGoyal404/rolerank/worker/internal/fetch"
)

const wikipedia = "https://en.wikipedia.org"

// Categories lists where the curated names come from.
var Categories = []string{
	"Category:Software_companies_of_India",
	"Category:Internet_properties_established_in_India",
	"Category:Indian_companies_established_in_2015",
	"Category:Indian_companies_established_in_2016",
	"Category:Indian_companies_established_in_2017",
	"Category:Financial_technology_companies_of_India",
	"Category:Online_retailers_of_India",
	"Category:Unicorn_startups_of_India",
	"Category:Indian_educational_websites",
	"Category:Technology_companies_of_India",
}

// Getter is the part of the fetcher this package needs.
type Getter interface {
	Get(ctx context.Context, rawURL, etag string) (*fetch.Result, error)
}

// Company is a candidate with its official site.
type Company struct {
	Name string
	Page string
	Site string
}

var (
	notName = regexp.MustCompile(`[^a-z0-9]+`)
	suffix  = regexp.MustCompile(`(?i)\s+(private limited|pvt\.? ltd\.?|limited|ltd\.?|inc\.?|technologies|technology|\(.*\))\s*$`)
	lists   = regexp.MustCompile(`(?i)^(list of|template:|category:|portal:|wikipedia:|file:|help:)`)
)

// Token makes the single guess for a company name.
func Token(name string) string {
	for {
		shorter := suffix.ReplaceAllString(name, "")
		if shorter == name {
			break
		}
		name = shorter
	}
	return notName.ReplaceAllString(strings.ToLower(name), "")
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasClass(n *html.Node, class string) bool {
	for _, c := range strings.Fields(attr(n, "class")) {
		if c == class {
			return true
		}
	}
	return false
}

func walk(n *html.Node, visit func(*html.Node)) {
	visit(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, visit)
	}
}

// CategoryPages reads the article links on a category page.
func CategoryPages(body []byte) []Company {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil
	}
	var out []Company
	walk(doc, func(n *html.Node) {
		if n.DataAtom != atom.Div || attr(n, "id") != "mw-pages" {
			return
		}
		walk(n, func(a *html.Node) {
			href := attr(a, "href")
			title := attr(a, "title")
			if a.DataAtom != atom.A || !strings.HasPrefix(href, "/wiki/") || title == "" || lists.MatchString(title) {
				return
			}
			out = append(out, Company{Name: title, Page: wikipedia + href})
		})
	})
	return out
}

// OfficialSite reads the website from an article's infobox.
func OfficialSite(body []byte) string {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return ""
	}
	site := ""
	walk(doc, func(n *html.Node) {
		if site != "" || n.DataAtom != atom.Table || !hasClass(n, "infobox") {
			return
		}
		walk(n, func(th *html.Node) {
			if site != "" || th.DataAtom != atom.Th || !strings.Contains(strings.ToLower(text(th)), "website") {
				return
			}
			for cell := th.NextSibling; cell != nil && site == ""; cell = cell.NextSibling {
				walk(cell, func(a *html.Node) {
					if site == "" && a.DataAtom == atom.A {
						if u, err := url.Parse(attr(a, "href")); err == nil && strings.HasPrefix(u.Scheme, "http") {
							site = u.Scheme + "://" + u.Host
						}
					}
				})
			}
		})
	})
	return site
}

func text(n *html.Node) string {
	var b strings.Builder
	walk(n, func(c *html.Node) {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
		}
	})
	return b.String()
}

// Hit reports whether Lever has a live, non-empty board for a token.
func Hit(ctx context.Context, lever Getter, token string) (bool, error) {
	res, err := lever.Get(ctx, "https://api.lever.co/v0/postings/"+url.PathEscape(token)+"?mode=json&limit=1", "")
	if err != nil {
		return false, err
	}
	if res.Status != 200 {
		return false, nil
	}
	var postings []json.RawMessage
	if err := json.Unmarshal(res.Body, &postings); err != nil {
		return false, nil
	}
	return len(postings) > 0, nil
}

// LinksTo reports whether a page links to this exact Lever board. The token must end at a
// boundary, so "acme" isn't confirmed by a link to "acme-labs".
func LinksTo(body []byte, token string) bool {
	pattern := `(?i)jobs\.(eu\.)?lever\.co/` + regexp.QuoteMeta(token) + `([/"'?#\s<]|$)`
	return regexp.MustCompile(pattern).Match(body)
}

// Hosts lists where a company's careers page may live, on its own domain only.
func Hosts(site string) []string {
	u, err := url.Parse(site)
	if err != nil || u.Host == "" {
		return nil
	}
	root := strings.TrimPrefix(u.Host, "www.")
	return []string{root, "www." + root, "careers." + root, "jobs." + root}
}

var careerPaths = []string{"/careers", "/jobs", "/"}

// Outcome of verifying a hit.
type Outcome string

// Outcomes. Only Verified is trusted.
const (
	Verified    Outcome = "verified"
	NotLinked   Outcome = "not_linked"
	Blocked     Outcome = "robots_disallowed"
	NoSite      Outcome = "no_site"
	Unreachable Outcome = "unreachable"
)

// Verify looks for the board on the company's own pages. Robots.txt is enforced by the
// fetcher, and a page it refuses counts as unverified, never as confirmed.
func Verify(ctx context.Context, site Getter, base, token string) Outcome {
	if base == "" {
		return NoSite
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return NoSite
	}
	root := strings.TrimPrefix(u.Host, "www.")
	targets := []string{}
	for _, host := range []string{"www." + root, root, "careers." + root} {
		for _, path := range careerPaths {
			targets = append(targets, "https://"+host+path)
		}
	}
	blocked, reached := false, false
	for _, target := range targets {
		res, err := site.Get(ctx, target, "")
		switch {
		case errors.Is(err, fetch.ErrDisallowed):
			blocked = true
			continue
		case err != nil:
			continue
		}
		reached = true
		if res.Status == 200 && LinksTo(res.Body, token) {
			return Verified
		}
	}
	switch {
	case reached:
		return NotLinked
	case blocked:
		return Blocked
	}
	return Unreachable
}

// Line formats one result for the report file.
func Line(c Company, token string, outcome Outcome) string {
	return fmt.Sprintf("%s\t%s\t%s\t%s", token, outcome, c.Name, c.Site)
}
