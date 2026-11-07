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

// D1-hard-4: held-out cases for the owner's second real-resume check (2026-11-07), written and
// committed before the fixes and run once after them. "dev4" draws the same layouts from another
// seed, for diagnosis and tuning; hard4 is never used for either.
//   - the employer on its own line above the title-and-dates line (D1-hard-3's experience gap);
//   - months counted inclusively: "Jun 2025 – Aug 2025" is 3 months, overlaps once;
//   - a venue line ("SemEval-2026 | ACL 2026 Workshop, Team X | LLMs, NLP  Jun 2026") followed by
//     bullets whose leading phrase names the paper ("Task 12 - Abductive Event Reasoning: ..."),
//     and a venue with no such bullet;
//   - comma-separated skills on research and project lines, and compounds joined by "&" or "/"
//     ("Generative AI & LLMs", "PyTorch/TensorFlow");
//   - a patent line with a 12-digit application number and an enrolment number, which are not
//     phone numbers, beside phones in several Indian formats;
//   - "Research Associate" held for a month or two while studying (an internship) and held
//     full time after graduating (a job).

type pub4 struct {
	Venue string `json:"venue"`
	Title string `json:"title"` // "" when the bullets name no paper or task
}

type gold4 struct {
	ID             string      `json:"id"`
	Segment        string      `json:"segment"` // student or graduate
	Template       string      `json:"template"`
	Format         string      `json:"format"`
	Level          string      `json:"level"`
	Name           string      `json:"name"`
	Email          string      `json:"email"`
	Phone          string      `json:"phone"`
	Skills         []string    `json:"skills"`
	Experience     []entry     `json:"experience"`
	Education      []education `json:"education"`
	Projects       []string    `json:"projects"`
	Publications   []pub4      `json:"publications"`
	Achievements   []string    `json:"achievements"`
	Certifications []string    `json:"certifications"`
	Months         int         `json:"months"`  // jobs and internships, inclusive, overlaps once
	Numbers        []string    `json:"numbers"` // digit runs that must survive contact stripping
}

var (
	venues4 = []string{"SemEval-2026", "WMT 2026", "BioNLP 2026", "SMM4H–HeaRD 2026", "CLEF eHealth 2026", "FinNLP 2026"}
	tasks4  = []string{"Task 12 - Abductive Event Reasoning", "Task 5 - Graded Word Sense Plausibility", "Task 3 - Multilingual Claim Detection",
		"Task 8 - Clinical Entity Linking", "Task 2 - Low-Resource Translation for Indic Languages", "Task 7 - Numerical Reasoning over Tables"}
	resSkills4 = []string{"LLMs", "Chain-of-Thought", "NLP", "XLM-RoBERTa", "CRF", "Transformers", "BERT", "Hugging Face"}
	labs4      = []string{"DRDO Centre for AI and Robotics", "Ministry of Electronics and IT, India", "IISc Bangalore AI Lab", "C-DAC Pune", "ISRO Space Applications Centre"}
	cities4    = []string{"Bengaluru, Karnataka", "New Delhi, India", "Pune, Maharashtra", "Mohali, Punjab", "Hyderabad, Telangana"}
	phoneFmt4  = []string{"+91 9%04d %05d", "+91-9%09d", "09%09d", "9%04d-%05d"}
)

// monthsInclusive counts the months of job and internship periods, both ends included, each
// month once. "present" is November 2026, when the set was made.
func monthsInclusive(es []entry) int {
	seen := map[int]bool{}
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
			for m := toM(p.Start); m <= toM(p.End); m++ {
				seen[m] = true
			}
		}
	}
	return len(seen)
}

func phone4(r *rand.Rand, i int) string {
	f := phoneFmt4[i%len(phoneFmt4)]
	if strings.Count(f, "%") == 1 {
		return fmt.Sprintf(f, r.IntN(1_000_000_000))
	}
	return fmt.Sprintf(f, r.IntN(10000), r.IntN(100000))
}

func generate4(i int, r *rand.Rand, prefix string) gold4 {
	fn, ln := pick(r, first), pick(r, last)
	g := gold4{ID: fmt.Sprintf("%s-%03d", prefix, i+1), Name: fn + " " + ln}
	g.Email = strings.ToLower(fn+"."+ln) + fmt.Sprintf("%d@example.com", r.IntN(90)+10)
	g.Phone = phone4(r, i)
	g.Template = []string{"academic", "academic_docx"}[i%2]
	g.Format = map[bool]string{true: "docx", false: "pdf"}[g.Template == "academic_docx"]
	lab := pick(r, labs4)
	if i%4 < 3 {
		// A student: a short research associateship and an internship, overlapping by a month.
		g.Segment, g.Level = "student", "intern"
		g.Experience = []entry{
			{Employer: lab, Title: pick(r, []string{"AI Research Associate", "AI & DevOps Research Associate", "Research Associate"}), Kind: "internship",
				Periods: []period{{"2025-08", fmt.Sprintf("2025-%02d", 8+r.IntN(2))}}},
			{Employer: pick(r, employers), Title: "Summer SDE Intern", Kind: "internship", Periods: []period{{"2025-06", "2025-08"}}},
		}
		g.Education = []education{{Institution: pick(r, colleges), Degree: "Bachelor of Technology in Computer Engineering", GradYear: 2027,
			Score: fmt.Sprintf("CGPA: %.2f/10", 7.5+float64(r.IntN(240))/100)}}
	} else {
		// A graduate: an internship, then research associate full time since graduating.
		g.Segment, g.Level = "graduate", "entry"
		g.Experience = []entry{
			{Employer: lab, Title: "Research Associate", Kind: "job", Periods: []period{{"2024-07", "present"}}},
			{Employer: pick(r, employers), Title: "Software Engineering Intern", Kind: "internship", Periods: []period{{"2024-01", "2024-06"}}},
		}
		g.Education = []education{{Institution: pick(r, colleges), Degree: "Bachelor of Technology in Computer Science", GradYear: 2024,
			Score: fmt.Sprintf("CGPA: %.2f/10", 7.5+float64(r.IntN(240))/100)}}
	}
	g.Months = monthsInclusive(g.Experience)

	res := sample(r, resSkills4, 3)
	base := sample(r, skillPool, 5)
	stack := sample(r, stackSkills, 3)
	g.Skills = append(append(append(append([]string{}, base...), "Generative AI", "LLMs", "PyTorch", "TensorFlow"), stack...), res...)
	g.Projects = sample(r, projects, 2)
	vs := sample(r, venues4, 2)
	ts := sample(r, tasks4, 2)
	g.Publications = []pub4{{vs[0], ts[0]}, {vs[0], ts[1]}, {vs[1], ""}}
	appNo := fmt.Sprintf("2026110%05d", r.IntN(100000))
	enrol := fmt.Sprintf("10%08d", r.IntN(100_000_000))
	g.Numbers = []string{appNo, enrol}
	g.Achievements = append([]string{"Indian Patent Application (Published) | Application No. " + appNo + " | Publication No. 32/2026"}, sample(r, hackathons, 1)...)
	g.Certifications = sample(r, certs, 2)
	return g
}

func renderAcademic4(g gold4, r *rand.Rand) []byte {
	n := len(g.Skills)
	base, stack, res := g.Skills[:5], g.Skills[n-6:n-3], g.Skills[n-3:]
	dr := func(p period, style int) string { return hardDate(p.Start, style) + " – " + hardDate(p.End, style) }
	type ln struct{ left, right string }
	style := r.IntN(4)
	edu := g.Education[0]
	rows := []ln{{g.Name, ""}, {g.Phone + " | " + g.Email + " | github.com/" + strings.ToLower(strings.ReplaceAll(g.Name, " ", "")), ""},
		{"Education", ""},
		{edu.Institution, fmt.Sprintf("%d – %d", edu.GradYear-4, edu.GradYear)},
		{edu.Degree + " | " + edu.Score, ""},
		{"University Enrolment No. " + g.Numbers[1], ""},
		{"Delhi Public School", fmt.Sprint(edu.GradYear - 4)}, {"CBSE Class XII | 93.2%", ""},
		{"Experience", ""}}
	for k, e := range g.Experience {
		// The employer sits on its own line, with its location in one of three places.
		switch (k + r.IntN(3)) % 3 {
		case 0:
			rows = append(rows, ln{e.Employer, pick(r, cities4)})
		case 1:
			rows = append(rows, ln{e.Employer + " | " + pick(r, cities4), ""})
		default:
			rows = append(rows, ln{e.Employer, ""})
		}
		rows = append(rows, ln{e.Title, dr(e.Periods[0], style)}, ln{"– Built and evaluated models on internal datasets", ""})
	}
	rows = append(rows, ln{"Research & Publications", ""},
		ln{g.Publications[0].Venue + " | ACL 2026 Workshop, Team Paradise | " + strings.Join(res, ", "), "Jun 2026"},
		ln{"– " + g.Publications[0].Title + ": Built an instruction-tuned system with chain-of-thought prompting", ""},
		ln{"and grounding; achieved an official score of 0.79, ranking 59th out of 221 systems.", ""},
		ln{"– " + g.Publications[1].Title + ": Explored interpretable features; reported a negative finding.", ""},
		ln{g.Publications[2].Venue + " | ACL 2026 Workshop | " + res[0] + ", " + res[1] + " | Published Link", "Jun 2026"},
		ln{"– Fine-tuned models across 3 tasks: ADE detection (macro-F1 0.597), span extraction (0.60).", ""},
		ln{"Projects", ""})
	for k, p := range g.Projects {
		rows = append(rows, ln{p + " | " + stack[k%3] + ", " + stack[(k+1)%3] + ", " + stack[(k+2)%3], "Dec 2025"},
			ln{"– Real-time pipeline with sub-200ms latency", ""})
	}
	rows = append(rows, ln{"Achievements & Certifications", ""}, ln{g.Achievements[0], "Aug 2026"})
	for _, a := range g.Achievements[1:] {
		rows = append(rows, ln{"– " + a, ""})
	}
	for _, c := range g.Certifications {
		rows = append(rows, ln{"– Certification: " + c, ""})
	}
	rows = append(rows, ln{"Technical Skills", ""},
		ln{"Languages and Tools: " + strings.Join(base, ", "), ""},
		ln{"AI/ML: Generative AI & LLMs, PyTorch/TensorFlow", ""})

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
	y := 770.0
	for _, row := range rows {
		size, font := 10.0, "serif"
		if row.right == "" && len(row.left) < 32 && !strings.HasPrefix(row.left, "–") {
			size, font = 12, "serif-bold"
		}
		it = append(it, extract.Item{X: 50, Y: y, Text: row.left, Font: font, Size: size})
		if row.right != "" {
			// Right-aligned dates, pushed right of a long left text so the two never touch.
			it = append(it, extract.Item{X: max(430, 60+float64(len(row.left))*size*0.5), Y: y, Text: row.right, Font: "serif", Size: size})
		}
		y -= size + 4
		if y < 40 {
			break
		}
	}
	return extract.MakeRichPDF([][]extract.Item{it})
}

func generateHard4(dir string, seed uint64, prefix string, count int) {
	r := rand.New(rand.NewPCG(seed, 4))
	gf, err := os.Create(filepath.Join(dir, "gold.jsonl"))
	if err != nil {
		panic(err)
	}
	defer gf.Close()
	enc := json.NewEncoder(gf)
	for i := range count {
		g := generate4(i, r, prefix)
		if err := os.WriteFile(filepath.Join(dir, g.ID+"."+g.Format), renderAcademic4(g, r), 0o644); err != nil {
			panic(err)
		}
		if err := enc.Encode(g); err != nil {
			panic(err)
		}
	}
	fmt.Println("wrote", count, "resumes to", dir)
}
