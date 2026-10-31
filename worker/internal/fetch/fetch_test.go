package fetch

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

const ua = "RoleRankBot/test (+https://example.test/about/bot; bot@dhruvgoyal.tech)"

type fakeLimiter struct {
	mu        sync.Mutex
	intervals []time.Duration
	blocked   map[string]time.Duration
}

func (f *fakeLimiter) Wait(_ context.Context, _ string, interval time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.intervals = append(f.intervals, interval)
	return nil
}

func (f *fakeLimiter) Block(_ context.Context, host string, d time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.blocked == nil {
		f.blocked = map[string]time.Duration{}
	}
	f.blocked[host] = d
	return nil
}

type source struct {
	server *httptest.Server
	mu     sync.Mutex
	hits   map[string]int
	seen   []*http.Request
}

func (s *source) count(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits[path]
}

func newSource(t *testing.T, robotsTxt func(http.ResponseWriter), handler http.HandlerFunc) *source {
	t.Helper()
	s := &source{hits: map[string]int{}}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.hits[r.URL.Path]++
		s.seen = append(s.seen, r)
		s.mu.Unlock()
		if r.URL.Path == "/robots.txt" {
			robotsTxt(w)
			return
		}
		handler(w, r)
	}))
	t.Cleanup(s.server.Close)
	return s
}

func openRobots(w http.ResponseWriter) { w.WriteHeader(http.StatusNotFound) }

func client(s *source, limiter Limiter) *Client {
	u, _ := url.Parse(s.server.URL)
	return New(Options{
		UserAgent:    ua,
		Agent:        "RoleRankBot",
		Hosts:        []string{u.Host},
		Limiter:      limiter,
		AllowHTTP:    true,
		AllowPrivate: true,
	})
}

func TestSendsHonestUserAgentAndReturnsBody(t *testing.T) {
	s := newSource(t, openRobots, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `W/"abc"`)
		_, _ = w.Write([]byte(`{"jobs":[]}`))
	})
	res, err := client(s, &fakeLimiter{}).Get(context.Background(), s.server.URL+"/jobs", "")
	if err != nil {
		t.Fatal(err)
	}
	if string(res.Body) != `{"jobs":[]}` || res.ETag != `W/"abc"` || res.NotModified {
		t.Fatalf("unexpected result: %+v", res)
	}
	for _, r := range s.seen {
		if r.Header.Get("User-Agent") != ua {
			t.Fatalf("%s sent User-Agent %q", r.URL.Path, r.Header.Get("User-Agent"))
		}
	}
}

func TestConditionalGet(t *testing.T) {
	s := newSource(t, openRobots, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `W/"abc"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	})
	res, err := client(s, &fakeLimiter{}).Get(context.Background(), s.server.URL+"/jobs", `W/"abc"`)
	if err != nil {
		t.Fatal(err)
	}
	if !res.NotModified || len(res.Body) != 0 {
		t.Fatalf("want NotModified with no body, got %+v", res)
	}
}

func TestRobotsIsObeyedBeforeTheRequest(t *testing.T) {
	s := newSource(t, func(w http.ResponseWriter) {
		_, _ = w.Write([]byte("User-agent: *\nDisallow: /jobs\n"))
	}, func(http.ResponseWriter, *http.Request) {})
	_, err := client(s, &fakeLimiter{}).Get(context.Background(), s.server.URL+"/jobs", "")
	if !errors.Is(err, ErrDisallowed) {
		t.Fatalf("want ErrDisallowed, got %v", err)
	}
	if s.count("/jobs") != 0 {
		t.Fatal("a disallowed path was requested")
	}
}

func TestRobots5xxBlocksTheHost(t *testing.T) {
	s := newSource(t, func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusInternalServerError)
	}, func(http.ResponseWriter, *http.Request) {})
	_, err := client(s, &fakeLimiter{}).Get(context.Background(), s.server.URL+"/jobs", "")
	if !errors.Is(err, ErrRobotsUnavailable) || s.count("/jobs") != 0 {
		t.Fatalf("a 5xx on robots.txt must stop the host, got %v", err)
	}
}

func TestUnreadableRobotsIsRetriedSoon(t *testing.T) {
	failing := true
	s := newSource(t, func(w http.ResponseWriter) {
		if failing {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}, func(http.ResponseWriter, *http.Request) {})
	now := time.Now()
	c := client(s, &fakeLimiter{})
	c.now = func() time.Time { return now }
	if _, err := c.Get(context.Background(), s.server.URL+"/jobs", ""); !errors.Is(err, ErrRobotsUnavailable) {
		t.Fatalf("got %v", err)
	}
	failing = false
	now = now.Add(robotsRetry + time.Second)
	if _, err := c.Get(context.Background(), s.server.URL+"/jobs", ""); err != nil {
		t.Fatalf("a brief outage must clear within minutes, got %v", err)
	}
}

func TestRobotsIsCachedThenRefreshed(t *testing.T) {
	s := newSource(t, openRobots, func(http.ResponseWriter, *http.Request) {})
	now := time.Now()
	c := client(s, &fakeLimiter{})
	c.now = func() time.Time { return now }
	for range 3 {
		if _, err := c.Get(context.Background(), s.server.URL+"/jobs", ""); err != nil {
			t.Fatal(err)
		}
	}
	if got := s.count("/robots.txt"); got != 1 {
		t.Fatalf("robots.txt fetched %d times, want 1", got)
	}
	now = now.Add(RobotsTTL + time.Minute)
	if _, err := c.Get(context.Background(), s.server.URL+"/jobs", ""); err != nil {
		t.Fatal(err)
	}
	if got := s.count("/robots.txt"); got != 2 {
		t.Fatalf("robots.txt fetched %d times after expiry, want 2", got)
	}
}

func TestCrawlDelayRaisesTheSpacing(t *testing.T) {
	s := newSource(t, func(w http.ResponseWriter) {
		_, _ = w.Write([]byte("User-agent: *\nCrawl-delay: 5\n"))
	}, func(http.ResponseWriter, *http.Request) {})
	limiter := &fakeLimiter{}
	if _, err := client(s, limiter).Get(context.Background(), s.server.URL+"/jobs", ""); err != nil {
		t.Fatal(err)
	}
	if last := limiter.intervals[len(limiter.intervals)-1]; last != 5*time.Second {
		t.Fatalf("spacing %v, want 5s", last)
	}
}

func TestEveryRequestPassesTheLimiter(t *testing.T) {
	s := newSource(t, openRobots, func(http.ResponseWriter, *http.Request) {})
	limiter := &fakeLimiter{}
	if _, err := client(s, limiter).Get(context.Background(), s.server.URL+"/jobs", ""); err != nil {
		t.Fatal(err)
	}
	if len(limiter.intervals) != 2 {
		t.Fatalf("%d limiter calls for robots plus one fetch, want 2", len(limiter.intervals))
	}
}

func TestSpacingCannotBeLoweredForRealSources(t *testing.T) {
	c := New(Options{Spacing: time.Millisecond, Limiter: &fakeLimiter{}})
	if c.spacing != DefaultSpacing {
		t.Fatalf("spacing %v on a client that can reach real hosts", c.spacing)
	}
}

func TestBearerGoesOnlyToItsOwnHost(t *testing.T) {
	const secret = "planted-key"
	other := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("the key reached another host on %s", r.URL.Path)
		}
	}))
	defer other.Close()
	s := newSource(t, openRobots, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/hop" {
			http.Redirect(w, r, other.URL+"/landing", http.StatusFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+secret {
			t.Errorf("own host got %q", r.Header.Get("Authorization"))
		}
	})
	own, _ := url.Parse(s.server.URL)
	second, _ := url.Parse(other.URL)
	c := New(Options{
		UserAgent: ua, Agent: "RoleRankBot", Hosts: []string{own.Host, second.Host},
		Limiter: &fakeLimiter{}, AllowHTTP: true, AllowPrivate: true,
		Bearer: map[string]string{own.Host: secret},
	})
	for _, target := range []string{s.server.URL + "/jobs", other.URL + "/jobs", s.server.URL + "/hop"} {
		if _, err := c.Get(context.Background(), target, ""); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBearerIsNeverInAnError(t *testing.T) {
	const secret = "planted-key"
	s := newSource(t, openRobots, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})
	own, _ := url.Parse(s.server.URL)
	c := New(Options{
		Agent: "RoleRankBot", Hosts: []string{own.Host}, Limiter: &fakeLimiter{},
		AllowHTTP: true, AllowPrivate: true, Bearer: map[string]string{own.Host: secret},
	})
	_, err := c.Get(context.Background(), s.server.URL+"/jobs", "")
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("got %v", err)
	}
}

func TestRetryAfterBlocksTheHost(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		s := newSource(t, openRobots, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(status)
		})
		limiter := &fakeLimiter{}
		_, err := client(s, limiter).Get(context.Background(), s.server.URL+"/jobs", "")
		if !errors.Is(err, ErrBackoff) {
			t.Fatalf("status %d: want ErrBackoff, got %v", status, err)
		}
		u, _ := url.Parse(s.server.URL)
		if limiter.blocked[u.Host] != 2*time.Minute {
			t.Fatalf("status %d: blocked for %v", status, limiter.blocked[u.Host])
		}
	}
}

func TestRetryAfterParsing(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	tests := map[string]time.Duration{
		"30":                            30 * time.Second,
		"":                              time.Minute,
		"soon":                          time.Minute,
		"0":                             time.Second,
		"999999":                        time.Hour,
		"Sat, 31 Oct 2026 12:05:00 GMT": 5 * time.Minute,
		"Sat, 31 Oct 2026 11:00:00 GMT": time.Second,
	}
	for value, want := range tests {
		if got := retryAfter(value, now); got != want {
			t.Errorf("retryAfter(%q) = %v, want %v", value, got, want)
		}
	}
}

func TestRefusesHostsOffTheAllowlist(t *testing.T) {
	s := newSource(t, openRobots, func(http.ResponseWriter, *http.Request) {})
	c := New(Options{Hosts: []string{"boards-api.greenhouse.io"}, Limiter: &fakeLimiter{}, AllowHTTP: true, AllowPrivate: true})
	_, err := c.Get(context.Background(), s.server.URL+"/jobs", "")
	if !errors.Is(err, ErrHostNotAllowed) {
		t.Fatalf("want ErrHostNotAllowed, got %v", err)
	}
	if len(s.seen) != 0 {
		t.Fatal("a request left for a host off the allowlist")
	}
}

func TestRefusesPlainHTTP(t *testing.T) {
	c := New(Options{Hosts: []string{"example.test"}, Limiter: &fakeLimiter{}})
	if _, err := c.Get(context.Background(), "http://example.test/x", ""); !errors.Is(err, ErrInsecure) {
		t.Fatalf("want ErrInsecure, got %v", err)
	}
}

func TestRefusesRedirectOffTheAllowlist(t *testing.T) {
	evil := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("followed a redirect off the allowlist")
	}))
	defer evil.Close()
	s := newSource(t, openRobots, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, evil.URL+"/steal", http.StatusFound)
	})
	_, err := client(s, &fakeLimiter{}).Get(context.Background(), s.server.URL+"/jobs", "")
	if !errors.Is(err, ErrHostNotAllowed) {
		t.Fatalf("want ErrHostNotAllowed, got %v", err)
	}
}

func TestRefusesPrivateAddressesByDefault(t *testing.T) {
	s := newSource(t, openRobots, func(http.ResponseWriter, *http.Request) {})
	u, _ := url.Parse(s.server.URL)
	c := New(Options{Hosts: []string{u.Host}, Limiter: &fakeLimiter{}, AllowHTTP: true})
	if _, err := c.do(context.Background(), u, ""); err == nil || !strings.Contains(err.Error(), "refusing address") {
		t.Fatalf("want a refusal, got %v", err)
	}
	if len(s.seen) != 0 {
		t.Fatal("a request reached a loopback address")
	}
}

func TestOversizedBody(t *testing.T) {
	s := newSource(t, openRobots, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, MaxBody+1))
	})
	_, err := client(s, &fakeLimiter{}).Get(context.Background(), s.server.URL+"/jobs", "")
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("want ErrTooLarge, got %v", err)
	}
}

func TestGzipBomb(t *testing.T) {
	var buf bytes.Buffer
	z := gzip.NewWriter(&buf)
	chunk := make([]byte, 1<<20)
	for range 30 {
		_, _ = z.Write(chunk)
	}
	_ = z.Close()
	if buf.Len() > 1<<20 {
		t.Fatalf("fixture isn't a bomb: %d bytes", buf.Len())
	}
	s := newSource(t, openRobots, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(buf.Bytes())
	})
	_, err := client(s, &fakeLimiter{}).Get(context.Background(), s.server.URL+"/jobs", "")
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("want ErrTooLarge, got %v", err)
	}
}

func TestDecodesNormalGzip(t *testing.T) {
	var buf bytes.Buffer
	z := gzip.NewWriter(&buf)
	_, _ = z.Write([]byte(`{"ok":true}`))
	_ = z.Close()
	s := newSource(t, openRobots, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(buf.Bytes())
	})
	res, err := client(s, &fakeLimiter{}).Get(context.Background(), s.server.URL+"/jobs", "")
	if err != nil || string(res.Body) != `{"ok":true}` {
		t.Fatalf("got %v, %v", res, err)
	}
}
