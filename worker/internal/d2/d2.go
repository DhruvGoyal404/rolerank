// Package d2 generates EVALS.md's D2: synthetic profiles and job descriptions whose skills,
// level and location are known, graded by a rule. It catches ranking regressions; it is never
// a measure of real quality (that is D3).
package d2

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
)

// The set EVALS.md specifies: 60 profiles x 400 jobs, from a fixed seed.
const (
	Seed     = 20261105
	Profiles = 60
	Jobs     = 400
)

// Fields and their core skills, all canonical names from the taxonomy.
var fields = map[string][]string{
	"backend":  {"Go", "Java", "Python", "PostgreSQL", "Redis", "Kafka", "REST APIs", "Microservices", "Docker", "Spring Boot", "gRPC", "AWS"},
	"frontend": {"React", "TypeScript", "JavaScript", "HTML", "CSS", "Next.js", "Redux", "Tailwind CSS", "Jest", "Webpack", "GraphQL"},
	"data":     {"Python", "SQL", "Apache Spark", "Apache Airflow", "dbt", "Snowflake", "Kafka", "ETL", "BigQuery", "Data Warehousing"},
	"ml":       {"Python", "PyTorch", "TensorFlow", "Machine Learning", "Deep Learning", "scikit-learn", "Pandas", "NumPy", "MLOps", "Natural Language Processing"},
	"devops":   {"Kubernetes", "Docker", "Terraform", "AWS", "CI/CD", "Linux", "Prometheus", "Grafana", "Helm", "Ansible", "Bash"},
	"mobile":   {"Kotlin", "Swift", "Android", "iOS", "Flutter", "React Native", "Jetpack Compose", "SwiftUI", "Firebase"},
	"qa":       {"Selenium", "Cypress", "Playwright", "Test Automation", "Java", "Python", "API Testing", "JMeter", "Postman"},
	"security": {"Application Security", "Penetration Testing", "OWASP", "Burp Suite", "SIEM", "Incident Response", "Python", "Cloud Security", "Threat Modeling"},
}

var fieldTitle = map[string]string{"backend": "Backend Engineer", "frontend": "Frontend Engineer", "data": "Data Engineer",
	"ml": "Machine Learning Engineer", "devops": "DevOps Engineer", "mobile": "Mobile Engineer", "qa": "QA Automation Engineer",
	"security": "Security Engineer"}

var levelTitle = map[string]string{"intern": "Intern", "entry": "Junior", "mid": "", "senior": "Senior"}
var levelYears = map[string]string{"intern": "Open to students graduating in 2027.", "entry": "0-1 years of experience; freshers welcome.",
	"mid": "3+ years of professional experience.", "senior": "6+ years of professional experience."}

// Places a job can be open to: on site in India, on site in the US, or remote worldwide.
var places = []string{"IN", "US", "remote"}

// Profile is a synthetic candidate.
type Profile struct {
	ID     string   `json:"id"`
	Field  string   `json:"field"`
	Level  string   `json:"level"`
	Skills []string `json:"skills"`
	Remote bool     `json:"remote"`
	Text   string   `json:"text"` // what the profile embedding is made from
}

// Job is a synthetic job description.
type Job struct {
	ID       string   `json:"id"`
	Field    string   `json:"field"`
	Level    string   `json:"level"`
	Place    string   `json:"place"`
	Required []string `json:"required"`
	Nice     []string `json:"nice"`
	Title    string   `json:"title"`
	Body     string   `json:"body"`
}

var levels = []string{"intern", "entry", "mid", "senior"}

func names(m map[string][]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func pick(r *rand.Rand, from []string, n int) []string {
	c := slices.Clone(from)
	r.Shuffle(len(c), func(i, j int) { c[i], c[j] = c[j], c[i] })
	return c[:min(n, len(c))]
}

// Generate builds the set from a seed. The same seed gives the same set.
func Generate(seed uint64, nProfiles, nJobs int) ([]Profile, []Job) {
	r := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	fs := names(fields)
	var ps []Profile
	for i := range nProfiles {
		f := fs[i%len(fs)]
		lv := levels[r.IntN(len(levels))]
		sk := pick(r, fields[f], 4+r.IntN(4))
		other := fields[fs[r.IntN(len(fs))]]
		for _, s := range pick(r, other, 1) {
			if !slices.Contains(sk, s) {
				sk = append(sk, s)
			}
		}
		ps = append(ps, Profile{ID: fmt.Sprintf("P%02d", i+1), Field: f, Level: lv, Skills: sk, Remote: r.IntN(2) == 0,
			Text: strings.TrimSpace(levelTitle[lv]+" "+fieldTitle[f]) + ". Skills: " + strings.Join(sk, ", ")})
	}
	var js []Job
	for i := range nJobs {
		f := fs[r.IntN(len(fs))]
		lv := levels[r.IntN(len(levels))]
		req := pick(r, fields[f], 3+r.IntN(4))
		var nice []string
		for _, s := range pick(r, fields[f], 3) {
			if !slices.Contains(req, s) {
				nice = append(nice, s)
			}
		}
		place := places[r.IntN(len(places))]
		title := strings.TrimSpace(levelTitle[lv] + " " + fieldTitle[f])
		where := map[string]string{"IN": "Bengaluru, India", "US": "Austin, United States", "remote": "Remote, worldwide"}[place]
		body := fmt.Sprintf("We are hiring a %s in %s.\nRequirements\n- %s\n- %s\nNice to have\n- %s\n",
			title, where, levelYears[lv], strings.Join(req, "\n- "), strings.Join(nice, "\n- "))
		js = append(js, Job{ID: fmt.Sprintf("J%03d", i+1), Field: f, Level: lv, Place: place, Required: req, Nice: nice,
			Title: title, Body: body})
	}
	return ps, js
}

// Country every synthetic profile can work in on site.
const Country = "IN"

// Open says whether a job is open to a profile: on site in India, or remote for a profile that
// accepts remote work.
func Open(p Profile, j Job) bool {
	return j.Place == Country || (p.Remote && j.Place == "remote")
}

// LevelFits says whether a job's level is in the profile's window.
func LevelFits(p Profile, j Job) bool {
	w := map[string][]string{"intern": {"intern"}, "entry": {"entry"}, "mid": {"entry", "mid"}, "senior": {"mid", "senior"}}
	return slices.Contains(w[p.Level], j.Level)
}

// Grade is EVALS.md's rubric made a rule: 3 would apply today, 2 plausible stretch, 1 same field
// but wrong level or stack, 0 irrelevant or ineligible.
func Grade(p Profile, j Job) int {
	if !Open(p, j) || p.Field != j.Field {
		return 0
	}
	if !LevelFits(p, j) {
		return 1
	}
	have := 0
	for _, s := range j.Required {
		if slices.Contains(p.Skills, s) {
			have++
		}
	}
	switch cov := float64(have) / float64(len(j.Required)); {
	case cov >= 0.6:
		return 3
	case cov >= 0.3:
		return 2
	}
	return 1
}
