package leverscan

import (
	"context"
	"errors"
	"testing"

	"github.com/DhruvGoyal404/rolerank/worker/internal/fetch"
)

func TestToken(t *testing.T) {
	for name, want := range map[string]string{
		"Razorpay":                  "razorpay",
		"Zeta Suite Technologies":   "zetasuite",
		"Acme Private Limited":      "acme",
		"Acme Pvt. Ltd.":            "acme",
		"Acme (company)":            "acme",
		"PhonePe":                   "phonepe",
		"Pine Labs":                 "pinelabs",
		"Acme Technologies Pvt Ltd": "acme",
	} {
		if got := Token(name); got != want {
			t.Errorf("Token(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestCategoryPagesReadsOnlyArticles(t *testing.T) {
	body := `<html><body><div id="mw-subcategories"><a href="/wiki/Category:X" title="Category:X">x</a></div>
	<div id="mw-pages"><a href="/wiki/Razorpay" title="Razorpay">Razorpay</a>
	<a href="/wiki/List_of_things" title="List of things">l</a>
	<a href="/wiki/Template:Foo" title="Template:Foo">t</a>
	<a href="/w/index.php?pagefrom=Z" title="Category:Y">next page</a>
	<a href="/wiki/Zerodha" title="Zerodha">Zerodha</a></div>
	<a href="/wiki/Outside" title="Outside">o</a></body></html>`
	got := CategoryPages([]byte(body))
	if len(got) != 2 || got[0].Name != "Razorpay" || got[1].Name != "Zerodha" {
		t.Fatalf("%+v", got)
	}
}

func TestOfficialSite(t *testing.T) {
	body := `<table class="infobox vcard"><tr><th>Founded</th><td>2014</td></tr>
	<tr><th scope="row">Website</th><td><span><a class="external" href="https://razorpay.com/careers?x=1">razorpay.com</a></span></td></tr></table>
	<a href="https://other.example">other</a>`
	if got := OfficialSite([]byte(body)); got != "https://razorpay.com" {
		t.Fatalf("got %q", got)
	}
	if got := OfficialSite([]byte(`<table class="infobox"><tr><th>Founded</th><td>x</td></tr></table>`)); got != "" {
		t.Fatalf("got %q", got)
	}
}

// Meesho's infobox has "Website type" first, which links to Wikipedia.
func TestOfficialSiteIgnoresTheWebsiteTypeRow(t *testing.T) {
	body := `<table class="infobox"><tr><th>Website type</th><td><a href="https://en.wikipedia.org/wiki/Public_company" title="Public company">Public</a></td></tr>
	<tr><th>Website</th><td class="url"><a class="external text" href="https://meesho.com/">meesho.com</a></td></tr></table>`
	if got := OfficialSite([]byte(body)); got != "https://meesho.com" {
		t.Fatalf("got %q", got)
	}
}

func TestListRowsFiltersByCountry(t *testing.T) {
	body := `<table class="wikitable"><tr><th>Company</th><th>Country</th></tr>
	<tr><td><a href="/wiki/Razorpay" title="Razorpay">Razorpay</a></td><td><a href="/wiki/India" title="India">India</a></td></tr>
	<tr><td><a href="/wiki/Stripe,_Inc." title="Stripe, Inc.">Stripe</a></td><td>United States</td></tr>
	<tr><td>No link Ltd</td><td>India</td></tr></table>`
	india := ListRows([]byte(body), true)
	if len(india) != 1 || india[0].Name != "Razorpay" || india[0].Page != wikipedia+"/wiki/Razorpay" {
		t.Fatalf("%+v", india)
	}
	if all := ListRows([]byte(body), false); len(all) != 2 {
		t.Fatalf("%+v", all)
	}
}

func TestLinksToNeedsTheExactBoard(t *testing.T) {
	tests := []struct {
		body string
		want bool
	}{
		{`<a href="https://jobs.lever.co/acme">Jobs</a>`, true},
		{`<a href="https://jobs.lever.co/acme/uuid">Job</a>`, true},
		{`<a href='https://jobs.lever.co/Acme?team=x'>`, true},
		{`<a href="https://jobs.eu.lever.co/acme">`, true},
		{`<a href="https://jobs.lever.co/acme-labs">Jobs</a>`, false},
		{`<a href="https://jobs.lever.co/acmeinc">Jobs</a>`, false},
		{`acme is hiring on Lever`, false},
		{`<a href="https://jobs.lever.co/other">`, false},
	}
	for _, tt := range tests {
		if got := LinksTo([]byte(tt.body), "acme"); got != tt.want {
			t.Errorf("LinksTo(%q) = %v", tt.body, got)
		}
	}
}

type fake map[string]any

func (f fake) Get(_ context.Context, target, _ string) (*fetch.Result, error) {
	switch v := f[target].(type) {
	case error:
		return nil, v
	case string:
		return &fetch.Result{Status: 200, Body: []byte(v)}, nil
	}
	return &fetch.Result{Status: 404}, nil
}

func TestHit(t *testing.T) {
	api := "https://api.lever.co/v0/postings/acme?mode=json&limit=1"
	for body, want := range map[string]bool{`[{"id":"1"}]`: true, `[]`: false, `<html>`: false} {
		got, err := Hit(context.Background(), fake{api: body}, "acme")
		if err != nil || got != want {
			t.Errorf("Hit(%q) = %v, %v", body, got, err)
		}
	}
	if got, err := Hit(context.Background(), fake{}, "acme"); got || err != nil {
		t.Fatalf("a 404 is no hit: %v %v", got, err)
	}
}

func TestVerify(t *testing.T) {
	ctx := context.Background()
	link := `<a href="https://jobs.lever.co/acme">Open roles</a>`
	const site = "https://acme.example"
	tests := []struct {
		name  string
		pages fake
		base  string
		want  Outcome
	}{
		{"linked from careers", fake{"https://www.acme.example/careers": link}, site, Verified},
		{"linked from the home page", fake{"https://acme.example/": link}, site, Verified},
		{"careers subdomain", fake{"https://careers.acme.example/": link}, site, Verified},
		{"reachable but no link", fake{"https://www.acme.example/careers": "<p>Join us</p>"}, site, NotLinked},
		{"no site known", fake{}, "", NoSite},
		{"robots refuses everything", refuse(fetch.ErrDisallowed), site, Blocked},
		{"host down", refuse(errors.New("dial failed")), site, Unreachable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Verify(ctx, tt.pages, tt.base, "acme"); got != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func refuse(err error) fake {
	pages := fake{}
	for _, host := range []string{"www.acme.example", "acme.example", "careers.acme.example"} {
		for _, path := range careerPaths {
			pages["https://"+host+path] = err
		}
	}
	return pages
}

func TestHostsStayOnTheCompanyDomain(t *testing.T) {
	got := Hosts("https://www.acme.example")
	want := []string{"acme.example", "www.acme.example", "careers.acme.example", "jobs.acme.example"}
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%v", got)
		}
	}
}
