package source

import (
	"strings"
	"testing"
	"time"
)

// Fixtures are synthetic, with the field shapes seen live on 2026-10-31. Real job text
// stays out of the repo (docs/EVALS.md, "Data rules").

func parse(t *testing.T, a Adapter, token, body string) Page {
	t.Helper()
	page, err := a.Parse(token, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func TestGreenhouse(t *testing.T) {
	page := parse(t, Greenhouse{}, "acme", `{"jobs":[{
	  "id":101,"internal_job_id":55,"title":" Software Engineer ","company_name":"Acme",
	  "absolute_url":"https://job-boards.greenhouse.io/acme/jobs/101","language":"en",
	  "updated_at":"2026-10-29T10:42:09-04:00","first_published":"2026-09-09T10:50:29-04:00",
	  "application_deadline":null,"location":{"name":"Bengaluru, India"},
	  "content":"&lt;p&gt;Build things&lt;/p&gt;"
	}],"meta":{"total":1}}`)
	if len(page.Postings) != 1 {
		t.Fatalf("%+v", page)
	}
	p := page.Postings[0]
	if p.SourceJobID != "101" || p.GroupID != "55" || p.Title != "Software Engineer" ||
		p.Company != "Acme" || p.Location != "Bengaluru, India" || !p.BodyIsHTML {
		t.Fatalf("%+v", p)
	}
	if !p.UpdatedAt.Equal(time.Date(2026, 10, 29, 14, 42, 9, 0, time.UTC)) {
		t.Fatalf("UpdatedAt %v", p.UpdatedAt)
	}
	if !p.ExpiresAt.IsZero() {
		t.Fatalf("a null deadline must stay empty, got %v", p.ExpiresAt)
	}
}

func TestGreenhouseProspectPostHasNoGroup(t *testing.T) {
	page := parse(t, Greenhouse{}, "acme", `{"jobs":[{"id":1,"internal_job_id":null,"title":"T",
	  "absolute_url":"https://x.example/1","location":{"name":"X"}}]}`)
	if page.Postings[0].GroupID != "" {
		t.Fatal("no group expected")
	}
}

func TestLever(t *testing.T) {
	page := parse(t, Lever{}, "acme", `[{
	  "id":"uuid-1","text":"Backend Intern","country":"in","workplaceType":"remote",
	  "createdAt":1772701710400,"hostedUrl":"https://jobs.lever.co/acme/uuid-1",
	  "applyUrl":"https://jobs.lever.co/acme/uuid-1/apply","openingPlain":"Hello",
	  "descriptionPlain":"Work","additionalPlain":"Benefits",
	  "categories":{"location":"Pune","commitment":"Intern","allLocations":["Pune","Delhi"]},
	  "lists":[{"text":"You have","content":"<li>Go</li>"}],
	  "salaryRange":{"min":1,"max":2,"currency":"INR","interval":"per-year-salary"}
	}]`)
	p := page.Postings[0]
	if p.Countries[0] != "IN" || p.Remote != RemoteYes || p.EmploymentType != "Intern" {
		t.Fatalf("%+v", p)
	}
	if p.Location != "Pune | Delhi" || p.PostedAt.Year() != 2026 || p.SalaryCurrency != "INR" {
		t.Fatalf("%+v", p)
	}
	for _, part := range []string{"Hello", "Work", "You have", "<li>Go</li>", "Benefits"} {
		if !strings.Contains(p.Body, part) {
			t.Fatalf("body lost %q: %q", part, p.Body)
		}
	}
	if !p.BodyIsHTML {
		t.Fatal("list items are HTML")
	}
}

func TestAshby(t *testing.T) {
	page := parse(t, Ashby{}, "acme", `{"jobs":[
	 {"id":"a1","title":"SDE","location":"Pune","isListed":true,"isRemote":null,"workplaceType":null,
	  "employmentType":"FullTime","descriptionPlain":"Work","publishedAt":"2026-10-01T00:00:00.000+00:00",
	  "jobUrl":"https://jobs.ashbyhq.com/acme/a1","applyUrl":"https://jobs.ashbyhq.com/acme/a1/application",
	  "address":{"postalAddress":{"addressCountry":"India"}},
	  "secondaryLocations":[{"location":"Remote","address":{"postalAddress":{"addressCountry":"Canada"}}}],
	  "compensation":{"summaryComponents":[
	    {"compensationType":"Bonus","interval":"1 YEAR","currencyCode":"INR","minValue":1,"maxValue":2},
	    {"compensationType":"Salary","interval":"1 YEAR","currencyCode":"INR","minValue":10,"maxValue":20}]}},
	 {"id":"a2","title":"Hidden","isListed":false,"jobUrl":"https://jobs.ashbyhq.com/acme/a2"},
	 {"id":"a3","title":"Remote one","isListed":true,"workplaceType":"Remote","descriptionHtml":"<p>x</p>",
	  "jobUrl":"https://jobs.ashbyhq.com/acme/a3","compensation":{"summaryComponents":[
	    {"compensationType":"Salary","minValue":null,"maxValue":null}]}}
	]}`)
	if len(page.Postings) != 2 || page.Skipped != 1 {
		t.Fatalf("unlisted jobs must be skipped: %+v", page)
	}
	p := page.Postings[0]
	if p.Location != "Pune | Remote" || p.Remote != RemoteUnknown || p.SalaryMin != 10 || len(p.Countries) != 2 {
		t.Fatalf("%+v", p)
	}
	second := page.Postings[1]
	if second.Remote != RemoteYes || !second.BodyIsHTML || second.SalaryMax != 0 {
		t.Fatalf("%+v", second)
	}
}

func TestHimalayas(t *testing.T) {
	page := parse(t, Himalayas{}, "", `{"nextCursor":"ab c=","jobs":[{
	  "guid":"https://himalayas.app/companies/acme/jobs/sde","title":"SDE","companyName":"Acme",
	  "description":"<p>x</p>","locationRestrictions":["India"],"seniority":["Entry-level"],
	  "categories":["Engineering"],"applicationLink":"https://himalayas.app/apply/1",
	  "pubDate":1793440842,"expiryDate":1798624841,"minSalary":0,"maxSalary":0,"timezoneRestrictions":[5.5]
	}]}`)
	p := page.Postings[0]
	if p.ApplyURL != "https://himalayas.app/apply/1" || p.ExpiresAt.IsZero() || p.Remote != RemoteYes {
		t.Fatalf("%+v", p)
	}
	if page.NextURL != himalayasFeed+"&cursor=ab+c%3D" {
		t.Fatalf("next %q", page.NextURL)
	}
	last := parse(t, Himalayas{}, "", `{"nextCursor":"","jobs":[]}`)
	if last.NextURL != "" {
		t.Fatal("no next page expected")
	}
}

func TestRemoteOK(t *testing.T) {
	page := parse(t, RemoteOK{}, "", `[
	 {"last_updated":1793376015,"legal":"API Terms of Service"},
	 {"id":"1137604","position":"Dev","company":"Acme","location":"","description":"<p>x</p>",
	  "url":"https://remoteOK.com/remote-jobs/1137604","apply_url":"https://remoteOK.com/remote-jobs/1137604",
	  "tags":["dev"],"epoch":1793347212,"salary_min":0,"salary_max":0},
	 {"id":"2","position":"Paid","company":"Acme","url":"https://remoteok.com/2","salary_min":50000,"salary_max":90000}
	]`)
	if len(page.Postings) != 2 {
		t.Fatalf("the legal notice must not become a job: %+v", page)
	}
	if page.Postings[0].SalaryCurrency != "" {
		t.Fatal("a zero salary means not stated")
	}
	if page.Postings[1].SalaryMax != 90000 {
		t.Fatalf("%+v", page.Postings[1])
	}
	if !(RemoteOK{}).Attribution().FollowLink {
		t.Fatal("their terms need a followable link")
	}
}

func TestMalformedResponsesAreErrorsNeverEmptyLists(t *testing.T) {
	cases := map[string][]string{
		"greenhouse": {`<html>`, `{}`, `{"jobs":null}`, ``},
		"lever":      {`<html>`, `null`, `{"ok":false}`, ``},
		"ashby":      {`<html>`, `{}`, `{"jobs":null}`},
		"himalayas":  {`<html>`, `{}`},
		"remoteok":   {`<html>`, `[]`, `[{},"x"]`},
	}
	for _, a := range All() {
		for _, body := range cases[a.Name()] {
			if _, err := a.Parse("t", []byte(body)); err == nil {
				t.Errorf("%s accepted %q", a.Name(), body)
			}
		}
	}
}

func TestEmptyBoardsAreNotErrors(t *testing.T) {
	bodies := map[string]string{
		"greenhouse": `{"jobs":[]}`, "lever": `[]`, "ashby": `{"jobs":[]}`,
		"himalayas": `{"jobs":[]}`, "remoteok": `[{"legal":"x"}]`,
	}
	for _, a := range All() {
		page := parse(t, a, "t", bodies[a.Name()])
		if len(page.Postings) != 0 {
			t.Errorf("%s: %+v", a.Name(), page)
		}
	}
}

func TestPostingsMissingRequiredFieldsAreDropped(t *testing.T) {
	page := parse(t, Greenhouse{}, "acme", `{"jobs":[
	  {"id":0,"title":"No id","absolute_url":"https://x.example/1"},
	  {"id":2,"title":"  ","absolute_url":"https://x.example/2"},
	  {"id":3,"title":"Plain http","absolute_url":"http://x.example/3"},
	  {"id":4,"title":"Script","absolute_url":"javascript:alert(1)"},
	  {"id":5,"title":"Good","absolute_url":"https://x.example/5"}
	]}`)
	if len(page.Postings) != 1 || page.Dropped != 4 {
		t.Fatalf("%+v", page)
	}
}

func TestBadApplyURLIsCleared(t *testing.T) {
	page := parse(t, Lever{}, "acme", `[{"id":"1","text":"T","hostedUrl":"https://x.example/1","applyUrl":"javascript:alert(1)"}]`)
	if page.Postings[0].ApplyURL != "" {
		t.Fatal("an unsafe apply link must not survive")
	}
}

func TestBoardTokensAreEscaped(t *testing.T) {
	for _, a := range []Adapter{Greenhouse{}, Lever{}, Ashby{}} {
		if got := a.FirstURL("../../evil?x=1"); strings.Contains(got, "../") || strings.Contains(got, "evil?") {
			t.Errorf("%s built %q", a.Name(), got)
		}
	}
}

func TestEveryAdapterDeclaresHostsAndCredit(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range All() {
		if seen[a.Name()] || len(a.Hosts()) == 0 || a.Attribution().Credit == "" {
			t.Errorf("%s is incomplete", a.Name())
		}
		seen[a.Name()] = true
		if !strings.Contains(a.FirstURL("acme"), a.Hosts()[0]) {
			t.Errorf("%s requests a host it doesn't declare", a.Name())
		}
	}
}
