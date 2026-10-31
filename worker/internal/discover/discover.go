// Package discover finds board names in Common Crawl's URL index. It reads URLs from the
// index only, never page content, and contacts no ATS. See docs/SOURCES.md, "Discovery".
package discover

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	indexHost  = "https://index.commoncrawl.org"
	pause      = 3 * time.Second
	maxBody    = 256 << 20
	maxRetries = 5
)

// Target is one host pattern to read from the index.
type Target struct {
	Source  string
	Pattern string
	Prefix  string
}

// Targets lists every source whose boards are found this way.
var Targets = []Target{
	{"greenhouse", "boards.greenhouse.io/*", "boards.greenhouse.io"},
	{"greenhouse", "job-boards.greenhouse.io/*", "job-boards.greenhouse.io"},
	{"lever", "jobs.lever.co/*", "jobs.lever.co"},
	{"ashby", "jobs.ashbyhq.com/*", "jobs.ashbyhq.com"},
}

var tokenShape = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$`)

// Paths on these hosts that aren't boards.
var notBoards = map[string]bool{
	"embed": true, "jobs": true, "api": true, "robots.txt": true, "sitemap.xml": true,
	"favicon.ico": true, "assets": true, "static": true, "_next": true, "login": true,
}

// Token returns the board name in a URL from the index, or "" if there isn't one.
func Token(raw, host string) string {
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Hostname(), host) {
		return ""
	}
	first, _, _ := strings.Cut(strings.TrimPrefix(u.Path, "/"), "/")
	if !tokenShape.MatchString(first) || notBoards[strings.ToLower(first)] {
		return ""
	}
	return first
}

// State records finished work, so a stopped run repeats nothing.
type State struct {
	Done map[string]bool `json:"done"`
}

func loadState(path string) (State, error) {
	state := State{Done: map[string]bool{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	return state, json.Unmarshal(data, &state)
}

// writeFile replaces a file in one step, so a stop can't leave half of it.
func writeFile(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Runner reads the index one request at a time.
type Runner struct {
	Dir       string
	UserAgent string
	Client    *http.Client
	Base      string
	Sleep     func(time.Duration)
	Log       func(string)
}

func (r *Runner) get(ctx context.Context, target string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", r.UserAgent)
	resp, err := r.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, err
	}
	return body, nil
}

// fetchRetrying waits longer after each failure, because the index server is often busy.
func (r *Runner) fetchRetrying(ctx context.Context, target string) ([]byte, error) {
	var last error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		body, err := r.get(ctx, target)
		if err == nil {
			return body, nil
		}
		last = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		r.Sleep(time.Duration(attempt*attempt) * 10 * time.Second)
	}
	return nil, last
}

func (r *Runner) query(index, pattern string, extra url.Values) string {
	q := url.Values{"url": {pattern}, "output": {"json"}}
	for k, v := range extra {
		q[k] = v
	}
	return r.Base + "/" + index + "-index?" + q.Encode()
}

// LatestIndexes returns the newest n index names.
func (r *Runner) LatestIndexes(ctx context.Context, n int) ([]string, error) {
	body, err := r.fetchRetrying(ctx, r.Base+"/collinfo.json")
	if err != nil {
		return nil, fmt.Errorf("list indexes: %w", err)
	}
	var list []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	var out []string
	for i := 0; i < len(list) && i < n; i++ {
		out = append(out, list[i].ID)
	}
	return out, nil
}

func (r *Runner) addTokens(sourceName string, found map[string]bool) error {
	path := filepath.Join(r.Dir, sourceName+".txt")
	known := map[string]string{}
	if file, err := os.Open(path); err == nil {
		lines := bufio.NewScanner(file)
		for lines.Scan() {
			known[strings.ToLower(lines.Text())] = lines.Text()
		}
		file.Close()
		if err := lines.Err(); err != nil {
			return err
		}
	}
	for token := range found {
		if _, ok := known[strings.ToLower(token)]; !ok {
			known[strings.ToLower(token)] = token
		}
	}
	tokens := make([]string, 0, len(known))
	for _, token := range known {
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)
	return writeFile(path, []byte(strings.Join(tokens, "\n")+"\n"))
}

// Run reads every target in the given indexes. A page that keeps failing is left undone,
// so the next run tries it again. It returns how many pages are still undone.
func (r *Runner) Run(ctx context.Context, indexes []string) (int, error) {
	statePath := filepath.Join(r.Dir, "discover-state.json")
	state, err := loadState(statePath)
	if err != nil {
		return 0, err
	}
	undone := 0
	for _, index := range indexes {
		for _, t := range Targets {
			pagesKey := index + "|" + t.Pattern
			body, err := r.fetchRetrying(ctx, r.query(index, t.Pattern, url.Values{"showNumPages": {"true"}}))
			if err != nil {
				if ctx.Err() != nil {
					return undone, ctx.Err()
				}
				r.Log(fmt.Sprintf("%s: page count unavailable: %v", pagesKey, err))
				undone++
				continue
			}
			var info struct {
				Pages int `json:"pages"`
			}
			if err := json.Unmarshal(body, &info); err != nil {
				return undone, err
			}
			for page := range info.Pages {
				key := fmt.Sprintf("%s|%d", pagesKey, page)
				if state.Done[key] {
					continue
				}
				r.Sleep(pause)
				body, err := r.fetchRetrying(ctx, r.query(index, t.Pattern, url.Values{
					"fl": {"url"}, "collapse": {"urlkey"}, "page": {fmt.Sprint(page)},
				}))
				if err != nil {
					if ctx.Err() != nil {
						return undone, ctx.Err()
					}
					r.Log(fmt.Sprintf("%s: left for the next run: %v", key, err))
					undone++
					continue
				}
				found := map[string]bool{}
				lines := bufio.NewScanner(strings.NewReader(string(body)))
				lines.Buffer(make([]byte, 1<<20), 1<<20)
				for lines.Scan() {
					var row struct {
						URL string `json:"url"`
					}
					if json.Unmarshal(lines.Bytes(), &row) != nil {
						continue
					}
					if token := Token(row.URL, t.Prefix); token != "" {
						found[token] = true
					}
				}
				if err := r.addTokens(t.Source, found); err != nil {
					return undone, err
				}
				state.Done[key] = true
				data, err := json.Marshal(state)
				if err != nil {
					return undone, err
				}
				if err := writeFile(statePath, data); err != nil {
					return undone, err
				}
				r.Log(fmt.Sprintf("%s: %d boards in this page", key, len(found)))
			}
		}
	}
	return undone, nil
}
