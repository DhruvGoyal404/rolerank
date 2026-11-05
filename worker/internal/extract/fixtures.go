package extract

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
)

// PDFLine is one line of a generated PDF.
type PDFLine struct {
	Text      string
	White     bool // drawn in white: invisible on the page
	Invisible bool // text render mode 3
	Size      float64
}

// MakePDF writes a minimal one-font PDF, one page per 40 lines. It exists for tests and the D1
// resumes, whose true contents must be known exactly.
func MakePDF(lines []PDFLine) []byte {
	var pages [][]PDFLine
	for i := 0; i < len(lines); i += 40 {
		pages = append(pages, lines[i:min(i+40, len(lines))])
	}
	if len(pages) == 0 {
		pages = [][]PDFLine{nil}
	}
	var objs []string
	add := func(s string) int { objs = append(objs, s); return len(objs) }
	add("") // 1: catalog, filled below
	add("") // 2: pages
	font := add("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	var kids []string
	for _, pg := range pages {
		var c strings.Builder
		y := 760.0
		for _, l := range pg {
			size := l.Size
			if size == 0 {
				size = 11
			}
			color, mode := "0 0 0 rg", "0 Tr"
			if l.White {
				color = "1 1 1 rg"
			}
			if l.Invisible {
				mode = "3 Tr"
			}
			fmt.Fprintf(&c, "BT %s %s /F1 %.1f Tf 50 %.1f Td (%s) Tj ET\n", color, mode, size, y, pdfEscape(l.Text))
			y -= 16
		}
		content := add(fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", c.Len(), c.String()))
		kids = append(kids, fmt.Sprintf("%d 0 R", add(fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>",
			font, content))))
	}
	objs[0] = "<< /Type /Catalog /Pages 2 0 R >>"
	objs[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(kids))
	return pdfFile(objs, "")
}

// pdfFile lays out objects with a correct xref table; extraTrailer goes into the trailer.
func pdfFile(objs []string, extraTrailer string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R %s>>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, extraTrailer, xref)
	return b.Bytes()
}

func pdfEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, "(", `\(`, ")", `\)`).Replace(s)
}

// DocxRun is one run of a generated DOCX paragraph.
type DocxRun struct {
	Text   string
	Hidden bool // w:vanish
	White  bool
}

// MakeDOCX writes a minimal Word document, one paragraph per entry.
func MakeDOCX(paragraphs [][]DocxRun) []byte {
	var doc strings.Builder
	doc.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	for _, p := range paragraphs {
		doc.WriteString("<w:p>")
		for _, r := range p {
			var props string
			if r.Hidden {
				props += "<w:vanish/>"
			}
			if r.White {
				props += `<w:color w:val="FFFFFF"/>`
			}
			if props != "" {
				props = "<w:rPr>" + props + "</w:rPr>"
			}
			fmt.Fprintf(&doc, `<w:r>%s<w:t xml:space="preserve">%s</w:t></w:r>`, props, xmlEscape(r.Text))
		}
		doc.WriteString("</w:p>")
	}
	doc.WriteString("</w:body></w:document>")
	return Zip(map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml": doc.String(),
	})
}

// Zip builds a zip from name to contents, deflated, names in sorted order.
func Zip(files map[string]string) []byte {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	var names []string
	for n := range files {
		names = append(names, n)
	}
	sortStrings(names)
	for _, n := range names {
		f, _ := w.Create(n)
		_, _ = f.Write([]byte(files[n]))
	}
	_ = w.Close()
	return b.Bytes()
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// Hostile files, for the suite that proves each is rejected or contained.

// ZipBomb is a DOCX-shaped zip whose one entry inflates far past the cap.
func ZipBomb() []byte {
	return Zip(map[string]string{"[Content_Types].xml": "<x/>", "word/document.xml": strings.Repeat("0", 30<<20)})
}

// XMLBomb is a DOCX whose document declares nested entities ("billion laughs").
func XMLBomb() []byte {
	var dtd strings.Builder
	dtd.WriteString(`<?xml version="1.0"?><!DOCTYPE w [<!ENTITY a0 "lol">`)
	for i := 1; i < 10; i++ {
		fmt.Fprintf(&dtd, `<!ENTITY a%d "%s">`, i, strings.Repeat(fmt.Sprintf("&a%d;", i-1), 10))
	}
	dtd.WriteString(`]><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>&a9;</w:t></w:r></w:p></w:body></w:document>`)
	return Zip(map[string]string{"[Content_Types].xml": "<Types/>", "word/document.xml": dtd.String()})
}

// DeepPDF nests arrays a hundred thousand deep inside a page.
func DeepPDF() []byte {
	nest := strings.Repeat("[", 100000) + strings.Repeat("]", 100000)
	return pdfFile([]string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Deep " + nest + " >>",
	}, "")
}

// ManyPages declares 100,000 pages, each the same small page object.
func ManyPages() []byte {
	kids := strings.TrimSpace(strings.Repeat("3 0 R ", 100000))
	return pdfFile([]string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count 100000 >>", kids),
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
	}, "")
}

// Encrypted declares the standard security handler, so a reader asks for a password.
func Encrypted() []byte {
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
		"<< /Filter /Standard /V 1 /R 2 /Length 40 /P -4 /O <" + strings.Repeat("ab", 32) + "> /U <" + strings.Repeat("cd", 32) + "> >>",
	}
	return pdfFile(objs, "/Encrypt 4 0 R /ID [<00112233445566778899aabbccddeeff> <00112233445566778899aabbccddeeff>] ")
}

// Polyglot is a PDF that is also a Word zip.
func Polyglot() []byte {
	return append(MakePDF([]PDFLine{{Text: "Resume"}}), MakeDOCX([][]DocxRun{{{Text: "Resume"}}})...)
}
