package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestToken(t *testing.T) {
	tests := []struct{ raw, host, want string }{
		{"https://boards.greenhouse.io/acme", "boards.greenhouse.io", "acme"},
		{"https://boards.greenhouse.io/acme/jobs/123", "boards.greenhouse.io", "acme"},
		{"https://jobs.lever.co/Acme-Inc/uuid?lever-source=x", "jobs.lever.co", "Acme-Inc"},
		{"https://jobs.ashbyhq.com/acme.io/uuid", "jobs.ashbyhq.com", "acme.io"},
		{"https://boards.greenhouse.io/embed/job_board?for=acme", "boards.greenhouse.io", ""},
		{"https://boards.greenhouse.io/", "boards.greenhouse.io", ""},
		{"https://boards.greenhouse.io/robots.txt", "boards.greenhouse.io", ""},
		{"https://evil.example/acme", "boards.greenhouse.io", ""},
		{"https://boards.greenhouse.io/%3Cscript%3E", "boards.greenhouse.io", ""},
		{"https://boards.greenhouse.io/../etc", "boards.greenhouse.io", ""},
	}
	for _, tt := range tests {
		if got := Token(tt.raw, tt.host); got != tt.want {
			t.Errorf("Token(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

// fakeIndex serves one index with two pages per pattern. Page 1 of the first pattern
// fails until allowed to work.
func fakeIndex(t *testing.T, fail *atomic.Bool, hits *atomic.Int64) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		q := r.URL.Query()
		if q.Get("showNumPages") == "true" {
			_ = json.NewEncoder(w).Encode(map[string]int{"pages": 2})
			return
		}
		pattern := q.Get("url")
		page := q.Get("page")
		if pattern == "boards.greenhouse.io/*" && page == "1" && fail.Load() {
			w.WriteHeader(http.StatusGatewayTimeout)
			return
		}
		host := strings.TrimSuffix(pattern, "/*")
		fmt.Fprintf(w, "{\"url\":\"https://%s/board%s/jobs/1\"}\n{\"url\":\"https://%s/shared\"}\nnot json\n", host, page, host)
	}))
	t.Cleanup(server.Close)
	return server
}

func runner(t *testing.T, base, dir string) *Runner {
	t.Helper()
	return &Runner{
		Dir: dir, UserAgent: "test", Client: http.DefaultClient, Base: base,
		Sleep: func(time.Duration) {}, Log: func(string) {},
	}
}

func lines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(data))
}

func TestRunWritesBoardsAndResumes(t *testing.T) {
	dir := t.TempDir()
	var fail atomic.Bool
	var hits atomic.Int64
	fail.Store(true)
	server := fakeIndex(t, &fail, &hits)
	r := runner(t, server.URL, dir)

	undone, err := r.Run(context.Background(), []string{"CC-1"})
	if err != nil {
		t.Fatal(err)
	}
	if undone != 1 {
		t.Fatalf("%d pages undone, want 1", undone)
	}
	if got := lines(t, filepath.Join(dir, "lever.txt")); len(got) != 3 {
		t.Fatalf("lever: %v", got)
	}

	fail.Store(false)
	hits.Store(0)
	undone, err = r.Run(context.Background(), []string{"CC-1"})
	if err != nil || undone != 0 {
		t.Fatalf("undone %d, err %v", undone, err)
	}
	// Four page counts, plus only the one page that hadn't finished.
	if got := hits.Load(); got != 5 {
		t.Fatalf("second run made %d requests, want 5: finished pages must not repeat", got)
	}
	greenhouse := lines(t, filepath.Join(dir, "greenhouse.txt"))
	if strings.Join(greenhouse, ",") != "board0,board1,shared" {
		t.Fatalf("greenhouse: %v", greenhouse)
	}
}

func TestTokensAreDedupedIgnoringCase(t *testing.T) {
	dir := t.TempDir()
	r := runner(t, "", dir)
	if err := r.addTokens("ashby", map[string]bool{"Acme": true}); err != nil {
		t.Fatal(err)
	}
	if err := r.addTokens("ashby", map[string]bool{"acme": true, "beta": true}); err != nil {
		t.Fatal(err)
	}
	got := lines(t, filepath.Join(dir, "ashby.txt"))
	if strings.Join(got, ",") != "Acme,beta" {
		t.Fatalf("%v", got)
	}
}
