package extract

import (
	"fmt"
	"strings"
)

// Item is one piece of text placed on a PDF page, for layouts that a line-per-row writer can't
// make: columns, sidebars, tables, right-aligned dates, icon rows and mixed fonts.
type Item struct {
	X, Y float64
	Text string
	Font string // "sans", "sans-bold", "serif", "serif-bold", "mono", "icons"
	Size float64
}

var fonts = []struct{ key, base string }{
	{"sans", "Helvetica"}, {"sans-bold", "Helvetica-Bold"}, {"serif", "Times-Roman"},
	{"serif-bold", "Times-Bold"}, {"mono", "Courier"}, {"icons", "ZapfDingbats"},
}

// winAnsi maps the few non-ASCII characters real resumes use to their WinAnsiEncoding bytes.
var winAnsi = map[rune]byte{'–': 0x96, '—': 0x97, '’': 0x92, '‘': 0x91, '•': 0x95, '“': 0x93, '”': 0x94, '·': 0xB7}

func encode(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '(' || r == ')' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 128:
			b.WriteRune(r)
		default:
			if c, ok := winAnsi[r]; ok {
				fmt.Fprintf(&b, "\\%03o", c)
			} else {
				b.WriteByte('?')
			}
		}
	}
	return b.String()
}

// MakeRichPDF writes one page per slice of items, with the base-14 fonts above.
func MakeRichPDF(pages [][]Item) []byte {
	var objs []string
	add := func(s string) int { objs = append(objs, s); return len(objs) }
	add("") // catalog
	add("") // pages
	var res strings.Builder
	res.WriteString("<< /Font << ")
	for i, f := range fonts {
		enc := " /Encoding /WinAnsiEncoding"
		if f.key == "icons" {
			enc = ""
		}
		id := add(fmt.Sprintf("<< /Type /Font /Subtype /Type1 /BaseFont /%s%s >>", f.base, enc))
		fmt.Fprintf(&res, "/F%d %d 0 R ", i+1, id)
	}
	res.WriteString(">> >>")
	fontID := map[string]int{}
	for i, f := range fonts {
		fontID[f.key] = i + 1
	}
	var kids []string
	for _, items := range pages {
		var c strings.Builder
		for _, it := range items {
			size := it.Size
			if size == 0 {
				size = 10
			}
			f := fontID[it.Font]
			if f == 0 {
				f = 1
			}
			fmt.Fprintf(&c, "BT /F%d %.1f Tf %.1f %.1f Td (%s) Tj ET\n", f, size, it.X, it.Y, encode(it.Text))
		}
		content := add(fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", c.Len(), c.String()))
		kids = append(kids, fmt.Sprintf("%d 0 R", add(fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources %s /Contents %d 0 R >>", res.String(), content))))
	}
	objs[0] = "<< /Type /Catalog /Pages 2 0 R >>"
	objs[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(kids))
	return pdfFile(objs, "")
}

// DocxBlock is a paragraph (Runs) or a table (Rows of cells, each cell one paragraph).
type DocxBlock struct {
	Runs []DocxRun
	Rows [][]string
}

// MakeDOCXBlocks writes a Word document with paragraphs and real tables.
func MakeDOCXBlocks(blocks []DocxBlock) []byte {
	var body strings.Builder
	for _, b := range blocks {
		if b.Rows == nil {
			body.WriteString(paragraph(b.Runs))
			continue
		}
		body.WriteString("<w:tbl><w:tblPr><w:tblW w:w=\"0\" w:type=\"auto\"/></w:tblPr>")
		for _, row := range b.Rows {
			body.WriteString("<w:tr>")
			for _, cell := range row {
				body.WriteString("<w:tc>" + paragraph([]DocxRun{{Text: cell}}) + "</w:tc>")
			}
			body.WriteString("</w:tr>")
		}
		body.WriteString("</w:tbl>")
	}
	doc := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` + body.String() + `</w:body></w:document>`
	files := docxParts()
	files["word/document.xml"] = doc
	return Zip(files)
}

func paragraph(runs []DocxRun) string {
	var b strings.Builder
	b.WriteString("<w:p>")
	for _, r := range runs {
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
		fmt.Fprintf(&b, `<w:r>%s<w:t xml:space="preserve">%s</w:t></w:r>`, props, xmlEscape(r.Text))
	}
	b.WriteString("</w:p>")
	return b.String()
}

func docxParts() map[string]string {
	return map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
	}
}
