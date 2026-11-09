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

// Why a sandbox run failed, for a plain message to the user. None carries file content.
var (
	ErrStartup = errors.New("sandbox didn't start in time") // Docker was slow, not the file
	ErrTimeout = errors.New("extraction timed out")         // the file ran past its limit
	ErrOutput  = errors.New("extraction output too large")
	ErrSandbox = errors.New("sandbox failed")
)

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
	Image          string        // the extractor image, pinned by digest outside development
	Runtime        string        // "runsc" (gVisor) in production; "" is Docker's default, for development
	Timeout        time.Duration // the extraction itself, once the container exists; 20 s if zero
	StartupTimeout time.Duration // Docker creating the container; 90 s if zero
	Docker         string        // the docker CLI; "docker" if empty (tests use a stand-in)
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
//
// Creating the container and running it have separate budgets. Docker Desktop can take 20 s or
// more just to create a container on a busy machine, and when one budget covered both, good files
// failed before the sandbox even started (walkthrough bug, 2026-11-08). The strict limit
// (Timeout) now applies only to the extraction, which is what it guards against.
func (r Runner) Run(ctx context.Context, b []byte) (Result, error) {
	kind, err := Check(b)
	if err != nil {
		return Result{}, err
	}
	docker := r.Docker
	if docker == "" {
		docker = "docker"
	}
	timeout, startup := r.Timeout, r.StartupTimeout
	if timeout == 0 {
		timeout = 20 * time.Second
	}
	if startup == 0 {
		startup = 90 * time.Second
	}
	id := make([]byte, 8)
	_, _ = rand.Read(id)
	name := "rolerank-extract-" + hex.EncodeToString(id)
	cleanup := func() {
		// Killing the docker client doesn't stop the container; kill it by name.
		_ = exec.Command(docker, "kill", name).Run()
		_ = exec.Command(docker, "rm", "-f", name).Run()
	}

	cctx, ccancel := context.WithTimeout(ctx, startup)
	defer ccancel()
	create := append([]string{"create"}, r.Args(name)[1:]...) // the run flags, without "run"
	var createErr bytes.Buffer
	cmd := exec.CommandContext(cctx, docker, create...)
	cmd.Stdout = io.Discard
	cmd.Stderr = &capped{max: 4096, w: &createErr}
	if err := cmd.Run(); err != nil {
		cleanup()
		if cctx.Err() != nil {
			return Result{}, fmt.Errorf("%w: no container after %s", ErrStartup, startup)
		}
		return Result{}, fmt.Errorf("%w: create: %v: %s", ErrSandbox, err, strings.TrimSpace(createErr.String()))
	}

	rctx, rcancel := context.WithTimeout(ctx, timeout)
	defer rcancel()
	cmd = exec.CommandContext(rctx, docker, "start", "--attach", "--interactive", name)
	cmd.Stdin = bytes.NewReader(b)
	out := &capped{max: MaxOutput}
	cmd.Stdout = out
	var stderr bytes.Buffer
	errOut := &capped{max: 4096, w: &stderr}
	cmd.Stderr = errOut
	runErr := cmd.Run()
	if rctx.Err() != nil || out.over || errOut.over {
		cleanup()
		if out.over {
			return Result{}, fmt.Errorf("%w: output over %d bytes", ErrOutput, MaxOutput)
		}
		if errOut.over {
			return Result{}, fmt.Errorf("%w: stderr over 4096 bytes", ErrSandbox)
		}
		return Result{}, fmt.Errorf("%w: after %s", ErrTimeout, timeout)
	}
	if runErr != nil {
		cleanup()
		return Result{}, fmt.Errorf("%w: %v: %s", ErrSandbox, runErr, strings.TrimSpace(stderr.String()))
	}
	var res Result
	if err := json.Unmarshal(out.buf.Bytes(), &res); err != nil {
		return Result{}, fmt.Errorf("%w: bad output: %v", ErrSandbox, err)
	}
	if res.Error == "" && res.Kind != kind {
		return Result{}, fmt.Errorf("%w: sandbox read %q, check said %q", ErrSandbox, res.Kind, kind)
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
