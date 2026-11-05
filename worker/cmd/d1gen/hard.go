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

// D1-hard: a held-out set, generated after D1 was scored and never tuned on. It uses its own
// seed and the layouts real Indian fresher resumes use: a real two-column sidebar, tables, icon
// contact rows, mixed fonts, Canva- and Overleaf-style templates, and awkward dates.
var hardTemplates = []string{"sidebar", "table", "overleaf", "canva", "docx_table"}

// hardDate writes a YYYY-MM the many ways resumes do. Gold keeps YYYY-MM.
func hardDate(d string, style int) string {
	if d == "present" {
		return []string{"Present", "Now", "Till date", "Current"}[style%4]
	}
	var y, m int
	fmt.Sscanf(d, "%d-%d", &y, &m)
	full := []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	switch style % 4 {
	case 0:
		return fmt.Sprintf("%s'%02d", months[m-1], y%100)
	case 1:
		return fmt.Sprintf("%s %d", full[m-1], y)
	case 2:
		return fmt.Sprintf("%02d/%d", m, y)
	}
	return fmt.Sprintf("%s %d", months[m-1], y)
}

func span(e experience, style int) string {
	return hardDate(e.Start, style) + " – " + hardDate(e.End, style)
}

// renderHard lays out one resume in a hard template, as PDF items or DOCX blocks.
func renderHard(g gold, style int) []byte {
	contact := []string{g.Email, g.Phone, "linkedin.com/in/" + strings.ToLower(strings.ReplaceAll(g.Name, " ", ""))}
	edu := g.Education[0]
	switch g.Template {
	case "sidebar":
		// Two real columns: the sidebar and the main column share rows, as in a Canva or Word
		// two-column template, so a reader has to keep them apart.
		var it []extract.Item
		y := 740.0
		it = append(it, extract.Item{X: 220, Y: y, Text: g.Name, Font: "sans-bold", Size: 22})
		y -= 34
		left := []extract.Item{{Text: "CONTACT", Font: "sans-bold"}}
		for i, c := range contact {
			left = append(left, extract.Item{Text: string(")%*"[i]) + " " + c, Font: "sans", Size: 8})
		}
		left = append(left, extract.Item{Text: "SKILLS", Font: "sans-bold"})
		for _, s := range g.Skills {
			left = append(left, extract.Item{Text: "• " + s, Font: "sans", Size: 9})
		}
		left = append(left, extract.Item{Text: "EDUCATION", Font: "sans-bold"},
			extract.Item{Text: edu.Degree, Size: 8}, extract.Item{Text: edu.Institution, Size: 8},
			extract.Item{Text: fmt.Sprintf("%d · %s", edu.GradYear, edu.Score), Size: 8})
		var right []extract.Item
		if len(g.Experience) > 0 {
			right = append(right, extract.Item{Text: "EXPERIENCE", Font: "sans-bold", Size: 12})
			for _, e := range g.Experience {
				right = append(right, extract.Item{Text: e.Title, Font: "sans-bold"},
					extract.Item{Text: e.Employer + " | " + span(e, style), Size: 9},
					extract.Item{Text: "• Shipped features used by internal teams", Size: 9})
			}
		}
		if len(g.Projects) > 0 {
			right = append(right, extract.Item{Text: "PROJECTS", Font: "sans-bold", Size: 12})
			for _, p := range g.Projects {
				right = append(right, extract.Item{Text: p, Font: "sans-bold"}, extract.Item{Text: "• Built with " + g.Skills[0], Size: 9})
			}
		}
		for i := 0; i < max(len(left), len(right)); i++ {
			if i < len(left) {
				l := left[i]
				l.X, l.Y = 36, y
				it = append(it, l)
			}
			if i < len(right) {
				r := right[i]
				r.X, r.Y = 220, y
				it = append(it, r)
			}
			y -= 15
		}
		return extract.MakeRichPDF([][]extract.Item{it})

	case "table":
		var it []extract.Item
		y := 750.0
		row := func(cells []string, xs []float64, font string) {
			for i, c := range cells {
				it = append(it, extract.Item{X: xs[i], Y: y, Text: c, Font: font, Size: 9})
			}
			y -= 16
		}
		it = append(it, extract.Item{X: 200, Y: y, Text: g.Name, Font: "serif-bold", Size: 20})
		y -= 24
		// Icon contact row: a ZapfDingbats glyph before each item, all on one line.
		x := 60.0
		for i, c := range contact {
			it = append(it, extract.Item{X: x, Y: y, Text: string(")%*"[i]), Font: "icons", Size: 9}, extract.Item{X: x + 12, Y: y, Text: c, Size: 8})
			x += 175
		}
		y -= 26
		cols := []float64{40, 200, 420, 500}
		it = append(it, extract.Item{X: 40, Y: y, Text: "ACADEMIC QUALIFICATIONS", Font: "sans-bold", Size: 11})
		y -= 18
		row([]string{"Degree", "Institute", "Year", "CGPA/%"}, cols, "sans-bold")
		row([]string{edu.Degree, edu.Institution, fmt.Sprint(edu.GradYear), edu.Score}, cols, "sans")
		row([]string{"Class XII", "CBSE", fmt.Sprint(edu.GradYear - 4), "89%"}, cols, "sans")
		y -= 10
		if len(g.Experience) > 0 {
			it = append(it, extract.Item{X: 40, Y: y, Text: "WORK EXPERIENCE", Font: "sans-bold", Size: 11})
			y -= 18
			row([]string{"Organisation", "Role", "Duration"}, []float64{40, 200, 420}, "sans-bold")
			for _, e := range g.Experience {
				row([]string{e.Employer, e.Title, span(e, style)}, []float64{40, 200, 420}, "sans")
			}
			y -= 10
		}
		it = append(it, extract.Item{X: 40, Y: y, Text: "TECHNICAL SKILLS", Font: "sans-bold", Size: 11})
		y -= 18
		row([]string{"Languages and tools:", strings.Join(g.Skills, ", ")}, []float64{40, 160}, "sans")
		if len(g.Projects) > 0 {
			it = append(it, extract.Item{X: 40, Y: y, Text: "PROJECTS", Font: "sans-bold", Size: 11})
			y -= 18
			for _, p := range g.Projects {
				row([]string{p, "Tech: " + g.Skills[len(g.Skills)-1]}, []float64{40, 300}, "sans")
			}
		}
		return extract.MakeRichPDF([][]extract.Item{it})

	case "overleaf":
		// LaTeX resume style: serif, small-caps-like headings, dates right-aligned on the title line.
		var it []extract.Item
		y := 750.0
		line := func(left, right, font string, size float64) {
			it = append(it, extract.Item{X: 50, Y: y, Text: left, Font: font, Size: size})
			if right != "" {
				it = append(it, extract.Item{X: 470, Y: y, Text: right, Font: "serif", Size: size})
			}
			y -= size + 5
		}
		line(g.Name, "", "serif-bold", 24)
		line(strings.Join(contact, "  |  "), "", "serif", 8)
		head := func(t string) { y -= 6; line(strings.ToUpper(t), "", "serif-bold", 11) }
		head("Education")
		line(edu.Institution, fmt.Sprint(edu.GradYear), "serif-bold", 10)
		line(edu.Degree+"; "+edu.Score, "", "serif", 10)
		if len(g.Experience) > 0 {
			head("Experience")
			for _, e := range g.Experience {
				line(e.Title+" — "+e.Employer, span(e, style), "serif-bold", 10)
				line("• Designed and maintained services; reviewed code", "", "serif", 9)
			}
		}
		if len(g.Projects) > 0 {
			head("Projects")
			for _, p := range g.Projects {
				line(p+" | "+g.Skills[0]+", "+g.Skills[1], "", "serif", 10)
			}
		}
		head("Technical Skills")
		line("Languages and Frameworks: "+strings.Join(g.Skills, ", "), "", "serif", 10)
		return extract.MakeRichPDF([][]extract.Item{it})

	case "canva":
		// Canva style: a big name, letter-spaced headings, skills in a three-column grid, dates
		// above the role in a monospace font, sections in an unusual order.
		var it []extract.Item
		y := 745.0
		spaced := func(s string) string { return strings.Join(strings.Split(strings.ToUpper(s), ""), " ") }
		it = append(it, extract.Item{X: 50, Y: y, Text: strings.ToUpper(g.Name), Font: "sans-bold", Size: 28})
		y -= 22
		it = append(it, extract.Item{X: 50, Y: y, Text: strings.Join(contact, "   "), Size: 8})
		y -= 30
		it = append(it, extract.Item{X: 50, Y: y, Text: spaced("Skills"), Font: "sans-bold", Size: 11})
		y -= 16
		for i, s := range g.Skills {
			it = append(it, extract.Item{X: 50 + float64(i%3)*180, Y: y, Text: s, Size: 9})
			if i%3 == 2 {
				y -= 13
			}
		}
		y -= 22
		if len(g.Projects) > 0 {
			it = append(it, extract.Item{X: 50, Y: y, Text: spaced("Projects"), Font: "sans-bold", Size: 11})
			y -= 16
			for _, p := range g.Projects {
				it = append(it, extract.Item{X: 50, Y: y, Text: p, Font: "sans-bold", Size: 10})
				y -= 14
			}
			y -= 10
		}
		if len(g.Experience) > 0 {
			it = append(it, extract.Item{X: 50, Y: y, Text: spaced("Experience"), Font: "sans-bold", Size: 11})
			y -= 16
			for _, e := range g.Experience {
				it = append(it, extract.Item{X: 50, Y: y, Text: span(e, style), Font: "mono", Size: 8})
				y -= 12
				it = append(it, extract.Item{X: 50, Y: y, Text: e.Title + " @ " + e.Employer, Font: "sans-bold", Size: 10})
				y -= 16
			}
		}
		it = append(it, extract.Item{X: 50, Y: y, Text: spaced("Education"), Font: "sans-bold", Size: 11})
		y -= 16
		it = append(it, extract.Item{X: 50, Y: y, Text: edu.Degree, Size: 10},
			extract.Item{X: 50, Y: y - 13, Text: edu.Institution + " · " + fmt.Sprint(edu.GradYear) + " · " + edu.Score, Size: 9})
		return extract.MakeRichPDF([][]extract.Item{it})
	}

	// docx_table: a Word resume whose education and experience sit in real tables.
	var b []extract.DocxBlock
	p := func(s string) { b = append(b, extract.DocxBlock{Runs: []extract.DocxRun{{Text: s}}}) }
	p(g.Name)
	p(strings.Join(contact, " | "))
	p("Education")
	b = append(b, extract.DocxBlock{Rows: [][]string{{"Qualification", "Institute", "Year", "Score"},
		{edu.Degree, edu.Institution, fmt.Sprint(edu.GradYear), edu.Score}}})
	if len(g.Experience) > 0 {
		p("Professional Experience")
		rows := [][]string{{"Company", "Designation", "Period"}}
		for _, e := range g.Experience {
			rows = append(rows, []string{e.Employer, e.Title, span(e, style)})
		}
		b = append(b, extract.DocxBlock{Rows: rows})
	}
	p("Skills: " + strings.Join(g.Skills, ", "))
	if len(g.Projects) > 0 {
		p("Projects")
		for _, pr := range g.Projects {
			p("Project: " + pr)
		}
	}
	return extract.MakeDOCXBlocks(b)
}

// generateHard writes the held-out set: 40 resumes, 60% freshers and interns.
func generateHard(dir string) {
	r := rand.New(rand.NewPCG(20261106, 7))
	gf, err := os.Create(filepath.Join(dir, "gold.jsonl"))
	if err != nil {
		panic(err)
	}
	defer gf.Close()
	enc := json.NewEncoder(gf)
	for i := range 40 {
		// Reuse the base generator for the facts; the segment mix leans to freshers.
		base := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 13, 14, 15, 16, 17, 18, 19, 12}[i%20]
		seg := segment(base)
		g := generate(base, r)
		if i%5 < 3 {
			seg = "fresher"
			g = generate(0, r) // the base generator's fresher branch
		}
		g.ID = fmt.Sprintf("D1H-%03d", i+1)
		g.Segment = seg
		g.Template = hardTemplates[i%len(hardTemplates)]
		g.Format = "pdf"
		if g.Template == "docx_table" {
			g.Format = "docx"
		}
		file := renderHard(g, r.IntN(4))
		if err := os.WriteFile(filepath.Join(dir, g.ID+"."+g.Format), file, 0o644); err != nil {
			panic(err)
		}
		if err := enc.Encode(g); err != nil {
			panic(err)
		}
	}
	fmt.Println("wrote 40 held-out hard resumes to", dir)
}
