// Package fetch makes every outbound request RoleRank sends to a job source. It's the
// one place that applies the rules in CLAUDE.md: an allowlist of hosts, an honest
// User-Agent, robots.txt, the shared rate limit, conditional GET, and hard size limits.
package fetch

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/DhruvGoyal404/rolerank/worker/internal/robots"
)

// Limits. The largest real boards send 13 MB of gzip that expands well past 25 MB, so the
// wire and the decompressed size have separate caps.
const (
	MaxBody        = 25 << 20
	MaxDecoded     = 256 << 20
	Timeout        = 120 * time.Second // large boards are slow to send; this isn't a rate
	RobotsTTL      = 24 * time.Hour
	robotsRetry    = 5 * time.Minute
	DefaultSpacing = time.Second
	maxRetryAfter  = time.Hour
)

// Errors a caller decides on. Everything else is a transport failure.
var (
	ErrHostNotAllowed = errors.New("fetch: host not on the allowlist")
	ErrDisallowed     = errors.New("fetch: disallowed by robots.txt")
	// ErrRobotsUnavailable means robots.txt couldn't be read, so nothing may be fetched for
	// now. It's a pause, not a verdict: a Wi-Fi drop mustn't look like a refusal.
	ErrRobotsUnavailable = errors.New("fetch: robots.txt unavailable")
	ErrTooLarge          = errors.New("fetch: response over the size limit")
	ErrBackoff           = errors.New("fetch: source asked us to back off")
	ErrInsecure          = errors.New("fetch: https only")
)

// Limiter is the shared per-host schedule.
type Limiter interface {
	Wait(ctx context.Context, host string, interval time.Duration) error
	Block(ctx context.Context, host string, d time.Duration) error
}

// Result is a completed exchange. NotModified means the caller's ETag still holds.
type Result struct {
	Status      int
	Body        []byte
	ETag        string
	NotModified bool
}

// Options configures a Client. AllowHTTP and AllowPrivate exist for local fake sources.
type Options struct {
	Spacing time.Duration // test-only, and refused unless AllowPrivate is set
	// Bearer maps a host to its API key. A key goes only to its own host, and nothing
	// here ever logs it or puts it in an error.
	Bearer       map[string]string
	UserAgent    string
	Agent        string
	Accept       string // defaults to application/json
	Hosts        []string
	Limiter      Limiter
	AllowHTTP    bool
	AllowPrivate bool
	Now          func() time.Time
}

type cachedPolicy struct {
	policy      robots.Policy
	fetched     time.Time
	unavailable bool
}

// Client is safe for concurrent use.
type Client struct {
	opts    Options
	spacing time.Duration
	hosts   map[string]bool
	http    *http.Client
	now     func() time.Time
	mu      sync.Mutex
	robots  map[string]cachedPolicy
}

// New builds a Client.
func New(opts Options) *Client {
	c := &Client{
		opts:   opts,
		hosts:  make(map[string]bool, len(opts.Hosts)),
		now:    opts.Now,
		robots: make(map[string]cachedPolicy),
	}
	if c.now == nil {
		c.now = time.Now
	}
	for _, h := range opts.Hosts {
		c.hosts[h] = true
	}
	// Real sources are never fetched faster than DefaultSpacing, whatever the caller asks.
	c.spacing = DefaultSpacing
	if opts.AllowPrivate && opts.Spacing > 0 {
		c.spacing = opts.Spacing
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if !opts.AllowPrivate {
		dialer.Control = refusePrivate
	}
	c.http = &http.Client{
		Timeout: Timeout,
		Transport: &http.Transport{
			DialContext: dialer.DialContext,
			// Off so we can cap the decompressed size ourselves.
			DisableCompression: true,
			Proxy:              nil,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("fetch: too many redirects")
			}
			// Go keeps Authorization across ports of one host, so decide on every hop.
			req.Header.Del("Authorization")
			if token := opts.Bearer[req.URL.Host]; token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			return c.checkURL(req.URL)
		},
	}
	return c
}

// refusePrivate runs after DNS, so a public name pointing inward is still refused.
func refusePrivate(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return fmt.Errorf("fetch: refusing address %s", address)
	}
	return nil
}

func (c *Client) checkURL(u *url.URL) error {
	if u.Scheme != "https" && !(c.opts.AllowHTTP && u.Scheme == "http") {
		return ErrInsecure
	}
	if !c.hosts[u.Host] {
		return fmt.Errorf("%w: %s", ErrHostNotAllowed, u.Host)
	}
	return nil
}

// Get fetches rawURL, sending etag when it's known. A 304 returns NotModified. A 429 or
// 503 blocks the host for every worker and returns ErrBackoff.
func (c *Client) Get(ctx context.Context, rawURL, etag string) (*Result, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if err := c.checkURL(u); err != nil {
		return nil, err
	}
	policy, unavailable, err := c.policy(ctx, u)
	if err != nil {
		return nil, err
	}
	if unavailable {
		return nil, ErrRobotsUnavailable
	}
	if !policy.Allowed(u.RequestURI()) {
		return nil, ErrDisallowed
	}
	spacing := c.spacing
	if delay := policy.CrawlDelay(); delay > spacing {
		spacing = delay
	}
	if err := c.opts.Limiter.Wait(ctx, u.Host, spacing); err != nil {
		return nil, err
	}

	res, err := c.do(ctx, u, etag)
	if err != nil {
		return nil, err
	}
	if res.Status == http.StatusNotModified {
		res.NotModified = true
	}
	return res, nil
}

func (c *Client) do(ctx context.Context, u *url.URL, etag string) (*Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.opts.UserAgent)
	accept := c.opts.Accept
	if accept == "" {
		accept = "application/json"
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Encoding", "gzip")
	if token := c.opts.Bearer[u.Host]; token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) && (errors.Is(urlErr.Err, ErrHostNotAllowed) || errors.Is(urlErr.Err, ErrInsecure)) {
			return nil, urlErr.Err
		}
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
		wait := retryAfter(resp.Header.Get("Retry-After"), c.now())
		if err := c.opts.Limiter.Block(ctx, u.Host, wait); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %d for %v", ErrBackoff, resp.StatusCode, wait)
	}

	body, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	return &Result{Status: resp.StatusCode, Body: body, ETag: resp.Header.Get("ETag")}, nil
}

// retryAfter reads seconds or an HTTP date, with a sane default and ceiling.
func retryAfter(value string, now time.Time) time.Duration {
	wait := time.Minute
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		wait = time.Duration(seconds) * time.Second
	} else if at, err := http.ParseTime(value); err == nil {
		wait = at.Sub(now)
	}
	if wait < time.Second {
		wait = time.Second
	}
	if wait > maxRetryAfter {
		wait = maxRetryAfter
	}
	return wait
}

func readBody(resp *http.Response) ([]byte, error) {
	var reader io.Reader = io.LimitReader(resp.Body, MaxBody+1)
	if resp.Header.Get("Content-Encoding") == "gzip" {
		compressed, err := io.ReadAll(reader)
		if err != nil {
			return nil, err
		}
		if len(compressed) > MaxBody {
			return nil, ErrTooLarge
		}
		zipped, err := gzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			return nil, err
		}
		defer zipped.Close()
		body, err := io.ReadAll(io.LimitReader(zipped, MaxDecoded+1))
		if err != nil {
			return nil, err
		}
		if len(body) > MaxDecoded {
			return nil, ErrTooLarge
		}
		return body, nil
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if len(body) > MaxBody {
		return nil, ErrTooLarge
	}
	return body, nil
}

func (c *Client) policy(ctx context.Context, u *url.URL) (robots.Policy, bool, error) {
	c.mu.Lock()
	cached, ok := c.robots[u.Host]
	c.mu.Unlock()
	ttl := RobotsTTL
	if cached.unavailable {
		ttl = robotsRetry
	}
	if ok && c.now().Sub(cached.fetched) < ttl {
		return cached.policy, cached.unavailable, nil
	}
	if err := c.opts.Limiter.Wait(ctx, u.Host, c.spacing); err != nil {
		return robots.Policy{}, false, err
	}
	target := &url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/robots.txt"}
	status, body := 0, ""
	// A failed read stays status 0, which RFC 9309 treats as complete disallow.
	if res, err := c.do(ctx, target, ""); err == nil {
		status, body = res.Status, string(res.Body)
	} else if errors.Is(err, ErrBackoff) {
		status = http.StatusServiceUnavailable
	}
	policy := robots.ForStatus(status, body, c.opts.Agent)
	unavailable := status == 0 || status >= 500
	c.mu.Lock()
	c.robots[u.Host] = cachedPolicy{policy: policy, fetched: c.now(), unavailable: unavailable}
	c.mu.Unlock()
	return policy, unavailable, nil
}
