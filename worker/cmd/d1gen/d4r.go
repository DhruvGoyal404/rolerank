package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/DhruvGoyal404/rolerank/worker/internal/extract"
	"github.com/DhruvGoyal404/rolerank/worker/internal/skills"
)

// D4-R (2026-11-08): held-out cases for re-scoped tailoring (evidence map, terminology
// alignment, emphasis), written and committed before the first run. D4 and D4-2 used D1's thin
// lines ("Built with SQL"); these bullets are realistic: multi-clause, with stacks and metrics,
// some written with aliases ("Golang", "Postgres", "k8s"), some with no number at all. Every
// bullet's skills are written by hand and checked against the Go matcher here, so the gold and
// the taxonomy agree before anything runs. A third of the resumes are PDFs whose long bullets wrap
// onto a second line, as real ones do.

type rbullet struct {
	text  string
	forms map[string]string // canonical skill -> how this bullet writes it
}

var rbullets = []rbullet{
	{"Built a payments reconciliation service in Golang and Postgres that processes 1.2M transactions a day, cutting manual review by 40%", map[string]string{"Go": "Golang", "PostgreSQL": "Postgres"}},
	{"Moved the order API to Node.js services on k8s, bringing p95 latency down from 900 ms to 220 ms during the festive sale", map[string]string{"Node.js": "Node.js", "Kubernetes": "k8s"}},
	{"Wrote a React.js dashboard for warehouse managers that replaced three spreadsheets and is used by 60 staff daily", map[string]string{"React": "React.js"}},
	{"Designed the event pipeline on Apache Kafka and Redis that fans out 3,000 notifications a minute with retries", map[string]string{"Kafka": "Apache Kafka", "Redis": "Redis"}},
	{"Automated test and deploy steps with GitHub Actions and Docker, so a release takes 12 minutes instead of an afternoon", map[string]string{"GitHub Actions": "GitHub Actions", "Docker": "Docker"}},
	{"Trained a churn model with sklearn and Pandas on two years of subscription data, lifting retention offers' hit rate by 18%", map[string]string{"scikit-learn": "sklearn", "Pandas": "Pandas"}},
	{"Rewrote the search backend in Python with FastAPI and Elasticsearch, serving 400 queries a second at peak", map[string]string{"Python": "Python", "FastAPI": "FastAPI", "Elasticsearch": "Elasticsearch"}},
	{"Set up Terraform modules for the staging and production VPCs on Amazon Web Services and documented how to use them", map[string]string{"Terraform": "Terraform", "AWS": "Amazon Web Services"}},
	{"Added structured logging and Grafana dashboards to the billing services, which the on-call team now uses for every incident", map[string]string{"Grafana": "Grafana"}},
	{"Built REST endpoints in Spring Boot for the loan application flow and wrote the integration tests that run on every merge", map[string]string{"Spring Boot": "Spring Boot", "REST APIs": "REST", "Integration Testing": "integration tests"}},
	{"Ported the image upload path to TS and added retries, which removed the most common support complaint", map[string]string{"TypeScript": "TS"}},
	{"Tuned slow MongoDB queries behind the catalogue pages and added the missing indexes, after which the pages stopped timing out", map[string]string{"MongoDB": "MongoDB"}},
	{"Wrote the data loaders for a PyTorch image classifier and ran the experiments that picked the final model", map[string]string{"PyTorch": "PyTorch"}},
	{"Mentored two interns through their first production changes and reviewed their pull requests every week", map[string]string{}},
	{"Built a GraphQL gateway over four internal services with Node.js, cutting the mobile app's calls per screen from 7 to 2", map[string]string{"GraphQL": "GraphQL", "Node.js": "Node.js"}},
	{"Migrated nightly batch jobs to Google Cloud and Golang workers, saving about Rs 40,000 a month in compute", map[string]string{"Google Cloud": "Google Cloud", "Go": "Golang"}},
	{"Wrote a JS widget for the partner portal that shows live delivery status, now embedded on 30 partner sites", map[string]string{"JavaScript": "JS"}},
	{"Maintained the RabbitMQ consumers that process refunds and fixed the duplicate-refund bug that had cost money each week", map[string]string{"RabbitMQ": "RabbitMQ"}},
}

type rlist struct {
	label string
	items []string // as written; checked against the matcher like bullets
}

var rlists = []rlist{
	{"Languages", []string{"Python", "Golang", "JavaScript", "SQL"}},
	{"Backend", []string{"Node.js", "FastAPI", "Spring Boot", "GraphQL"}},
	{"Data and infra", []string{"Postgres", "MongoDB", "Redis", "Docker", "k8s"}},
	{"Modelling", []string{"PyTorch", "sklearn", "Pandas"}},
}

type d4rLine struct {
	Text   string            `json:"text"`
	Forms  map[string]string `json:"forms"` // canonical -> surface, as written
	List   bool              `json:"list"`
	Metric bool              `json:"metric"` // holds a number
}

type d4rCase struct {
	ID       string            `json:"id"`
	Format   string            `json:"format"`
	Fit      string            `json:"fit"`
	Name     string            `json:"name"`
	Email    string            `json:"email"`
	Phone    string            `json:"phone"`
	Lines    []d4rLine         `json:"lines"` // bullets and skills lists, as the resume writes them
	JD       string            `json:"jd"`
	Required []string          `json:"required"`
	Nice     []string          `json:"nice"`
	Pressure []string          `json:"pressure"`
	JDForms  map[string]string `json:"jd_forms"` // how the JD writes each skill
}

func checkForms(t *skills.Taxonomy, text string, forms map[string]string) {
	got := t.Find(text)
	want := make([]string, 0, len(forms))
	for k, v := range forms {
		want = append(want, k)
		if !strings.Contains(text, v) {
			panic(fmt.Sprintf("%q is not written in %q", v, text))
		}
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		panic(fmt.Sprintf("gold %v but the matcher finds %v in %q", want, got, text))
	}
}

func generateD4R(dir string) {
	t := skills.Default
	for _, b := range rbullets {
		checkForms(t, b.text, b.forms)
	}
	r := rand.New(rand.NewPCG(20261111, 8))
	f, err := os.Create(filepath.Join(dir, "cases.jsonl"))
	if err != nil {
		panic(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for i := range 30 {
		fn, ln := pick(r, first), pick(r, last)
		c := d4rCase{ID: fmt.Sprintf("D4R-%03d", i+1), Name: fn + " " + ln, Format: []string{"docx", "docx", "pdf"}[i%3]}
		c.Email = strings.ToLower(fn+"."+ln) + "@example.com"
		c.Phone = fmt.Sprintf("+91 9%04d %05d", r.IntN(10000), r.IntN(100000))
		var have []string
		for _, b := range sampleBullets(r, 7) {
			c.Lines = append(c.Lines, d4rLine{Text: "- " + b.text, Forms: b.forms, Metric: strings.ContainsAny(b.text, "0123456789")})
			for k := range b.forms {
				if !slices.Contains(have, k) {
					have = append(have, k)
				}
			}
		}
		l := rlists[r.IntN(len(rlists))]
		forms := map[string]string{}
		for _, it := range l.items {
			forms[t.Find(it)[0]] = it
		}
		listText := l.label + ": " + strings.Join(l.items, ", ")
		checkForms(t, listText, forms)
		c.Lines = append(c.Lines, d4rLine{Text: listText, Forms: forms, List: true})
		for k := range forms {
			if !slices.Contains(have, k) {
				have = append(have, k)
			}
		}
		slices.Sort(have)
		text := strings.ToLower(strings.Join(lineTexts(c.Lines), "\n"))
		absent := func(s string) bool { return len(t.Find(text+" "+s)) > len(t.Find(text)) }
		c.JDForms = map[string]string{}
		if i%5 == 4 {
			c.Fit = "nofit"
			j := noFitJobs[r.IntN(len(noFitJobs))]
			c.Required, c.Pressure = slices.Clone(j.req), slices.Clone(j.req)
			c.JD = j.title + "\nRequirements\n- " + strings.Join(j.req, ", ")
		} else {
			c.Fit = "fit"
			c.Required = sample(r, have, min(3, len(have)))
			for _, s := range sample(r, skillPool, len(skillPool)) {
				if len(c.Pressure) < 1+r.IntN(2) && absent(s) {
					c.Pressure = append(c.Pressure, s)
				}
			}
			c.Required = append(c.Required, c.Pressure...)
			for _, s := range sample(r, have, len(have)) {
				if !slices.Contains(c.Required, s) && len(c.Nice) < 1 {
					c.Nice = append(c.Nice, s)
				}
			}
			// The JD names each skill canonically, but now and then in the resume's own alias.
			written := func(s string) string {
				w := s
				for _, ln := range c.Lines {
					if v, ok := ln.Forms[s]; ok && v != s && r.IntN(4) == 0 {
						w = v
						break
					}
				}
				c.JDForms[s] = w
				return w
			}
			var req, nice []string
			for _, s := range c.Required {
				req = append(req, written(s))
			}
			for _, s := range c.Nice {
				nice = append(nice, written(s))
			}
			c.JD = pick(r, d4Titles) + "\nRequirements\n- " + strings.Join(req, "\n- ") + "\nNice to have\n- " + strings.Join(nice, "\n- ")
		}
		for _, s := range c.Pressure {
			if !absent(s) {
				panic(fmt.Sprintf("%s: pressure skill %s is in the resume", c.ID, s))
			}
		}
		if err := os.WriteFile(filepath.Join(dir, c.ID+"."+c.Format), renderD4R(c), 0o644); err != nil {
			panic(err)
		}
		if err := enc.Encode(c); err != nil {
			panic(err)
		}
	}
	fmt.Println("wrote 30 realistic tailoring cases to", dir)
}

func sampleBullets(r *rand.Rand, n int) []rbullet {
	idx := r.Perm(len(rbullets))[:n]
	out := make([]rbullet, 0, n)
	for _, i := range idx {
		out = append(out, rbullets[i])
	}
	return out
}

func lineTexts(ls []d4rLine) []string {
	out := make([]string, 0, len(ls))
	for _, l := range ls {
		out = append(out, l.Text)
	}
	return out
}

// renderD4R lays out a one-page resume: header, education, two roles and a project with their
// bullets, then the skills list. In PDF, a bullet longer than about 95 characters wraps.
func renderD4R(c d4rCase) []byte {
	head := []string{c.Name, c.Email + " | " + c.Phone, "EDUCATION", "B.Tech in Computer Science, NIT Trichy, 2025", "EXPERIENCE",
		"Software Engineer, Razorpay", "Jul 2025 - Present"}
	var body []string
	for k, l := range c.Lines {
		switch k {
		case 3:
			body = append(body, "Software Engineering Intern, Zoho", "Jan 2025 - Jun 2025")
		case 5:
			body = append(body, "PROJECTS", "Campus Ride Share")
		case len(c.Lines) - 1:
			body = append(body, "SKILLS")
		}
		body = append(body, l.Text)
	}
	all := append(head, body...)
	if c.Format == "docx" {
		var ps [][]extract.DocxRun
		for _, l := range all {
			ps = append(ps, []extract.DocxRun{{Text: l}})
		}
		return extract.MakeDOCX(ps)
	}
	var ls []extract.PDFLine
	for _, l := range all {
		for len(l) > 95 {
			cut := strings.LastIndex(l[:95], " ")
			ls = append(ls, extract.PDFLine{Text: l[:cut]})
			l = "  " + l[cut+1:]
		}
		ls = append(ls, extract.PDFLine{Text: l})
	}
	return extract.MakePDF(ls)
}
