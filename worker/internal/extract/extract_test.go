package extract

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestCheckRefusesWhatShouldNeverReachTheSandbox(t *testing.T) {
	many := map[string]string{"[Content_Types].xml": "<x/>", "word/document.xml": "<x/>"}
	for i := range MaxEntries + 1 {
		many[fmt.Sprintf("part%04d.bin", i)] = ""
	}
	tests := map[string][]byte{
		"empty":          nil,
		"too large":      append([]byte("%PDF-"), bytes.Repeat([]byte{0}, MaxBytes)...),
		"not a resume":   []byte("MZ\x90\x00 an exe"),
		"zip bomb":       ZipBomb(),
		"xml bomb":       XMLBomb(),
		"polyglot":       Polyglot(),
		"plain zip":      Zip(map[string]string{"a.txt": "hi"}),
		"too many files": Zip(many),
		"truncated zip":  MakeDOCX([][]DocxRun{{{Text: "x"}}})[:40],
	}
	for name, b := range tests {
		if _, err := Check(b); !errors.Is(err, ErrRejected) {
			t.Errorf("%s: got %v, want a rejection", name, err)
		}
	}
}

func TestCheckAcceptsRealResumes(t *testing.T) {
	if k, err := Check(MakePDF([]PDFLine{{Text: "Jane Doe"}})); err != nil || k != PDF {
		t.Fatalf("pdf: %q %v", k, err)
	}
	if k, err := Check(MakeDOCX([][]DocxRun{{{Text: "Jane Doe"}}})); err != nil || k != DOCX {
		t.Fatalf("docx: %q %v", k, err)
	}
}

func TestArgsLockTheContainerDown(t *testing.T) {
	a := Runner{Image: "img", Runtime: "runsc"}.Args("n")
	for _, want := range []string{"--network none", "--read-only", "--cap-drop ALL", "--security-opt no-new-privileges",
		"--user 65534:65534", "--memory 512m", "--memory-swap 512m", "--pids-limit 64", "--runtime runsc",
		"--label rolerank.managed=true"} {
		if !strings.Contains(strings.Join(a, " "), want) {
			t.Errorf("missing %q", want)
		}
	}
	if a[len(a)-1] != "img" || slices.Contains(a, "--privileged") {
		t.Fatal(a)
	}
}
