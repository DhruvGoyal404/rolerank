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

// D1-hard-3: held-out cases for the patterns the owner's real resume exposed (2026-11-06),
// written before the fix and run once after it.
//   - an entry with two date ranges on one line ("Aug 2025 – Sept 2025 & Jul 2026 – Aug 2026");
//   - programmes, summer schools, bootcamps and courses, which are not work experience;
//   - titles with superscript ordinals ("2nd", drawn with "nd" raised);
//   - "Research & Publications" and "Hackathons, Achievements & Certifications" sections;
//   - skills named only in project tech stacks and research lines;
//   - school boards (ISC, ICSE, CBSE, HSC, SSC, Higher Secondary, Intermediate, 10th/12th),
//     which are not degrees;
//   - the abbreviations OS, DBMS, CN, OOPS, DSA and GNN.

type period struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type entry struct {
	Employer string   `json:"employer"`
	Title    string   `json:"title"`
	Kind     string   `json:"kind"` // job, internship or programme
	Periods  []period `json:"periods"`
}

type gold3 struct {
	ID             string      `json:"id"`
	Segment        string      `json:"segment"`
	Template       string      `json:"template"`
	Format         string      `json:"format"`
	Level          string      `json:"level"`
	Name           string      `json:"name"`
	Email          string      `json:"email"`
	Phone          string      `json:"phone"`
	Skills         []string    `json:"skills"` // every skill, wherever the resume names it
	Experience     []entry     `json:"experience"`
	Education      []education `json:"education"` // degrees only: school rows are not here
	Projects       []string    `json:"projects"`
	Publications   []string    `json:"publications"`
	Achievements   []string    `json:"achievements"`
	Certifications []string    `json:"certifications"`
	Months         int         `json:"months"` // jobs and internships, overlaps counted once
}

var (
	programmes = []string{"ML Summer School", "Google Summer of Code Bootcamp", "AWS Cloud Bootcamp", "Deep Learning Specialization Course", "Microsoft Learn Student Program"}
	ordinals   = []struct{ n, sup string }{{"2", "nd"}, {"3", "rd"}, {"1", "st"}, {"4", "th"}}
	boards     = []struct{ degree, inst string }{
		{"ISC Intermediate PCM with Computer Science", "Modern Academy Lucknow"},
		{"ICSE Class X", "St. Joseph's School"},
		{"CBSE Class XII", "Delhi Public School"},
		{"HSC (Maharashtra Board), Science", "Fergusson Junior College"},
		{"SSC", "Kendriya Vidyalaya"},
		{"Higher Secondary (12th), PCM", "Govt. Senior Secondary School"},
	}
	stackSkills    = []string{"MediaPipe", "TensorRT", "GNN", "Suricata", "DPDK", "OpenCV", "FastAPI", "Socket.io", "Redis", "Docker"}
	researchSkills = []string{"NLP", "LLMs", "XLM-RoBERTa", "CRF", "Chain-of-Thought", "Transformers"}
	coreAbbrev     = []string{"OS", "DBMS", "CN", "OOPS", "DSA"}
	venues         = []string{"IEEE ICACCS 2026", "ACL Student Research Workshop 2026", "Springer LNNS 2025", "arXiv preprint 2026"}
	hackathons     = []string{"Winner, Smart India Hackathon 2025", "Finalist, HackMIT 2025", "2nd place, Code for Bharat 2026", "Top 10, Flipkart GRiD 6.0"}
	certs          = []string{"AWS Certified Cloud Practitioner", "Google Data Analytics Certificate", "NPTEL Deep Learning (Elite)", "Microsoft Azure Fundamentals AZ-900"}
)

func monthsOf(es []entry) int {
	type span struct{ a, b int }
	var spans []span
	toM := func(d string) int {
		if d == "present" {
			return 2026*12 + 10
		}
		var y, m int
		fmt.Sscanf(d, "%d-%d", &y, &m)
		return y*12 + m - 1
	}
	for _, e := range es {
		if e.Kind == "programme" {
			continue
		}
		for _, p := range e.Periods {
			spans = append(spans, span{toM(p.Start), toM(p.End)})
		}
	}
	for i := range spans {
		for j := i + 1; j < len(spans); j++ {
			if spans[j].a < spans[i].a {
				spans[i], spans[j] = spans[j], spans[i]
			}
		}
	}
	total, ca, cb := 0, -1, -1
	for _, s := range spans {
		if ca >= 0 && s.a <= cb {
			cb = max(cb, s.b)
			continue
		}
		if ca >= 0 {
			total += cb - ca
		}
		ca, cb = s.a, s.b
	}
	if ca >= 0 {
		total += cb - ca
	}
	return total
}

func generate3(i int, r *rand.Rand) (gold3, string) {
	fn, ln := pick(r, first), pick(r, last)
	g := gold3{ID: fmt.Sprintf("D1H3-%03d", i+1), Segment: "fresher", Level: "intern", Name: fn + " " + ln}
	g.Email = strings.ToLower(fn+"."+ln) + fmt.Sprintf("%d@example.com", r.IntN(90)+10)
	g.Phone = fmt.Sprintf("+91 9%04d %05d", r.IntN(10000), r.IntN(100000))
	g.Template = []string{"academic", "academic_docx"}[i%2]
	g.Format = map[bool]string{true: "docx", false: "pdf"}[g.Template == "academic_docx"]
	ord := ordinals[r.IntN(len(ordinals))]
	prog := pick(r, programmes)
	// A programme attended twice: two ranges on one line, and an ordinal in the title.
	g.Experience = append(g.Experience, entry{
		Employer: pick(r, employers), Title: fmt.Sprintf("Apprentice - %s (%s%s Consecutive Year)", prog, ord.n, ord.sup), Kind: "programme",
		Periods: []period{{"2025-08", "2025-09"}, {"2026-07", "2026-08"}},
	})
	g.Experience = append(g.Experience,
		entry{Employer: pick(r, employers), Title: "Summer SDE Intern", Kind: "internship", Periods: []period{{"2025-06", "2025-08"}}},
		entry{Employer: "Ministry of Electronics and IT, India", Title: "AI Research Associate", Kind: "internship", Periods: []period{{"2025-08", "2025-10"}}},
	)
	g.Months = monthsOf(g.Experience)
	g.Education = []education{{Institution: pick(r, colleges), Degree: "Bachelor of Technology in Computer Engineering", GradYear: 2027,
		Score: fmt.Sprintf("CGPA: %.2f/10", 7.5+float64(r.IntN(240))/100)}}
	school := boards[r.IntN(len(boards))]
	base := sample(r, skillPool, 6)
	abbrev := sample(r, coreAbbrev, 3+r.IntN(3))
	stack := sample(r, stackSkills, 3)
	research := sample(r, researchSkills, 3)
	g.Skills = append(append(append(append([]string{}, base...), abbrev...), stack...), research...)
	g.Projects = sample(r, projects, 2)
	g.Publications = []string{fmt.Sprintf("%s-based Named Entity Recognition for Code-Mixed Hindi-English Text", research[0])}
	g.Achievements = sample(r, hackathons, 2)
	g.Certifications = sample(r, certs, 2)
	return g, school.degree + "|" + school.inst
}

// renderAcademic draws the owner-style layout: right-aligned dates, a raised ordinal, sections
// for research, achievements and certifications, and tech stacks in project lines.
func renderAcademic(g gold3, school string, r *rand.Rand) []byte {
	sch := strings.SplitN(school, "|", 2)
	stack := g.Skills[len(g.Skills)-6 : len(g.Skills)-3]
	research := g.Skills[len(g.Skills)-3:]
	base := g.Skills[:6] // generate3 orders skills: 6 base, the abbreviations, 3 stack, 3 research
	core := g.Skills[6 : len(g.Skills)-6]
	dr := func(p period) string { return hardDate(p.Start, 3) + " – " + hardDate(p.End, 3) }
	type ln struct{ left, right, font string }
	var rows []ln
	rows = append(rows, ln{g.Name, "", "serif-bold"}, ln{g.Email + " | " + g.Phone, "", "serif"},
		ln{"Education", "", "serif-bold"},
		ln{g.Education[0].Institution, "2023 – 2027", "serif-bold"},
		ln{g.Education[0].Degree + " | " + g.Education[0].Score, "", "serif"},
		ln{sch[1], "2023", "serif-bold"}, ln{sch[0] + " | 94.5%", "", "serif"},
		ln{"Experience", "", "serif-bold"})
	for _, e := range g.Experience {
		var spans []string
		for _, p := range e.Periods {
			spans = append(spans, dr(p))
		}
		rows = append(rows, ln{e.Employer, "Remote", "serif-bold"}, ln{e.Title, strings.Join(spans, " & "), "serif"},
			ln{"– Built and evaluated models on internal datasets", "", "serif"})
	}
	rows = append(rows, ln{"Projects", "", "serif-bold"})
	for k, p := range g.Projects {
		rows = append(rows, ln{p + " | " + stack[k%len(stack)] + ", " + stack[(k+1)%len(stack)], "", "serif-bold"},
			ln{"– Real-time pipeline using " + stack[2], "", "serif"})
	}
	rows = append(rows, ln{"Research & Publications", "", "serif-bold"},
		ln{g.Publications[0], venues[r.IntN(len(venues))], "serif"},
		ln{"– Fine-tuned " + research[1] + " with " + research[2] + " decoding", "", "serif"},
		ln{"Hackathons, Achievements & Certifications", "", "serif-bold"})
	for _, a := range g.Achievements {
		rows = append(rows, ln{"– " + a, "", "serif"})
	}
	for _, c := range g.Certifications {
		rows = append(rows, ln{"– Certification: " + c, "", "serif"})
	}
	rows = append(rows, ln{"Technical Skills", "", "serif-bold"},
		ln{"Languages and Tools: " + strings.Join(base, ", "), "", "serif"},
		ln{"Core Subjects: " + strings.Join(core, ", "), "", "serif"})

	if g.Format == "docx" {
		var b []extract.DocxBlock
		for _, row := range rows {
			text := row.left
			if row.right != "" {
				text += "\t" + row.right
			}
			b = append(b, extract.DocxBlock{Runs: []extract.DocxRun{{Text: text}}})
		}
		return extract.MakeDOCXBlocks(b)
	}
	var it []extract.Item
	y := 760.0
	for _, row := range rows {
		size := 10.0
		if row.font == "serif-bold" && row.right == "" && len(row.left) < 45 {
			size = 12
		}
		text := row.left
		// The ordinal's suffix is drawn raised and small, as Word and LaTeX do.
		if k := strings.Index(text, " Consecutive Year)"); k > 1 {
			num, sup := text[:k-2], text[k-2:k]
			width := float64(len(num)) * size * 0.45
			it = append(it, extract.Item{X: 50, Y: y, Text: num, Font: "serif", Size: size},
				extract.Item{X: 50 + width, Y: y + 3.5, Text: sup, Font: "serif", Size: 6},
				extract.Item{X: 50 + width + 6, Y: y, Text: text[k:], Font: "serif", Size: size})
		} else {
			it = append(it, extract.Item{X: 50, Y: y, Text: text, Font: row.font, Size: size})
		}
		if row.right != "" {
			it = append(it, extract.Item{X: 400, Y: y, Text: row.right, Font: "serif", Size: size})
		}
		y -= size + 5
		if y < 60 {
			break
		}
	}
	return extract.MakeRichPDF([][]extract.Item{it})
}

func generateHard3(dir string, seed uint64, prefix string, count int) {
	r := rand.New(rand.NewPCG(seed, 3))
	gf, err := os.Create(filepath.Join(dir, "gold.jsonl"))
	if err != nil {
		panic(err)
	}
	defer gf.Close()
	enc := json.NewEncoder(gf)
	for i := range count {
		g, school := generate3(i, r)
		g.ID = fmt.Sprintf("%s-%03d", prefix, i+1)
		if err := os.WriteFile(filepath.Join(dir, g.ID+"."+g.Format), renderAcademic(g, school, r), 0o644); err != nil {
			panic(err)
		}
		if err := enc.Encode(g); err != nil {
			panic(err)
		}
	}
	fmt.Println("wrote", count, "resumes (real-resume patterns) to", dir)
}
