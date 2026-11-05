// Package extract turns an uploaded resume into numbered text lines. It never parses the file
// itself: Check looks at the bytes, then Run hands them to a single-use sandbox container
// (extractor/), with no network, a read-only root, no capabilities, as nobody, under gVisor in
// production, and reads JSON back (ARCHITECTURE.md, "Resume pipeline").
package extract

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// Limits. A file outside them never reaches the sandbox.
const (
	MaxBytes    = 5 << 20  // upload size
	MaxUnzipped = 20 << 20 // a DOCX's declared uncompressed total
	MaxEntries  = 1000     // files inside a DOCX
	MaxRatio    = 100      // compression ratio of any one entry
	MaxOutput   = 200 << 10
)

// Kinds of file accepted.
const (
	PDF  = "pdf"
	DOCX = "docx"
)

// ErrRejected wraps every reason Check refuses a file.
var ErrRejected = errors.New("file rejected")

func reject(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrRejected, fmt.Sprintf(format, a...))
}

// Check decides what a file is from its bytes, never its name or declared type, and refuses
// anything outside the limits. A DOCX's zip directory is read without inflating the parts,
// except the XML ones, which are inflated under the cap and refused if they declare entities.
func Check(b []byte) (string, error) {
	switch {
	case len(b) == 0:
		return "", reject("empty")
	case len(b) > MaxBytes:
		return "", reject("larger than %d bytes", MaxBytes)
	case bytes.HasPrefix(b, []byte("%PDF-")):
		if bytes.Contains(b, []byte("PK\x03\x04")) && bytes.Contains(b, []byte("[Content_Types].xml")) {
			return "", reject("a PDF that is also a zip")
		}
		return PDF, nil
	case bytes.HasPrefix(b, []byte("PK\x03\x04")):
		return DOCX, checkDocx(b)
	}
	return "", reject("not a PDF or DOCX")
}

func checkDocx(b []byte) error {
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return reject("unreadable zip: %v", err)
	}
	if len(z.File) > MaxEntries {
		return reject("%d entries", len(z.File))
	}
	var total uint64
	names := map[string]bool{}
	for _, f := range z.File {
		names[f.Name] = true
		total += f.UncompressedSize64
		if total > MaxUnzipped {
			return reject("declared size over %d bytes", MaxUnzipped)
		}
		if f.CompressedSize64 > 0 && f.UncompressedSize64/f.CompressedSize64 > MaxRatio {
			return reject("%s compresses %dx", f.Name, f.UncompressedSize64/f.CompressedSize64)
		}
	}
	if !names["[Content_Types].xml"] || !names["word/document.xml"] {
		return reject("not a Word document")
	}
	var inflated int64
	for _, f := range z.File {
		if !strings.HasSuffix(f.Name, ".xml") && !strings.HasSuffix(f.Name, ".rels") {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return reject("%s: %v", f.Name, err)
		}
		// Read at most what's left of the cap, plus one byte, so a lying header can't inflate more.
		data, err := io.ReadAll(io.LimitReader(r, MaxUnzipped-inflated+1))
		r.Close()
		if err != nil {
			return reject("%s: %v", f.Name, err)
		}
		if inflated += int64(len(data)); inflated > MaxUnzipped {
			return reject("inflates past %d bytes", MaxUnzipped)
		}
		if bytes.Contains(data, []byte("<!ENTITY")) || bytes.Contains(data, []byte("<!DOCTYPE")) {
			return reject("%s declares entities", f.Name)
		}
	}
	return nil
}

// Line is one line of resume text, numbered from 1.
type Line struct {
	N      int    `json:"n"`
	Text   string `json:"text"`
	Hidden bool   `json:"hidden"` // white, invisible or vanished text: shown to the user, flagged
	Tiny   bool   `json:"tiny"`
}

// Result is what the sandbox returns.
type Result struct {
	Kind  string `json:"kind"`
	Pages int    `json:"pages"`
	Lines []Line `json:"lines"`
	Error string `json:"error"`
}

// Runner starts sandbox containers.
type Runner struct {
	Image   string        // the extractor image, pinned by digest outside development
	Runtime string        // "runsc" (gVisor) in production; "" is Docker's default, for development
	Timeout time.Duration // whole run, create to result; 20 s if zero
}

// Args are the docker run flags for one container: no network, read-only root, every
// capability dropped, no privilege gain, nobody, a small noexec /tmp, 512 MB with no swap,
// one CPU, 64 processes, no logs kept.
func (r Runner) Args(name string) []string {
	a := []string{"run", "--rm", "-i", "--name", name, "--label", "rolerank.managed=true", "--label", "rolerank.role=extract",
		"--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--user", "65534:65534", "--tmpfs", "/tmp:rw,noexec,nosuid,nodev,size=16m",
		"--memory", "512m", "--memory-swap", "512m", "--cpus", "1", "--pids-limit", "64",
		"--ulimit", "nofile=256:256", "--log-driver", "none"}
	if r.Runtime != "" {
		a = append(a, "--runtime", r.Runtime)
	}
	return append(a, r.Image)
}

// Run checks the file, then extracts it in a fresh container, which is always removed.
func (r Runner) Run(ctx context.Context, b []byte) (Result, error) {
	kind, err := Check(b)
	if err != nil {
		return Result{}, err
	}
	timeout := r.Timeout
	if timeout == 0 {
		timeout = 20 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	id := make([]byte, 8)
	_, _ = rand.Read(id)
	name := "rolerank-extract-" + hex.EncodeToString(id)
	cmd := exec.CommandContext(ctx, "docker", r.Args(name)...)
	cmd.Stdin = bytes.NewReader(b)
	out := &capped{max: MaxOutput}
	cmd.Stdout = out
	var stderr bytes.Buffer
	errOut := &capped{max: 4096, w: &stderr}
	cmd.Stderr = errOut
	runErr := cmd.Run()
	if ctx.Err() != nil || out.over || errOut.over {
		// Killing the docker client doesn't stop the container; kill it by name.
		_ = exec.Command("docker", "kill", name).Run()
		_ = exec.Command("docker", "rm", "-f", name).Run()
		if out.over {
			return Result{}, fmt.Errorf("extract: output over %d bytes", MaxOutput)
		}
		return Result{}, fmt.Errorf("extract: timed out after %s", timeout)
	}
	if runErr != nil {
		return Result{}, fmt.Errorf("extract: %v: %s", runErr, strings.TrimSpace(stderr.String()))
	}
	var res Result
	if err := json.Unmarshal(out.buf.Bytes(), &res); err != nil {
		return Result{}, fmt.Errorf("extract: bad output: %w", err)
	}
	if res.Error == "" && res.Kind != kind {
		return Result{}, fmt.Errorf("extract: sandbox read %q, check said %q", res.Kind, kind)
	}
	return res, nil
}

// capped keeps at most max bytes, in buf or passed on to w, and refuses the rest.
type capped struct {
	buf  bytes.Buffer
	w    io.Writer
	max  int
	n    int
	over bool
}

func (c *capped) Write(p []byte) (int, error) {
	if c.n+len(p) > c.max {
		c.over = true
		return 0, errors.New("output cap")
	}
	c.n += len(p)
	if c.w != nil {
		return c.w.Write(p)
	}
	return c.buf.Write(p)
}
