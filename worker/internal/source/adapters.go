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

// Ashby reads the public job posting API.
type Ashby struct{}

// Name implements Adapter.
func (Ashby) Name() string { return "ashby" }

// Hosts implements Adapter.
func (Ashby) Hosts() []string { return []string{"api.ashbyhq.com"} }

// Attribution implements Adapter.
func (Ashby) Attribution() Attribution { return Attribution{Credit: "Ashby"} }

// FirstURL implements Adapter.
func (Ashby) FirstURL(token string) string {
	return "https://api.ashbyhq.com/posting-api/job-board/" + url.PathEscape(token) + "?includeCompensation=true"
}

// Parse implements Adapter.
func (a Ashby) Parse(token string, body []byte) (Page, error) {
	type address struct {
		PostalAddress struct {
			Country string `json:"addressCountry"`
		} `json:"postalAddress"`
	}
	var data struct {
		Jobs *[]struct {
			ID        string  `json:"id"`
			Title     string  `json:"title"`
			Location  string  `json:"location"`
			Listed    *bool   `json:"isListed"`
			IsRemote  *bool   `json:"isRemote"`
			Workplace *string `json:"workplaceType"`
			Type      string  `json:"employmentType"`
			Plain     string  `json:"descriptionPlain"`
			HTML      string  `json:"descriptionHtml"`
			Published string  `json:"publishedAt"`
			JobURL    string  `json:"jobUrl"`
			ApplyURL  string  `json:"applyUrl"`
			Address   address `json:"address"`
			Secondary []struct {
				Location string  `json:"location"`
				Address  address `json:"address"`
			} `json:"secondaryLocations"`
			Compensation *struct {
				Summary []struct {
					Type     string   `json:"compensationType"`
					Interval string   `json:"interval"`
					Currency string   `json:"currencyCode"`
					Min      *float64 `json:"minValue"`
					Max      *float64 `json:"maxValue"`
				} `json:"summaryComponents"`
			} `json:"compensation"`
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
		// The employer chose direct-link-only. We respect that (SOURCES.md).
		if j.Listed != nil && !*j.Listed {
			page.Skipped++
			continue
		}
		p := Posting{
			SourceJobID: j.ID, Title: j.Title, Company: token, Location: j.Location,
			EmploymentType: j.Type, Body: j.Plain, URL: j.JobURL, ApplyURL: j.ApplyURL,
			PostedAt: parseTime(j.Published),
		}
		if p.Body == "" {
			p.Body, p.BodyIsHTML = j.HTML, true
		}
		locations := []string{j.Location}
		countries := []string{j.Address.PostalAddress.Country}
		for _, s := range j.Secondary {
			locations = append(locations, s.Location)
			countries = append(countries, s.Address.PostalAddress.Country)
		}
		p.Location = strings.Join(nonEmpty(locations), " | ")
		p.Countries = nonEmpty(countries)
		switch {
		case j.Workplace != nil && *j.Workplace == "Remote":
			p.Remote = RemoteYes
		case j.Workplace != nil && *j.Workplace == "Hybrid":
			p.Remote = RemoteHybrid
		case j.Workplace != nil && *j.Workplace == "OnSite":
			p.Remote = RemoteOnsite
		case j.IsRemote != nil && *j.IsRemote:
			p.Remote = RemoteYes
		}
		if j.Compensation != nil {
			for _, c := range j.Compensation.Summary {
				if c.Type == "Salary" && c.Min != nil && c.Max != nil {
					p.SalaryMin, p.SalaryMax = *c.Min, *c.Max
					p.SalaryCurrency, p.SalaryPeriod = c.Currency, c.Interval
					break
				}
			}
		}
		keep(&page, p)
	}
	return page, nil
}

// Himalayas reads the public remote jobs feed.
type Himalayas struct{}

// The India-eligible search, not the whole feed: it's the part in scope, and 300 pages
// instead of many thousands. It pages by number, unlike the browse feed's cursor.
const himalayasSearch = "https://himalayas.app/jobs/api/search?country=IN&sort=recent"

// Name implements Adapter.
func (Himalayas) Name() string { return "himalayas" }

// Hosts implements Adapter.
func (Himalayas) Hosts() []string { return []string{"himalayas.app"} }

// Attribution implements Adapter. Apply links must be applicationLink, unchanged.
func (Himalayas) Attribution() Attribution { return Attribution{Credit: "Himalayas"} }

// FirstURL implements Adapter.
func (Himalayas) FirstURL(string) string { return himalayasSearch + "&page=1" }

// Parse implements Adapter.
func (a Himalayas) Parse(_ string, body []byte) (Page, error) {
	var data struct {
		Offset int `json:"offset"`
		Limit  int `json:"limit"`
		Total  int `json:"totalCount"`
		Jobs   *[]struct {
			GUID        string   `json:"guid"`
			Title       string   `json:"title"`
			Company     string   `json:"companyName"`
			Type        string   `json:"employmentType"`
			Description string   `json:"description"`
			Countries   []string `json:"locationRestrictions"`
			Seniority   []string `json:"seniority"`
			Categories  []string `json:"categories"`
			Application string   `json:"applicationLink"`
			Published   int64    `json:"pubDate"`
			Expires     int64    `json:"expiryDate"`
			Min         float64  `json:"minSalary"`
			Max         float64  `json:"maxSalary"`
			Currency    string   `json:"currency"`
			Period      string   `json:"salaryPeriod"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return Page{}, decodeError(a.Name(), err)
	}
	if data.Jobs == nil {
		return Page{}, decodeError(a.Name(), fmt.Errorf("no jobs field"))
	}
	var page Page
	if data.Limit > 0 && len(*data.Jobs) > 0 && data.Offset+data.Limit < data.Total {
		page.NextURL = himalayasSearch + "&page=" + strconv.Itoa(data.Offset/data.Limit+2)
	}
	for _, j := range *data.Jobs {
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
