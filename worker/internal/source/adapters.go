package source

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Greenhouse reads the public Job Board API.
type Greenhouse struct{}

// Name implements Adapter.
func (Greenhouse) Name() string { return "greenhouse" }

// Hosts implements Adapter.
func (Greenhouse) Hosts() []string { return []string{"boards-api.greenhouse.io"} }

// Attribution implements Adapter.
func (Greenhouse) Attribution() Attribution { return Attribution{Credit: "Greenhouse"} }

// FirstURL implements Adapter.
func (Greenhouse) FirstURL(token string) string {
	return "https://boards-api.greenhouse.io/v1/boards/" + url.PathEscape(token) + "/jobs?content=true"
}

// Parse implements Adapter.
func (a Greenhouse) Parse(_ string, body []byte) (Page, error) {
	var data struct {
		Jobs *[]struct {
			ID             int64  `json:"id"`
			InternalJobID  *int64 `json:"internal_job_id"`
			Title          string `json:"title"`
			CompanyName    string `json:"company_name"`
			Content        string `json:"content"`
			AbsoluteURL    string `json:"absolute_url"`
			Language       string `json:"language"`
			UpdatedAt      string `json:"updated_at"`
			FirstPublished string `json:"first_published"`
			Deadline       string `json:"application_deadline"`
			Location       struct {
				Name string `json:"name"`
			} `json:"location"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return Page{}, decodeError(a.Name(), err)
	}
	if data.Jobs == nil {
		return Page{}, decodeError(a.Name(), fmt.Errorf("no jobs field"))
	}
	var page Page
	for _, j := range *data.Jobs {
		p := Posting{
			Title: j.Title, Company: j.CompanyName, Location: j.Location.Name,
			Language: j.Language, Body: j.Content, BodyIsHTML: true, URL: j.AbsoluteURL,
			PostedAt: parseTime(j.FirstPublished), UpdatedAt: parseTime(j.UpdatedAt),
			ExpiresAt: parseTime(j.Deadline),
		}
		if j.ID != 0 {
			p.SourceJobID = strconv.FormatInt(j.ID, 10)
		}
		if j.InternalJobID != nil {
			p.GroupID = strconv.FormatInt(*j.InternalJobID, 10)
		}
		keep(&page, p)
	}
	return page, nil
}

// Lever reads the public postings API.
type Lever struct{}

// Name implements Adapter.
func (Lever) Name() string { return "lever" }

// Hosts implements Adapter.
func (Lever) Hosts() []string { return []string{"api.lever.co"} }

// Attribution implements Adapter.
func (Lever) Attribution() Attribution { return Attribution{Credit: "Lever"} }

// FirstURL implements Adapter.
func (Lever) FirstURL(token string) string {
	return "https://api.lever.co/v0/postings/" + url.PathEscape(token) + "?mode=json"
}

// Parse implements Adapter.
func (a Lever) Parse(token string, body []byte) (Page, error) {
	var data []struct {
		ID         string `json:"id"`
		Text       string `json:"text"`
		Country    string `json:"country"`
		Workplace  string `json:"workplaceType"`
		CreatedAt  int64  `json:"createdAt"`
		Hosted     string `json:"hostedUrl"`
		Apply      string `json:"applyUrl"`
		Opening    string `json:"openingPlain"`
		Plain      string `json:"descriptionPlain"`
		Additional string `json:"additionalPlain"`
		Categories struct {
			Location     string   `json:"location"`
			Commitment   string   `json:"commitment"`
			AllLocations []string `json:"allLocations"`
		} `json:"categories"`
		Lists []struct {
			Text    string `json:"text"`
			Content string `json:"content"`
		} `json:"lists"`
		Salary *struct {
			Min      float64 `json:"min"`
			Max      float64 `json:"max"`
			Currency string  `json:"currency"`
			Interval string  `json:"interval"`
		} `json:"salaryRange"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return Page{}, decodeError(a.Name(), err)
	}
	if data == nil {
		return Page{}, decodeError(a.Name(), fmt.Errorf("null body"))
	}
	var page Page
	for _, j := range data {
		// Lever splits a posting across fields. Only the list items are HTML.
		parts := []string{j.Opening, j.Plain}
		for _, list := range j.Lists {
			parts = append(parts, list.Text, list.Content)
		}
		parts = append(parts, j.Additional)
		location := j.Categories.Location
		if len(j.Categories.AllLocations) > 1 {
			location = strings.Join(j.Categories.AllLocations, " | ")
		}
		p := Posting{
			SourceJobID: j.ID, Title: j.Text, Company: token, Location: location,
			EmploymentType: j.Categories.Commitment, URL: j.Hosted, ApplyURL: j.Apply,
			Body: strings.Join(nonEmpty(parts), "\n"), BodyIsHTML: len(j.Lists) > 0,
			PostedAt: epoch(j.CreatedAt / 1000),
		}
		if j.Country != "" {
			p.Countries = []string{strings.ToUpper(j.Country)}
		}
		switch j.Workplace {
		case "remote":
			p.Remote = RemoteYes
		case "hybrid":
			p.Remote = RemoteHybrid
		case "on-site":
			p.Remote = RemoteOnsite
		}
		if j.Salary != nil {
			p.SalaryMin, p.SalaryMax = j.Salary.Min, j.Salary.Max
			p.SalaryCurrency, p.SalaryPeriod = j.Salary.Currency, j.Salary.Interval
		}
		keep(&page, p)
	}
	return page, nil
}

func nonEmpty(values []string) []string {
	out := values[:0:0]
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}

// Himalayas reads the public remote jobs feed.
type Himalayas struct{}

// The browse feed, walked by cursor and filtered here, which is what their docs say to do
// to collect one country. The search endpoint pages with `&page=`, which their robots.txt
// disallows, so it's not an option.
const himalayasFeed = "https://himalayas.app/jobs/api?limit=20"

// indiaOffset is India's standard UTC offset, for their timezone rule.
const indiaOffset = 5.5

// openToIndia applies the rule in their docs: no location restrictions or India among
// them, and India's UTC offset among the timezones. An empty timezone list is no limit.
func openToIndia(locations []string, zones []float64) bool {
	located := len(locations) == 0
	for _, l := range locations {
		if strings.EqualFold(l, "India") || strings.EqualFold(l, "IN") {
			located = true
		}
	}
	if !located {
		return false
	}
	if len(zones) == 0 {
		return true
	}
	for _, z := range zones {
		if z == indiaOffset {
			return true
		}
	}
	return false
}

// Name implements Adapter.
func (Himalayas) Name() string { return "himalayas" }

// Hosts implements Adapter.
func (Himalayas) Hosts() []string { return []string{"himalayas.app"} }

// Attribution implements Adapter. Apply links must be applicationLink, unchanged.
func (Himalayas) Attribution() Attribution { return Attribution{Credit: "Himalayas"} }

// FirstURL implements Adapter.
func (Himalayas) FirstURL(string) string { return himalayasFeed }

// Parse implements Adapter.
func (a Himalayas) Parse(_ string, body []byte) (Page, error) {
	var data struct {
		NextCursor string `json:"nextCursor"`
		Jobs       *[]struct {
			GUID        string    `json:"guid"`
			Title       string    `json:"title"`
			Company     string    `json:"companyName"`
			Type        string    `json:"employmentType"`
			Description string    `json:"description"`
			Countries   []string  `json:"locationRestrictions"`
			Zones       []float64 `json:"timezoneRestrictions"`
			Seniority   []string  `json:"seniority"`
			Categories  []string  `json:"categories"`
			Application string    `json:"applicationLink"`
			Published   int64     `json:"pubDate"`
			Expires     int64     `json:"expiryDate"`
			Min         float64   `json:"minSalary"`
			Max         float64   `json:"maxSalary"`
			Currency    string    `json:"currency"`
			Period      string    `json:"salaryPeriod"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return Page{}, decodeError(a.Name(), err)
	}
	if data.Jobs == nil {
		return Page{}, decodeError(a.Name(), fmt.Errorf("no jobs field"))
	}
	var page Page
	// A page can be short before the end, so only an empty page or no cursor stops us.
	if data.NextCursor != "" && len(*data.Jobs) > 0 {
		page.NextURL = himalayasFeed + "&cursor=" + url.QueryEscape(data.NextCursor)
	}
	for _, j := range *data.Jobs {
		if !openToIndia(j.Countries, j.Zones) {
			page.Skipped++
			continue
		}
		keep(&page, Posting{
			SourceJobID: j.GUID, Title: j.Title, Company: j.Company, EmploymentType: j.Type,
			Body: j.Description, BodyIsHTML: true, Remote: RemoteYes, Countries: j.Countries,
			Location: strings.Join(j.Countries, " | "), Seniority: j.Seniority, Tags: j.Categories,
			URL: j.GUID, ApplyURL: j.Application, PostedAt: epoch(j.Published),
			ExpiresAt: epoch(j.Expires), SalaryMin: j.Min, SalaryMax: j.Max,
			SalaryCurrency: j.Currency, SalaryPeriod: j.Period,
		})
	}
	return page, nil
}

// RemoteOK reads the public feed.
type RemoteOK struct{}

// Name implements Adapter.
func (RemoteOK) Name() string { return "remoteok" }

// Hosts implements Adapter.
func (RemoteOK) Hosts() []string { return []string{"remoteok.com"} }

// Attribution implements Adapter. Their terms need a followable link, and no logo.
func (RemoteOK) Attribution() Attribution {
	return Attribution{Credit: "Remote OK", FollowLink: true}
}

// FirstURL implements Adapter.
func (RemoteOK) FirstURL(string) string { return "https://remoteok.com/api" }

// Parse implements Adapter.
func (a RemoteOK) Parse(_ string, body []byte) (Page, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return Page{}, decodeError(a.Name(), err)
	}
	if len(raw) == 0 {
		return Page{}, decodeError(a.Name(), fmt.Errorf("no legal notice element"))
	}
	var page Page
	// Element 0 is the legal notice, not a job.
	for _, item := range raw[1:] {
		var j struct {
			ID          string   `json:"id"`
			Position    string   `json:"position"`
			Company     string   `json:"company"`
			Location    string   `json:"location"`
			Description string   `json:"description"`
			URL         string   `json:"url"`
			Apply       string   `json:"apply_url"`
			Tags        []string `json:"tags"`
			Epoch       int64    `json:"epoch"`
			Min         float64  `json:"salary_min"`
			Max         float64  `json:"salary_max"`
		}
		if err := json.Unmarshal(item, &j); err != nil {
			return Page{}, decodeError(a.Name(), err)
		}
		// Zero means not stated. It must never read as a salary of nothing.
		p := Posting{
			SourceJobID: j.ID, Title: j.Position, Company: j.Company, Location: j.Location,
			Body: j.Description, BodyIsHTML: true, Remote: RemoteYes, URL: j.URL,
			ApplyURL: j.Apply, Tags: j.Tags, PostedAt: epoch(j.Epoch),
		}
		if j.Min > 0 || j.Max > 0 {
			p.SalaryMin, p.SalaryMax, p.SalaryCurrency, p.SalaryPeriod = j.Min, j.Max, "USD", "year"
		}
		keep(&page, p)
	}
	return page, nil
}
