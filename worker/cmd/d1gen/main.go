// Command d1gen writes EVALS.md's D1: synthetic resumes whose true contents are known exactly.
// Generation runs backwards: a seeded generator writes the gold JSON, then renders it to a PDF or
// DOCX in one of five template families. The parser never sees the gold.
//
//	d1gen <out-dir>        writes D1: <out-dir>/<id>.pdf|.docx and <out-dir>/gold.jsonl
//	d1gen <out-dir> hard   writes D1-hard, the held-out set (hard.go)
package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"

	"github.com/DhruvGoyal404/rolerank/worker/internal/extract"
)

type experience struct {
	Employer string `json:"employer"`
	Title    string `json:"title"`
	Start    string `json:"start"` // YYYY-MM
	End      string `json:"end"`   // YYYY-MM or "present"
}

type education struct {
	Institution string `json:"institution"`
	Degree      string `json:"degree"`
	GradYear    int    `json:"grad_year"`
	Score       string `json:"score"` // "CGPA 8.40/10" or "82%"
}

type gold struct {
	ID         string       `json:"id"`
	Segment    string       `json:"segment"`
	Template   string       `json:"template"`
	Format     string       `json:"format"`
	Level      string       `json:"level"`
	Name       string       `json:"name"`
	Email      string       `json:"email"`
	Phone      string       `json:"phone"`
	Skills     []string     `json:"skills"`
	Experience []experience `json:"experience"`
	Education  []education  `json:"education"`
	Projects   []string     `json:"projects"`
}

var (
	first     = []string{"Aarav", "Diya", "Rohan", "Ananya", "Kabir", "Isha", "Arjun", "Meera", "Vihaan", "Sara", "Aditya", "Nisha", "Karan", "Pooja", "Rahul", "Sneha"}
	last      = []string{"Sharma", "Verma", "Iyer", "Reddy", "Gupta", "Nair", "Singh", "Patel", "Mehta", "Rao", "Das", "Kulkarni", "Bose", "Chopra"}
	employers = []string{"Infosys", "Tata Consultancy Services", "Wipro", "Zoho", "Freshworks", "Razorpay", "Swiggy", "Flipkart", "Paytm", "Juspay", "Postman", "BrowserStack", "Atlassian", "Thoughtworks", "Cred", "Meesho", "Zeta", "PhonePe"}
	titles    = map[string][]string{
		"entry":  {"Software Engineer", "Associate Software Engineer", "Graduate Engineer Trainee", "Junior Developer"},
		"mid":    {"Software Engineer II", "Backend Developer", "Full Stack Developer", "Data Engineer", "SDE 2"},
		"senior": {"Senior Software Engineer", "Lead Engineer", "Staff Engineer", "Senior Data Engineer"},
		"intern": {"Software Engineering Intern", "Backend Intern", "Data Science Intern"},
	}
	colleges  = []string{"Thapar Institute of Engineering and Technology", "IIT Delhi", "NIT Trichy", "BITS Pilani", "VIT Vellore", "Delhi Technological University", "IIIT Hyderabad", "PEC Chandigarh", "Manipal Institute of Technology", "NSUT Delhi"}
	degrees   = []string{"B.Tech in Computer Science", "B.E. Computer Engineering", "B.Tech in Electronics and Communication", "M.Tech in Computer Science", "MCA", "B.Sc Computer Science"}
	skillPool = []string{"Python", "Java", "C++", "JavaScript", "TypeScript", "Go", "SQL", "React", "Node.js", "Spring Boot", "Django", "FastAPI", "PostgreSQL", "MySQL", "MongoDB", "Redis", "Docker", "Kubernetes", "AWS", "Git", "Linux", "Kafka", "Machine Learning", "Pandas", "TensorFlow", "REST APIs", "GraphQL", "Terraform", "HTML", "CSS"}
	projects  = []string{"Campus Event Portal", "Expense Splitter App", "Library Management System", "Stock Price Predictor", "Chat Application", "URL Shortener", "Online Judge", "Weather Dashboard", "Hostel Mess Feedback System", "Resume Parser", "Food Delivery Clone", "Attendance Tracker"}
	templates = []string{"classic", "modern", "latex", "compact", "twocol"}
	months    = []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
)

func pick[T any](r *rand.Rand, xs []T) T { return xs[r.IntN(len(xs))] }

func sample(r *rand.Rand, xs []string, n int) []string {
	c := append([]string{}, xs...)
	r.Shuffle(len(c), func(i, j int) { c[i], c[j] = c[j], c[i] })
	return c[:min(n, len(c))]
}

func ym(y, m int) string { return fmt.Sprintf("%04d-%02d", y, m) }

// segment follows EVALS.md's mix: 40% fresher and intern, 25% 1-4 years, 15% 5+, 10% hard
// layouts, 10% awkward content.
func segment(i int) string {
	switch p := i % 20; {
	case p < 8:
		return "fresher"
	case p < 13:
		return "experienced"
	case p < 16:
		return "senior"
	case p < 18:
		return "hard_layout"
	}
	return "awkward"
}

func generate(i int, r *rand.Rand) gold {
	seg := segment(i)
	g := gold{ID: fmt.Sprintf("D1-%03d", i+1), Segment: seg, Template: templates[i%len(templates)]}
	if seg == "hard_layout" {
		g.Template = "twocol"
	}
	g.Format = map[bool]string{true: "docx", false: "pdf"}[i%3 == 2]
	fn, ln := pick(r, first), pick(r, last)
	g.Name = fn + " " + ln
	g.Email = strings.ToLower(fn+"."+ln) + fmt.Sprintf("%d@example.com", r.IntN(90)+10)
	g.Phone = fmt.Sprintf("+91 9%04d %05d", r.IntN(10000), r.IntN(100000))
	g.Skills = sample(r, skillPool, 5+r.IntN(6))
	grad := 2026
	years := 0
	switch seg {
	case "fresher":
		g.Level = pick(r, []string{"entry", "intern"})
		if g.Level == "intern" {
			grad = 2027
		}
	case "experienced", "hard_layout":
		g.Level, years = "mid", 1+r.IntN(4)
	case "senior":
		g.Level, years = "senior", 5+r.IntN(6)
	case "awkward":
		g.Level, years = "mid", 2+r.IntN(3)
	}
	grad -= years
	score := fmt.Sprintf("CGPA %.2f/10", 6.5+float64(r.IntN(350))/100)
	if seg == "awkward" {
		score = fmt.Sprintf("%d%%", 60+r.IntN(35)) // percentage instead of CGPA
	}
	g.Education = []education{{Institution: pick(r, colleges), Degree: pick(r, degrees), GradYear: grad, Score: score}}
	// Walk back from today through the roles.
	end, endM := 2026, 10
	for k := 0; years > 0 && k < 3; k++ {
		span := min(years, 1+r.IntN(3))
		start, startM := end-span, 1+r.IntN(12)
		if k == 0 {
			g.Experience = append(g.Experience, experience{pick(r, employers), pick(r, titles[g.Level]), ym(start, startM), "present"})
		} else {
			g.Experience = append(g.Experience, experience{pick(r, employers), pick(r, titles["mid"]), ym(start, startM), ym(end, endM)})
		}
		years -= span
		end, endM = start, max(1, startM-1)
		if seg == "awkward" && k == 0 {
			end-- // a career gap
		}
	}
	if g.Level == "intern" || g.Level == "entry" {
		g.Projects = sample(r, projects, 2+r.IntN(2))
		if g.Level == "intern" {
			g.Experience = []experience{{pick(r, employers), titles["intern"][r.IntN(3)], ym(2026, 5), ym(2026, 7)}}
		}
	} else {
		g.Projects = sample(r, projects, r.IntN(2))
	}
	return g
}

// date renders a YYYY-MM per template: "Mar 2024", "03/2024", "2024-03", or the awkward "Mar'24".
func date(d, template string, awkward bool) string {
	if d == "present" {
		return map[bool]string{true: "Present", false: "Present"}[awkward]
	}
	var y, m int
	fmt.Sscanf(d, "%d-%d", &y, &m)
	switch {
	case awkward:
		return fmt.Sprintf("%s'%02d", months[m-1], y%100)
	case template == "latex":
		return fmt.Sprintf("%s %d", months[m-1], y)
	case template == "compact":
		return fmt.Sprintf("%02d/%d", m, y)
	}
	return fmt.Sprintf("%s %d", months[m-1], y)
}

// render lays out a resume as text lines in the template's order and wording.
func render(g gold) []string {
	aw := g.Segment == "awkward"
	var out []string
	add := func(s ...string) { out = append(out, s...) }
	contact := g.Email + " | " + g.Phone + " | linkedin.com/in/" + strings.ToLower(strings.ReplaceAll(g.Name, " ", "-"))
	heading := map[string][4]string{
		"classic": {"SKILLS", "EXPERIENCE", "EDUCATION", "PROJECTS"},
		"modern":  {"Technical Skills", "Work Experience", "Education", "Projects"},
		"latex":   {"Skills", "Professional Experience", "Education", "Academic Projects"},
		"compact": {"Core Skills", "Employment", "Qualifications", "Personal Projects"},
		"twocol":  {"SKILLS", "EXPERIENCE", "EDUCATION", "PROJECTS"},
	}[g.Template]
	add(g.Name, contact)
	sections := []func(){
		func() { add(heading[0], strings.Join(g.Skills, ", ")) },
		func() {
			if len(g.Experience) == 0 {
				return
			}
			add(heading[1])
			for _, e := range g.Experience {
				span := date(e.Start, g.Template, aw) + " - " + date(e.End, g.Template, aw)
				if g.Template == "compact" {
					add(fmt.Sprintf("%s, %s (%s)", e.Title, e.Employer, span))
				} else {
					add(e.Title+" | "+e.Employer, span)
				}
				add("- Built and maintained services used by internal teams")
			}
		},
		func() {
			add(heading[2])
			for _, e := range g.Education {
				add(fmt.Sprintf("%s, %s, %d", e.Degree, e.Institution, e.GradYear), e.Score)
			}
			if g.Level == "intern" || g.Level == "entry" {
				add("Class XII (CBSE): 91%", "Class X (CBSE): 94%")
			}
		},
		func() {
			if len(g.Projects) == 0 {
				return
			}
			add(heading[3])
			for _, p := range g.Projects {
				add("Project: "+p, "- Built with "+g.Skills[0])
			}
		},
	}
	order := []int{0, 1, 2, 3}
	if g.Level == "intern" || g.Level == "entry" {
		order = []int{3, 0, 2, 1} // freshers lead with projects
	}
	for _, k := range order {
		sections[k]()
	}
	return out
}

func main() {
	if len(os.Args) < 2 || len(os.Args) > 3 || (len(os.Args) == 3 && os.Args[2] != "hard" && os.Args[2] != "hard2" && os.Args[2] != "hard3") {
		fmt.Fprintln(os.Stderr, "usage: d1gen <out-dir> [hard|hard2|hard3]")
		os.Exit(2)
	}
	dir := os.Args[1]
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(err)
	}
	if len(os.Args) == 3 {
		// hard: D1-hard, run once for P4's honest figure. hard2: D1-hard-2, made after the
		// parse-v3 fix and held out to measure it.
		if os.Args[2] == "hard3" {
			generateHard3(dir) // real-resume patterns, 2026-11-06
			return
		}
		if os.Args[2] == "hard2" {
			generateHard(dir, 20261107, "D1H2")
		} else {
			generateHard(dir, 20261106, "D1H")
		}
		return
	}
	r := rand.New(rand.NewPCG(20261105, 1))
	gf, err := os.Create(filepath.Join(dir, "gold.jsonl"))
	if err != nil {
		panic(err)
	}
	defer gf.Close()
	enc := json.NewEncoder(gf)
	for i := range 200 {
		g := generate(i, r)
		lines := render(g)
		var file []byte
		if g.Format == "docx" {
			var ps [][]extract.DocxRun
			for _, l := range lines {
				ps = append(ps, []extract.DocxRun{{Text: l}})
			}
			file = extract.MakeDOCX(ps)
		} else {
			var ls []extract.PDFLine
			for _, l := range lines {
				ls = append(ls, extract.PDFLine{Text: l})
			}
			if g.Template == "twocol" {
				ls = twoColumns(ls)
			}
			file = extract.MakePDF(ls)
		}
		if err := os.WriteFile(filepath.Join(dir, g.ID+"."+g.Format), file, 0o644); err != nil {
			panic(err)
		}
		if err := enc.Encode(g); err != nil {
			panic(err)
		}
	}
	fmt.Println("wrote 200 resumes to", dir)
}

// twoColumns puts the skills section in a left sidebar beside the rest, the hard layout a
// reader has to untangle: each sidebar line shares a row with a main line.
func twoColumns(ls []extract.PDFLine) []extract.PDFLine {
	var side, main []extract.PDFLine
	for i, l := range ls {
		if i >= 2 && i <= 3 {
			side = append(side, l)
		} else {
			main = append(main, l)
		}
	}
	for i := range side {
		if i < len(main) {
			main[i].Text = side[i].Text + "    " + main[i].Text
		}
	}
	return main
}
