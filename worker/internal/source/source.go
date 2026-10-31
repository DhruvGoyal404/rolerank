// Package source turns each provider's JSON into one Posting shape. Field details and
// terms for every source are in docs/SOURCES.md. A malformed response is an error, never
// an empty list: an empty list would mark a board's jobs as closed.
package source

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Remote is what a source states about where the work happens.
type Remote string

// Values of Remote. Enrichment decides who a remote job is open to.
const (
	RemoteUnknown Remote = ""
	RemoteOnsite  Remote = "onsite"
	RemoteHybrid  Remote = "hybrid"
	RemoteYes     Remote = "remote"
)

// Posting is a job as a source described it, before enrichment.
type Posting struct {
	SourceJobID    string
	GroupID        string // Greenhouse's internal_job_id: posts of one underlying job
	Title          string
	Company        string
	Location       string
	Countries      []string
	Remote         Remote
	EmploymentType string
	Language       string
	Body           string
	BodyIsHTML     bool
	URL            string
	ApplyURL       string
	PostedAt       time.Time
	UpdatedAt      time.Time
	ExpiresAt      time.Time
	SalaryMin      float64
	SalaryMax      float64
	SalaryCurrency string
	SalaryPeriod   string
	Tags           []string
	Seniority      []string
}

// Page is one response. NextURL is empty on the last page. Dropped counts postings
// refused for missing a required field. Skipped counts ones we chose not to use.
type Page struct {
	Postings []Posting
	NextURL  string
	Dropped  int
	Skipped  int
}

// Attribution is what the UI must do for a source's postings.
type Attribution struct {
	Credit string // the name shown as "via ..."
	// FollowLink requires a plain followable link to the source page.
	FollowLink bool
}

// Adapter reads one source.
type Adapter interface {
	Name() string
	Hosts() []string
	Attribution() Attribution
	// FirstURL is the request for a board. Sources with one feed ignore token.
	FirstURL(token string) string
	Parse(token string, body []byte) (Page, error)
}

// All lists the sources approved in docs/SOURCES.md.
func All() []Adapter {
	return []Adapter{Greenhouse{}, Lever{}, Ashby{}, Himalayas{}, RemoteOK{}}
}

func validURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != ""
}

// keep applies the fields every posting must have, and normalises the rest.
func keep(page *Page, p Posting) {
	p.Title = strings.TrimSpace(p.Title)
	if p.SourceJobID == "" || p.Title == "" || !validURL(p.URL) {
		page.Dropped++
		return
	}
	if p.ApplyURL != "" && !validURL(p.ApplyURL) {
		p.ApplyURL = ""
	}
	page.Postings = append(page.Postings, p)
}

func parseTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func epoch(seconds int64) time.Time {
	if seconds <= 0 {
		return time.Time{}
	}
	return time.Unix(seconds, 0).UTC()
}

func decodeError(name string, err error) error {
	return fmt.Errorf("source %s: malformed response: %w", name, err)
}
